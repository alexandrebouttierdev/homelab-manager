package dockerengine

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"homelab-manager/backend/internal/domain"
)

const (
	// maxLogBody is the bound of the proxy on an answer.
	maxLogBody = 8 << 20
	// logRetries is how many times a too large log is asked again with a
	// quarter of the lines.
	logRetries = 3
	// tooLargeHeader is set by the proxy on an answer above its bound
	// (dockerproxy.TooLargeHeader).
	tooLargeHeader = "X-Homelab-Too-Large"
)

var errLogTooLarge = errors.New("container log answer is too large")

// ContainerLogs reads the end of the log of a container through the proxy.
// When the answer exceeds the bound of the proxy, it asks again with fewer
// lines and marks the result truncated.
func (c *Client) ContainerLogs(ctx context.Context, dockerID string, query domain.ContainerLogQuery) (domain.ContainerLogs, error) {
	if err := query.Validate(); err != nil {
		return domain.ContainerLogs{}, err
	}
	tail := query.Tail
	truncated := false
	for attempt := 0; ; attempt++ {
		logs, err := c.readLogs(ctx, dockerID, tail, query.Since)
		if errors.Is(err, errLogTooLarge) && attempt < logRetries && tail > 1 {
			tail = max(tail/4, 1)
			truncated = true
			continue
		}
		if err != nil {
			return domain.ContainerLogs{}, err
		}
		logs.Truncated = logs.Truncated || truncated
		return logs, nil
	}
}

func (c *Client) readLogs(ctx context.Context, dockerID string, tail int, since *time.Time) (domain.ContainerLogs, error) {
	values := url.Values{}
	values.Set("stdout", "1")
	values.Set("stderr", "1")
	values.Set("timestamps", "1")
	values.Set("tail", strconv.Itoa(tail))
	if since != nil {
		values.Set("since", strconv.FormatInt(since.Unix(), 10))
	}
	endpoint := "http://docker/" + apiVersion + "/containers/" + url.PathEscape(dockerID) + "/logs?" + values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domain.ContainerLogs{}, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return domain.ContainerLogs{}, fmt.Errorf("%w: %v", domain.ErrDockerUnavailable, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxLogBody+1))
	if err != nil {
		return domain.ContainerLogs{}, fmt.Errorf("%w: read log: %v", domain.ErrDockerUnavailable, err)
	}
	switch {
	case response.Header.Get(tooLargeHeader) == "1":
		return domain.ContainerLogs{}, errLogTooLarge
	case response.StatusCode == http.StatusNotFound:
		return domain.ContainerLogs{}, domain.ErrContainerRemoved
	case response.StatusCode != http.StatusOK:
		return domain.ContainerLogs{}, fmt.Errorf("%w: %s", domain.ErrDockerUnavailable, response.Status)
	case len(body) > maxLogBody:
		return domain.ContainerLogs{}, errLogTooLarge
	}
	raw := response.Header.Get("Content-Type") == "application/vnd.docker.raw-stream"
	return domain.ContainerLogs{Lines: ParseLogStream(body, raw)}, nil
}

// ParseLogStream splits a Docker log answer into lines. A multiplexed
// stream (container without TTY) is a series of frames: one byte for the
// stream, three zero bytes, the length in four big-endian bytes, then the
// data. A raw stream (container with a TTY) is stdout only. Each line
// starts with the RFC 3339 time Docker adds with timestamps=1.
func ParseLogStream(body []byte, raw bool) []domain.ContainerLogLine {
	if !raw && !looksMultiplexed(body) {
		raw = true
	}
	var lines []domain.ContainerLogLine
	if raw {
		return appendLines(lines, domain.ContainerStdout, body, true)
	}
	pending := map[domain.ContainerLogStream][]byte{}
	for len(body) >= 8 {
		stream := domain.ContainerStdout
		if body[0] == 2 {
			stream = domain.ContainerStderr
		}
		size := int(binary.BigEndian.Uint32(body[4:8]))
		body = body[8:]
		if size > len(body) {
			size = len(body)
		}
		data := append(pending[stream], body[:size]...)
		body = body[size:]
		// A frame may end inside a line: the rest waits for the next frame
		// of the same stream.
		cut := bytes.LastIndexByte(data, '\n')
		if cut < 0 {
			pending[stream] = data
			continue
		}
		lines = appendLines(lines, stream, data[:cut+1], false)
		pending[stream] = append([]byte(nil), data[cut+1:]...)
	}
	for _, stream := range []domain.ContainerLogStream{domain.ContainerStdout, domain.ContainerStderr} {
		lines = appendLines(lines, stream, pending[stream], true)
	}
	return lines
}

// looksMultiplexed checks the header of the first frame.
func looksMultiplexed(body []byte) bool {
	if len(body) < 8 {
		return len(body) == 0
	}
	return body[0] <= 2 && body[1] == 0 && body[2] == 0 && body[3] == 0
}

func appendLines(lines []domain.ContainerLogLine, stream domain.ContainerLogStream, data []byte, keepLast bool) []domain.ContainerLogLine {
	parts := strings.Split(string(data), "\n")
	if !keepLast || parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	for _, part := range parts {
		lines = append(lines, parseLogLine(stream, strings.TrimSuffix(part, "\r")))
	}
	return lines
}

func parseLogLine(stream domain.ContainerLogStream, text string) domain.ContainerLogLine {
	line := domain.ContainerLogLine{Stream: stream, Message: text}
	if stamp, rest, found := strings.Cut(text, " "); found {
		if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
			at = at.UTC()
			line.Time, line.Message = &at, rest
		}
	}
	if len(line.Message) > domain.MaxContainerLogLineBytes {
		cut := domain.MaxContainerLogLineBytes
		for cut > 0 && !utf8.RuneStart(line.Message[cut]) {
			cut--
		}
		line.Message, line.Cut = line.Message[:cut], true
	}
	if !utf8.ValidString(line.Message) {
		line.Message = strings.ToValidUTF8(line.Message, "�")
	}
	return line
}

// ActOnContainer asks the proxy to start, stop or restart a container and
// waits for the answer of Docker.
func (c *Client) ActOnContainer(ctx context.Context, dockerID string, verb domain.ContainerVerb) error {
	if !verb.Valid() {
		return domain.ErrInvalidContainerVerb
	}
	endpoint := "http://docker/" + apiVersion + "/containers/" + url.PathEscape(dockerID) + "/" + string(verb)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := c.actions.Do(request)
	if err != nil {
		failure := domain.ContainerActionUnavailable
		if errors.Is(err, context.DeadlineExceeded) {
			failure = domain.ContainerActionTimedOut
		}
		return &domain.ContainerActionError{Failure: failure, Err: err}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	switch response.StatusCode {
	case http.StatusNoContent, http.StatusNotModified:
		// 304: already started or already stopped, which is the state
		// asked for.
		return nil
	case http.StatusNotFound:
		return &domain.ContainerActionError{Failure: domain.ContainerActionNotFound, Err: errors.New(response.Status)}
	case http.StatusForbidden:
		return &domain.ContainerActionError{Failure: domain.ContainerActionNotAllowed, Err: errors.New(response.Status)}
	case http.StatusBadGateway, http.StatusServiceUnavailable:
		return &domain.ContainerActionError{Failure: domain.ContainerActionUnavailable, Err: errors.New(response.Status)}
	default:
		return &domain.ContainerActionError{Failure: domain.ContainerActionRejected, Err: errors.New(response.Status)}
	}
}
