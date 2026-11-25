package dockerengine

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func frame(stream byte, data string) string {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	return string(header) + data
}

func TestParseLogStream(t *testing.T) {
	body := frame(1, "2026-11-25T08:00:00.123456789Z started\n") +
		frame(2, "2026-11-25T08:00:01Z warn: part") +
		frame(1, "2026-11-25T08:00:02Z second\n") +
		frame(2, "ial line\n") +
		frame(1, "no time\n")
	lines := ParseLogStream([]byte(body), false)
	if len(lines) != 4 {
		t.Fatalf("lines = %+v", lines)
	}
	if lines[0].Stream != domain.ContainerStdout || lines[0].Message != "started" || lines[0].Time == nil ||
		lines[0].Time.Nanosecond() != 123456789 {
		t.Fatalf("first line = %+v", lines[0])
	}
	if lines[2].Stream != domain.ContainerStderr || lines[2].Message != "warn: partial line" {
		t.Fatalf("a line split over two frames = %+v", lines[2])
	}
	if lines[3].Time != nil || lines[3].Message != "no time" {
		t.Fatalf("line without time = %+v", lines[3])
	}

	tty := ParseLogStream([]byte("2026-11-25T08:00:00Z a\r\n2026-11-25T08:00:01Z b"), true)
	if len(tty) != 2 || tty[0].Message != "a" || tty[1].Message != "b" || tty[1].Stream != domain.ContainerStdout {
		t.Fatalf("raw stream = %+v", tty)
	}
	// Text without frame header is read as a raw stream.
	if plain := ParseLogStream([]byte("hello\n"), false); len(plain) != 1 || plain[0].Message != "hello" {
		t.Fatalf("plain = %+v", plain)
	}
	long := ParseLogStream([]byte(frame(1, strings.Repeat("é", domain.MaxContainerLogLineBytes)+"\n")), false)
	if len(long) != 1 || !long[0].Cut || len(long[0].Message) > domain.MaxContainerLogLineBytes || !strings.HasPrefix(long[0].Message, "é") {
		t.Fatalf("long line cut = %t, %d bytes", long[0].Cut, len(long[0].Message))
	}
	if invalid := ParseLogStream([]byte(frame(1, "bad \xff byte\n")), false); invalid[0].Message != "bad � byte" {
		t.Fatalf("invalid UTF-8 = %q", invalid[0].Message)
	}
}

func TestContainerLogsAsksFewerLinesWhenTooLarge(t *testing.T) {
	var tails []string
	client := newClient(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		query := request.URL.Query()
		tails = append(tails, query.Get("tail"))
		if query.Get("stdout") != "1" || query.Get("stderr") != "1" || query.Get("timestamps") != "1" || query.Get("since") != "1795593600" {
			t.Fatalf("query = %s", request.URL.RawQuery)
		}
		header := make(http.Header)
		if query.Get("tail") == "2000" {
			header.Set(tooLargeHeader, "1")
			return &http.Response{StatusCode: http.StatusBadGateway, Header: header, Body: io.NopCloser(strings.NewReader("too large"))}, nil
		}
		header.Set("Content-Type", "application/vnd.docker.multiplexed-stream")
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(frame(1, "line\n")))}, nil
	}))
	since := time.Unix(1795593600, 0)
	logs, err := client.ContainerLogs(context.Background(), "0123456789ab", domain.ContainerLogQuery{Tail: 2000, Since: &since})
	if err != nil || !logs.Truncated || len(logs.Lines) != 1 || strings.Join(tails, ",") != "2000,500" {
		t.Fatalf("logs = %+v, %v, tails %v", logs, err, tails)
	}
	if _, err := client.ContainerLogs(context.Background(), "0123456789ab", domain.ContainerLogQuery{Tail: 0}); !errors.Is(err, domain.ErrInvalidLogQuery) {
		t.Fatalf("tail 0 = %v", err)
	}
}

func TestActOnContainerMapsTheAnswers(t *testing.T) {
	for status, want := range map[int]domain.ContainerActionFailure{
		http.StatusNoContent:           "",
		http.StatusNotModified:         "",
		http.StatusNotFound:            domain.ContainerActionNotFound,
		http.StatusForbidden:           domain.ContainerActionNotAllowed,
		http.StatusBadGateway:          domain.ContainerActionUnavailable,
		http.StatusInternalServerError: domain.ContainerActionRejected,
	} {
		client := newClient(roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodPost || request.URL.Path != "/v1.41/containers/0123456789ab/stop" || request.URL.RawQuery != "" {
				t.Fatalf("request = %s %s", request.Method, request.URL)
			}
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
		}))
		err := client.ActOnContainer(context.Background(), "0123456789ab", domain.ContainerStop)
		var actionErr *domain.ContainerActionError
		switch {
		case want == "" && err != nil:
			t.Fatalf("status %d: %v", status, err)
		case want != "" && (!errors.As(err, &actionErr) || actionErr.Failure != want):
			t.Fatalf("status %d: %v, want %s", status, err, want)
		}
	}
	client := newClient(roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unused") }))
	if err := client.ActOnContainer(context.Background(), "0123456789ab", "kill"); !errors.Is(err, domain.ErrInvalidContainerVerb) {
		t.Fatalf("kill = %v", err)
	}
}
