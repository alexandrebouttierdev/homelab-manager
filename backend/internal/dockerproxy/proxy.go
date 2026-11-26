package dockerproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxResponseBytes = 8 << 20
	dockerAPIVersion = "/v1.41"
	// maxLogTail bounds the lines of one log request (the Manager asks
	// for at most domain.MaxContainerLogLines).
	maxLogTail = 2000
	// maxActionResponseBytes bounds the answer of Docker to an action: an
	// empty body or a short error message.
	maxActionResponseBytes = 64 << 10
	// actionTimeout bounds a start, stop or restart: Docker waits up to the
	// stop timeout of the container (10 s by default) before killing it.
	actionTimeout = 75 * time.Second
	// TooLargeHeader marks an answer refused because it exceeds
	// maxResponseBytes, so the Manager can ask for fewer log lines.
	TooLargeHeader = "X-MonoOps-Too-Large"
)

// dockerStreamTypes are the content types of a log answer: multiplexed for
// a container without TTY, raw with one.
var dockerStreamTypes = map[string]bool{
	"application/vnd.docker.multiplexed-stream": true,
	"application/vnd.docker.raw-stream":         true,
}

// containerVerbs are the only actions the proxy forwards, and only when
// Options.ContainerActions is true.
var containerVerbs = map[string]bool{"start": true, "stop": true, "restart": true}

// Options are the settings of the proxy.
type Options struct {
	// ContainerActions lets start, stop and restart through
	// (MONOOPS_DOCKER_PROXY_CONTAINER_ACTIONS). Off by default: the proxy
	// is then read-only.
	ContainerActions bool
}

var containerIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{12,64}$`)

type Proxy struct {
	client  *http.Client
	actions *http.Client
	options Options
	active  chan struct{}
}

// New returns a read-only proxy.
func New(engineSocket string) *Proxy {
	return NewWithOptions(engineSocket, Options{})
}

func NewWithOptions(engineSocket string, options Options) *Proxy {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", engineSocket)
		},
	}
	return &Proxy{
		client:  &http.Client{Transport: transport, Timeout: 10 * time.Second},
		actions: &http.Client{Transport: transport, Timeout: actionTimeout},
		options: options, active: make(chan struct{}, 16),
	}
}

func (p *Proxy) CloseIdleConnections() {
	p.client.CloseIdleConnections()
	p.actions.CloseIdleConnections()
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var serve func(http.ResponseWriter, *http.Request)
	switch {
	case r.Method == http.MethodGet && allowedPath(r.URL):
		serve = p.serveJSON
	case r.Method == http.MethodGet && logsPath(r.URL):
		serve = p.serveLogs
	case r.Method == http.MethodPost && actionPath(r.URL):
		if !p.options.ContainerActions {
			http.Error(w, "container actions are disabled", http.StatusForbidden)
			return
		}
		serve = p.serveAction
	default:
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	select {
	case p.active <- struct{}{}:
		defer func() { <-p.active }()
	case <-r.Context().Done():
		return
	default:
		http.Error(w, "Docker proxy is busy", http.StatusServiceUnavailable)
		return
	}
	serve(w, r)
}

func (p *Proxy) serveJSON(w http.ResponseWriter, r *http.Request) {
	upstreamURL := &url.URL{
		Scheme:   "http",
		Host:     "docker",
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstreamURL.String(), nil)
	if err != nil {
		http.Error(w, "invalid Docker request", http.StatusBadRequest)
		return
	}
	response, err := p.client.Do(request)
	if err != nil {
		http.Error(w, "Docker daemon unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		http.Error(w, "could not read Docker response", http.StatusBadGateway)
		return
	}
	if len(body) > maxResponseBytes {
		http.Error(w, "Docker response too large", http.StatusBadGateway)
		return
	}
	body, err = sanitize(body)
	if err != nil {
		http.Error(w, "invalid Docker response", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	if _, err := w.Write(body); err != nil {
		return
	}
}

// serveLogs forwards the end of a container log as Docker writes it, bounded
// by maxResponseBytes. A log is not JSON: it is not sanitized, and the
// Manager shows it only to signed-in users, like the system journal.
func (p *Proxy) serveLogs(w http.ResponseWriter, r *http.Request) {
	upstreamURL := &url.URL{Scheme: "http", Host: "docker", Path: r.URL.Path, RawQuery: r.URL.RawQuery}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstreamURL.String(), nil)
	if err != nil {
		http.Error(w, "invalid Docker request", http.StatusBadRequest)
		return
	}
	response, err := p.client.Do(request)
	if err != nil {
		http.Error(w, "Docker daemon unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		http.Error(w, "could not read Docker response", http.StatusBadGateway)
		return
	}
	if len(body) > maxResponseBytes {
		w.Header().Set(TooLargeHeader, "1")
		http.Error(w, "Docker response too large", http.StatusBadGateway)
		return
	}
	if response.StatusCode != http.StatusOK {
		writeDockerError(w, response.StatusCode, body)
		return
	}
	contentType := response.Header.Get("Content-Type")
	if !dockerStreamTypes[contentType] {
		// An old engine without the header writes the multiplexed format
		// unless the container has a TTY; the Manager tells them apart.
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		return
	}
}

// serveAction forwards start, stop or restart without body nor parameter:
// Docker applies the stop timeout of the container.
func (p *Proxy) serveAction(w http.ResponseWriter, r *http.Request) {
	upstreamURL := &url.URL{Scheme: "http", Host: "docker", Path: r.URL.Path}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstreamURL.String(), nil)
	if err != nil {
		http.Error(w, "invalid Docker request", http.StatusBadRequest)
		return
	}
	response, err := p.actions.Do(request)
	if err != nil {
		http.Error(w, "Docker daemon unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxActionResponseBytes+1))
	if err != nil || len(body) > maxActionResponseBytes {
		http.Error(w, "could not read Docker response", http.StatusBadGateway)
		return
	}
	if response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusNotModified {
		w.WriteHeader(response.StatusCode)
		return
	}
	writeDockerError(w, response.StatusCode, body)
}

// writeDockerError forwards an error status with the message of Docker
// when it is a JSON object, sanitized; any other body is dropped.
func writeDockerError(w http.ResponseWriter, status int, body []byte) {
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	sanitized, err := sanitize(body)
	if err != nil {
		http.Error(w, http.StatusText(status), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(sanitized)
}

// containerPath splits /v1.41/containers/<id>/<leaf> and checks the id;
// the path must be clean, without escape.
func containerPath(requestURL *url.URL) (string, bool) {
	if requestURL == nil || strings.Contains(requestURL.EscapedPath(), "%") ||
		strings.Contains(requestURL.Path, "..") || strings.Contains(requestURL.Path, "//") {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(requestURL.Path, dockerAPIVersion+"/containers/"), "/")
	if len(parts) != 2 || !containerIDPattern.MatchString(parts[0]) ||
		requestURL.Path != dockerAPIVersion+"/containers/"+parts[0]+"/"+parts[1] {
		return "", false
	}
	return parts[1], true
}

// logsPath allows the end of a log, without follow: stdout=1, stderr=1,
// timestamps=1, tail between 1 and maxLogTail, and an optional since in
// Unix seconds. Any other parameter is refused.
func logsPath(requestURL *url.URL) bool {
	leaf, ok := containerPath(requestURL)
	if !ok || leaf != "logs" {
		return false
	}
	query, err := url.ParseQuery(requestURL.RawQuery)
	if err != nil {
		return false
	}
	for key, values := range query {
		if len(values) != 1 {
			return false
		}
		switch key {
		case "stdout", "stderr", "timestamps":
			if values[0] != "1" {
				return false
			}
		case "tail":
			tail, err := strconv.Atoi(values[0])
			if err != nil || tail < 1 || tail > maxLogTail || strconv.Itoa(tail) != values[0] {
				return false
			}
		case "since":
			since, err := strconv.ParseInt(values[0], 10, 64)
			if err != nil || since < 1 || strconv.FormatInt(since, 10) != values[0] {
				return false
			}
		default:
			return false
		}
	}
	return query.Has("stdout") && query.Has("stderr") && query.Has("timestamps") && query.Has("tail")
}

// actionPath allows POST .../start, .../stop and .../restart without query.
func actionPath(requestURL *url.URL) bool {
	leaf, ok := containerPath(requestURL)
	return ok && containerVerbs[leaf] && requestURL.RawQuery == ""
}

func allowedPath(requestURL *url.URL) bool {
	if requestURL == nil || requestURL.Path == "" || strings.Contains(requestURL.EscapedPath(), "%") {
		return false
	}
	if strings.Contains(requestURL.Path, "..") || strings.Contains(requestURL.Path, "//") {
		return false
	}
	if requestURL.Path == dockerAPIVersion+"/containers/json" {
		query := requestURL.Query()
		return len(query) == 1 && len(query["all"]) == 1 && query.Get("all") == "1"
	}
	// The inventory of images, volumes and networks, without parameters:
	// no filter, no inspection of one item.
	switch requestURL.Path {
	case dockerAPIVersion + "/images/json", dockerAPIVersion + "/volumes", dockerAPIVersion + "/networks":
		return requestURL.RawQuery == ""
	}
	parts := strings.Split(strings.TrimPrefix(requestURL.Path, dockerAPIVersion+"/containers/"), "/")
	if len(parts) != 2 || !containerIDPattern.MatchString(parts[0]) {
		return false
	}
	switch parts[1] {
	case "json":
		return requestURL.Path == dockerAPIVersion+"/containers/"+parts[0]+"/json" && requestURL.RawQuery == ""
	case "stats":
		query := requestURL.Query()
		return requestURL.Path == dockerAPIVersion+"/containers/"+parts[0]+"/stats" &&
			len(query) == 1 && len(query["stream"]) == 1 && query.Get("stream") == "false"
	default:
		return false
	}
}

func sanitize(body []byte) ([]byte, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("multiple JSON values")
	}
	removeSecrets(value)
	return json.Marshal(value)
}

func removeSecrets(value any) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			removeSecrets(item)
		}
	case map[string]any:
		for key, item := range typed {
			switch strings.ToLower(key) {
			// Driver options of volumes and networks may hold credentials
			// (a CIFS password, for instance); the Manager never reads them.
			case "env", "labels", "options", "driveropts":
				delete(typed, key)
			default:
				removeSecrets(item)
			}
		}
	}
}
