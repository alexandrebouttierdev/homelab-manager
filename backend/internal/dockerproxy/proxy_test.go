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
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1.41/containers/0123456789ab/start", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("mutation status = %d, want %d", recorder.Code, http.StatusNotFound)
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
