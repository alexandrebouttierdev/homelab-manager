package dockerproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestAllowedPath(t *testing.T) {
	tests := []struct {
		path string
		ok   bool
	}{
		{"/v1.41/containers/json?all=1", true},
		{"/v1.41/containers/0123456789ab/json", true},
		{"/v1.41/containers/0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef/stats?stream=false", true},
		{"/v1.41/containers/json?all=1&filters=%7B%7D", false},
		{"/v1.41/containers/0123456789ab/start", false},
		{"/v1.41/containers/0123456789ab/logs?stdout=1&stderr=1&timestamps=1&tail=10", false},
		{"/v1.41/containers/../events", false},
		{"/v1.41/containers/0123456789ab/stats?stream=true", false},
		{"/v1.41/images/json", true},
		{"/v1.41/volumes", true},
		{"/v1.41/networks", true},
		{"/v1.41/images/json?all=1", false},
		{"/v1.41/images/json?filters=%7B%7D", false},
		{"/v1.41/volumes?filters=%7B%7D", false},
		{"/v1.41/networks/net-front", false},
		{"/v1.41/volumes/web-data", false},
		{"/v1.41/images/nginx/json", false},
		{"/v1.41/images/create", false},
		{"/v1.41/volumes/prune", false},
		{"/v1.41/networks/prune", false},
		{"/v1.41/system/df", false},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		if got := allowedPath(request.URL); got != test.ok {
			t.Errorf("allowedPath(%q) = %t, want %t", test.path, got, test.ok)
		}
	}
}

