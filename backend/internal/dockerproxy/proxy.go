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
	"strings"
	"time"
)

const (
	maxResponseBytes = 8 << 20
	dockerAPIVersion = "/v1.41"
)

var containerIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{12,64}$`)

type Proxy struct {
	client *http.Client
	active chan struct{}
}

func New(engineSocket string) *Proxy {
	return &Proxy{client: &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", engineSocket)
			},
		},
		Timeout: 10 * time.Second,
	}, active: make(chan struct{}, 16)}
}

func (p *Proxy) CloseIdleConnections() {
	p.client.CloseIdleConnections()
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !allowedPath(r.URL) {
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
			case "env", "labels":
				delete(typed, key)
			default:
				removeSecrets(item)
			}
		}
	}
}
