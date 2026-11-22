package dockerengine

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCPUPercent(t *testing.T) {
	stats := statsResponse{}
	stats.CPUStats.CPUUsage.TotalUsage = 150
	stats.CPUStats.SystemCPUUsage = 300
	stats.CPUStats.OnlineCPUs = 2
	stats.PreCPUStats.CPUUsage.TotalUsage = 100
	stats.PreCPUStats.SystemCPUUsage = 200
	value, ok := CPUPercent(stats)
	if !ok || value != 100 {
		t.Fatalf("CPUPercent() = (%v, %t), want (100, true)", value, ok)
	}
	stats.CPUStats.SystemCPUUsage = stats.PreCPUStats.SystemCPUUsage
	if _, ok := CPUPercent(stats); ok {
		t.Fatal("CPUPercent() should reject a zero system delta")
	}
}

func TestMemoryMetrics(t *testing.T) {
	inactive := uint64(20)
	stats := statsResponse{}
	stats.MemoryStats.Usage = 100
	stats.MemoryStats.Limit = 200
	stats.MemoryStats.Stats = map[string]uint64{"inactive_file": inactive}
	usage, limit, percent, available, ok := MemoryMetrics(stats)
	if !ok || usage != 80 || limit != 200 || percent != 40 || available != 120 {
		t.Fatalf("MemoryMetrics() = (%v, %v, %v, %v, %t)", usage, limit, percent, available, ok)
	}
	stats.MemoryStats.Limit = 0
	usage, limit, percent, available, ok = MemoryMetrics(stats)
	if !ok || usage != 80 || limit != 0 || percent != 0 || available != 0 {
		t.Fatalf("unlimited memory metrics = (%v, %v, %v, %v, %t)", usage, limit, percent, available, ok)
	}
}

// fakeDockerAPI answers like the restricted proxy: listing, inspection and a
// 404 for a container removed between the two calls.
func fakeDockerAPI(t *testing.T, inspectStatus int) *Client {
	t.Helper()
	handler := http.NewServeMux()
	handler.HandleFunc("/v1.41/containers/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"Id":"aaa111","Names":["/kept"],"Image":"nginx","State":"running","Status":"Up","Created":1700000000},
			{"Id":"bbb222","Names":["/gone"],"Image":"redis","State":"exited","Status":"Exited","Created":1700000000}
		]`))
	})
	handler.HandleFunc("/v1.41/containers/aaa111/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"RestartCount":2,"Config":{"Image":"nginx:stable"},"HostConfig":{"RestartPolicy":{"Name":"always"}},"State":{"Running":true,"StartedAt":"2026-11-22T10:00:00Z"}}`))
	})
	handler.HandleFunc("/v1.41/containers/bbb222/json", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(inspectStatus)
		_, _ = w.Write([]byte(`{"message":"no such container"}`))
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	transport := &http.Transport{}
	transport.Proxy = func(*http.Request) (*url.URL, error) { return url.Parse(server.URL) }
	return newClient(transport)
}

func TestListContainersSkipsContainerRemovedBeforeInspection(t *testing.T) {
	containers, err := fakeDockerAPI(t, http.StatusNotFound).ListContainers(t.Context())
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	if len(containers) != 1 || containers[0].Name != "kept" {
		t.Fatalf("containers = %+v, want only the inspectable container", containers)
	}
	kept := containers[0]
	if kept.Image != "nginx:stable" || kept.RestartCount != 2 || kept.RestartPolicy != "always" || kept.StartedAt == nil {
		t.Fatalf("inspection details not applied: %+v", kept)
	}
}

func TestListContainersFailsOnUnexpectedInspectionError(t *testing.T) {
	_, err := fakeDockerAPI(t, http.StatusInternalServerError).ListContainers(t.Context())
	if err == nil || isNotFound(err) || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v, want a non-404 Docker API error", err)
	}
}