func TestProxyRejectsMutationsAndRemovesSecrets(t *testing.T) {
	proxy := New("/unused")
	proxy.client.Transport = roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1.41/containers/0123456789ab/json" {
			t.Fatalf("unexpected upstream path: %s", request.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"Config":{"Env":["TOKEN=secret"],"Labels":{"private":"value"},"Image":"image"}}`)),
		}, nil
	})

	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1.41/containers/prune", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("mutation status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	// A read-only proxy names the refusal of an action, so the Manager can
	// tell it from an unknown container.
	recorder = httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1.41/containers/0123456789ab/start", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("action status = %d, want %d", recorder.Code, http.StatusForbidden)
	}

	recorder = httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1.41/containers/0123456789ab/json", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("inspect status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if strings.Contains(recorder.Body.String(), "secret") || strings.Contains(recorder.Body.String(), "private") {
		t.Fatalf("proxy returned sensitive fields: %s", recorder.Body.String())
	}
}

func TestProxyRemovesDriverOptions(t *testing.T) {
	proxy := New("/unused")
	proxy.client.Transport = roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		body := `{"Volumes":[{"Name":"share","Driver":"local","Options":{"type":"cifs","o":"username=u,password=hunter2"}}]}`
		if request.URL.Path == "/v1.41/networks" {
			body = `[{"Name":"front","Options":{"com.docker.network.secret":"hunter2"},"IPAM":{"Options":{"x":"hunter2"},"Config":[]}}]`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	for _, path := range []string{"/v1.41/volumes", "/v1.41/networks"} {
		recorder := httptest.NewRecorder()
		proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "hunter2") || !strings.Contains(recorder.Body.String(), "Name") {
			t.Fatalf("GET %s = %d %s", path, recorder.Code, recorder.Body)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		recorder := httptest.NewRecorder()
		proxy.ServeHTTP(recorder, httptest.NewRequest(method, "/v1.41/volumes", nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s /volumes = %d, want 404", method, recorder.Code)
		}
	}
}

func TestLogsAndActionPaths(t *testing.T) {
	id := "0123456789ab"
	logs := "/v1.41/containers/" + id + "/logs?"
	for path, want := range map[string]bool{
		logs + "stdout=1&stderr=1&timestamps=1&tail=200":                          true,
		logs + "stdout=1&stderr=1&timestamps=1&tail=2000&since=1795600000":        true,
		logs + "stdout=1&stderr=1&timestamps=1&tail=2001":                         false,
		logs + "stdout=1&stderr=1&timestamps=1&tail=0":                            false,
		logs + "stdout=1&stderr=1&timestamps=1&tail=all":                          false,
		logs + "stdout=1&stderr=1&timestamps=1&tail=010":                          false,
		logs + "stdout=1&stderr=1&timestamps=1":                                   false,
		logs + "stdout=1&stderr=1&timestamps=1&tail=10&follow=1":                  false,
		logs + "stdout=1&stderr=1&timestamps=1&tail=10&tail=20":                   false,
		logs + "stdout=1&stderr=1&timestamps=1&tail=10&since=-1":                  false,
		logs + "stdout=1&stderr=0&timestamps=1&tail=10":                           false,
		"/v1.41/containers/not-an-id/logs?stdout=1&stderr=1&timestamps=1&tail=10": false,
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if got := logsPath(request.URL); got != want {
			t.Errorf("logsPath(%q) = %t, want %t", path, got, want)
		}
	}
	for path, want := range map[string]bool{
		"/v1.41/containers/" + id + "/start":       true,
		"/v1.41/containers/" + id + "/stop":        true,
		"/v1.41/containers/" + id + "/restart":     true,
		"/v1.41/containers/" + id + "/stop?t=0":    false,
		"/v1.41/containers/" + id + "/kill":        false,
		"/v1.41/containers/" + id + "/exec":        false,
		"/v1.41/containers/" + id + "/pause":       false,
		"/v1.41/containers/" + id:                  false,
		"/v1.41/containers/" + id + "/start/extra": false,
		"/v1.41/containers/prune":                  false,
		"/v1.41/containers/" + id + "/../x/start":  false,
	} {
		request := httptest.NewRequest(http.MethodPost, path, nil)
		if got := actionPath(request.URL); got != want {
			t.Errorf("actionPath(%q) = %t, want %t", path, got, want)
		}
	}
}

func TestProxyRefusesActionsUnlessEnabled(t *testing.T) {
	calls := 0
	upstream := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Method != http.MethodPost || request.URL.Path != "/v1.41/containers/0123456789ab/restart" || request.URL.RawQuery != "" {
			t.Fatalf("unexpected upstream request: %s %s", request.Method, request.URL)
		}
		return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	readOnly := New("/unused")
	readOnly.actions.Transport = upstream
	recorder := httptest.NewRecorder()
	readOnly.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1.41/containers/0123456789ab/restart", nil))
	if recorder.Code != http.StatusForbidden || calls != 0 {
		t.Fatalf("read-only proxy answered %d after %d calls", recorder.Code, calls)
	}

	enabled := NewWithOptions("/unused", Options{ContainerActions: true})
	enabled.actions.Transport = upstream
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1.41/containers/0123456789ab/restart", nil))
	if recorder.Code != http.StatusNoContent || calls != 1 {
		t.Fatalf("enabled proxy answered %d after %d calls", recorder.Code, calls)
	}
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1.41/containers/0123456789ab/kill", nil))
	if recorder.Code != http.StatusNotFound || calls != 1 {
		t.Fatalf("kill answered %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1.41/containers/0123456789ab/restart", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("GET restart answered %d", recorder.Code)
	}
}

func TestProxyForwardsLogsAndRefusesLargeOnes(t *testing.T) {
	body := "\x01\x00\x00\x00\x00\x00\x00\x05hello"
	proxy := New("/unused")
	proxy.client.Transport = roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		header := make(http.Header)
		header.Set("Content-Type", "application/vnd.docker.multiplexed-stream")
		if strings.Contains(request.URL.RawQuery, "tail=2000") {
			return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", maxResponseBytes+1)))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1.41/containers/0123456789ab/logs?stdout=1&stderr=1&timestamps=1&tail=10", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != body ||
		recorder.Header().Get("Content-Type") != "application/vnd.docker.multiplexed-stream" {
		t.Fatalf("logs = %d %q %q", recorder.Code, recorder.Body.String(), recorder.Header().Get("Content-Type"))
	}
	recorder = httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1.41/containers/0123456789ab/logs?stdout=1&stderr=1&timestamps=1&tail=2000", nil))
	if recorder.Code != http.StatusBadGateway || recorder.Header().Get(TooLargeHeader) != "1" {
		t.Fatalf("large logs = %d, header %q", recorder.Code, recorder.Header().Get(TooLargeHeader))
	}
}
