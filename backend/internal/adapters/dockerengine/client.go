package dockerengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

const apiVersion = "v1.41"

// apiStatusError reports a non-2xx answer from the Docker API (through the
// restricted proxy), so callers can distinguish "gone" from "unavailable".
type apiStatusError struct {
	StatusCode int
	Status     string
}

func (e *apiStatusError) Error() string { return "Docker API returned " + e.Status }

func isNotFound(err error) bool {
	var statusErr *apiStatusError
	return errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound
}

type Client struct {
	http *http.Client
	// actions waits for a start, stop or restart, longer than a read.
	actions         *http.Client
	mu              sync.Mutex
	previousNetwork map[string]networkSnapshot
}

func NewClient(socketPath string) *Client {
	return newClient(&http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socketPath)
		},
	})
}

func newClient(transport http.RoundTripper) *Client {
	return &Client{
		http:            &http.Client{Transport: transport, Timeout: 10 * time.Second},
		actions:         &http.Client{Transport: transport, Timeout: domain.ContainerActionTimeout},
		previousNetwork: make(map[string]networkSnapshot),
	}
}

func (c *Client) CloseIdleConnections() {
	c.http.CloseIdleConnections()
}

func (c *Client) ListContainers(ctx context.Context) ([]domain.Container, error) {
	var summaries []containerSummary
	if err := c.get(ctx, "/containers/json?all=1", &summaries); err != nil {
		return nil, err
	}
	containers := make([]domain.Container, 0, len(summaries))
	for _, summary := range summaries {
		container := domain.Container{
			ID: uuid.New(), DockerID: summary.ID, Image: summary.Image,
			State: summary.State, Status: summary.Status,
			CreatedAt: time.Unix(summary.Created, 0).UTC(),
			Ports:     make([]map[string]any, 0, len(summary.Ports)),
			Networks:  make([]string, 0),
			Mounts:    make([]map[string]any, 0, len(summary.Mounts)),
		}
		if len(summary.Names) > 0 {
			container.Name = strings.TrimPrefix(summary.Names[0], "/")
		}
		for _, port := range summary.Ports {
			item := map[string]any{"privatePort": port.PrivatePort, "type": port.Type}
			if port.PublicPort > 0 {
				item["publicPort"] = port.PublicPort
			}
			if port.IP != "" {
				item["ip"] = port.IP
			}
			container.Ports = append(container.Ports, item)
		}
		for network := range summary.NetworkSettings.Networks {
			container.Networks = append(container.Networks, network)
		}
		for _, mount := range summary.Mounts {
			container.Mounts = append(container.Mounts, map[string]any{
				"type": mount.Type, "source": mount.Source, "destination": mount.Destination, "rw": mount.RW,
			})
		}
		details, err := c.inspect(ctx, summary.ID)
		if isNotFound(err) {
			// Removed between the listing and the inspection: the next
			// reconciliation records it as removed if it is really gone.
			continue
		}
		if err != nil {
			return nil, err
		}
		container.RestartCount = details.RestartCount
		container.RestartPolicy = details.HostConfig.RestartPolicy.Name
		if started, parseErr := time.Parse(time.RFC3339Nano, details.State.StartedAt); parseErr == nil && !started.IsZero() {
			container.StartedAt = &started
			if details.State.Running {
				if elapsed := time.Since(started); elapsed >= 0 {
					uptime := uint64(elapsed.Seconds())
					container.UptimeSeconds = &uptime
				}
			}
		}
		if details.Config.Image != "" {
			container.Image = details.Config.Image
		}
		containers = append(containers, container)
	}
	return containers, nil
}

func (c *Client) ContainerStats(ctx context.Context, dockerID string) ([]domain.MetricSample, error) {
	endpoint := "/containers/" + url.PathEscape(dockerID) + "/stats?stream=false"
	var stats statsResponse
	if err := c.get(ctx, endpoint, &stats); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	samples := make([]domain.MetricSample, 0, 20)
	add := func(name string, value float64, unit string) {
		samples = append(samples, domain.MetricSample{
			Name: name, Value: value, Unit: unit, RecordedAt: now,
		})
	}
	addLabeled := func(name string, value float64, unit string, labels map[string]string) {
		samples = append(samples, domain.MetricSample{
			Name: name, Value: value, Unit: unit, Labels: labels, RecordedAt: now,
		})
	}
	if value, ok := CPUPercent(stats); ok {
		add("cpu.percent", value, "percent")
	}
	memoryUsage, memoryLimit, memoryPercent, memoryAvailable, hasMemory := MemoryMetrics(stats)
	if hasMemory {
		add("memory.usage_bytes", memoryUsage, "bytes")
		if memoryLimit > 0 {
			add("memory.limit_bytes", memoryLimit, "bytes")
			add("memory.available_bytes", memoryAvailable, "bytes")
			add("memory.percent", memoryPercent, "percent")
		}
	}
	currentNetwork := make(map[string]networkCounters, len(stats.Networks))
	for interfaceName, network := range stats.Networks {
		labels := map[string]string{"interface": interfaceName}
		addLabeled("network.rx_bytes", float64(network.RxBytes), "bytes", labels)
		addLabeled("network.tx_bytes", float64(network.TxBytes), "bytes", labels)
		addLabeled("network.rx_packets", float64(network.RxPackets), "packets", labels)
		addLabeled("network.tx_packets", float64(network.TxPackets), "packets", labels)
		addLabeled("network.errors", float64(network.RxErrors+network.TxErrors), "count", labels)
		addLabeled("network.drops", float64(network.RxDropped+network.TxDropped), "count", labels)
		currentNetwork[interfaceName] = networkCounters{
			rxBytes: network.RxBytes, txBytes: network.TxBytes,
		}
	}
	if previous, ok := c.storeNetworkSnapshot(dockerID, now, currentNetwork); ok {
		elapsed := now.Sub(previous.at).Seconds()
		if elapsed > 0 {
			for name, current := range currentNetwork {
				before, exists := previous.values[name]
				if !exists {
					continue
				}
				labels := map[string]string{"interface": name}
				if current.rxBytes >= before.rxBytes {
					addLabeled("network.rx_bytes_per_second", float64(current.rxBytes-before.rxBytes)/elapsed, "bytes_per_second", labels)
				}
				if current.txBytes >= before.txBytes {
					addLabeled("network.tx_bytes_per_second", float64(current.txBytes-before.txBytes)/elapsed, "bytes_per_second", labels)
				}
			}
		}
	}
	for _, entry := range stats.Blkio.IOServiceBytesRecursive {
		labels := map[string]string{"device": fmt.Sprintf("%d:%d", entry.Major, entry.Minor)}
		switch strings.ToLower(entry.Op) {
		case "read":
			addLabeled("block.read_bytes", float64(entry.Value), "bytes", labels)
		case "write":
			addLabeled("block.write_bytes", float64(entry.Value), "bytes", labels)
		}
	}
	if stats.PidsStats.Current != nil {
		add("process.count", float64(*stats.PidsStats.Current), "count")
	}
	return samples, nil
}

type networkCounters struct {
	rxBytes uint64
	txBytes uint64
}

type networkSnapshot struct {
	at     time.Time
	values map[string]networkCounters
}

func (c *Client) storeNetworkSnapshot(id string, at time.Time, values map[string]networkCounters) (networkSnapshot, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous, found := c.previousNetwork[id]
	c.previousNetwork[id] = networkSnapshot{at: at, values: values}
	for containerID, snapshot := range c.previousNetwork {
		if at.Sub(snapshot.at) > time.Hour {
			delete(c.previousNetwork, containerID)
		}
	}
	return previous, found
}

func CPUPercent(stats statsResponse) (float64, bool) {
	if stats.CPUStats.CPUUsage.TotalUsage <= stats.PreCPUStats.CPUUsage.TotalUsage ||
		stats.CPUStats.SystemCPUUsage <= stats.PreCPUStats.SystemCPUUsage {
		return 0, false
	}
	cpuDelta := stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage
	systemDelta := stats.CPUStats.SystemCPUUsage - stats.PreCPUStats.SystemCPUUsage
	cpus := stats.CPUStats.OnlineCPUs
	if cpus == 0 {
		cpus = uint64(len(stats.CPUStats.CPUUsage.PercpuUsage))
	}
	if cpus == 0 {
		return 0, false
	}
	return 100 * float64(cpuDelta) / float64(systemDelta) * float64(cpus), true
}

func MemoryMetrics(stats statsResponse) (usage, limit, percent, available float64, ok bool) {
	usage = float64(stats.MemoryStats.Usage)
	limit = float64(stats.MemoryStats.Limit)
	if inactive, exists := stats.MemoryStats.Stats["total_inactive_file"]; exists {
		usage = max(usage-float64(inactive), 0)
	} else if inactive, exists := stats.MemoryStats.Stats["inactive_file"]; exists {
		usage = max(usage-float64(inactive), 0)
	}
	if stats.MemoryStats.Usage == 0 {
		return 0, 0, 0, 0, false
	}
	if limit > 0 {
		available = max(limit-usage, 0)
		percent = 100 * usage / limit
	}
	return usage, limit, percent, available, true
}

func (c *Client) inspect(ctx context.Context, id string) (containerInspect, error) {
	var result containerInspect
	err := c.get(ctx, "/containers/"+url.PathEscape(id)+"/json", &result)
	return result, err
}

func (c *Client) get(ctx context.Context, endpoint string, target any) error {
	if !strings.HasPrefix(endpoint, "/") || path.Clean(endpoint) != endpoint {
		return fmt.Errorf("invalid Docker API endpoint")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/"+apiVersion+endpoint, nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Docker API request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return &apiStatusError{StatusCode: response.StatusCode, Status: response.Status}
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode Docker API response: %w", err)
	}
	return nil
}

type containerSummary struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	Image   string   `json:"Image"`
	State   string   `json:"State"`
	Status  string   `json:"Status"`
	Created int64    `json:"Created"`
	Ports   []struct {
		IP          string `json:"IP"`
		PrivatePort uint16 `json:"PrivatePort"`
		PublicPort  uint16 `json:"PublicPort"`
		Type        string `json:"Type"`
	} `json:"Ports"`
	Mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]json.RawMessage `json:"Networks"`
	} `json:"NetworkSettings"`
}

type containerInspect struct {
	Created      string `json:"Created"`
	RestartCount uint64 `json:"RestartCount"`
	Config       struct {
		Image string `json:"Image"`
	} `json:"Config"`
	HostConfig struct {
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
	State struct {
		Running   bool   `json:"Running"`
		StartedAt string `json:"StartedAt"`
	} `json:"State"`
}

type statsResponse struct {
	Read     string `json:"read"`
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint64 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Limit uint64            `json:"limit"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		Name      string `json:"name"`
		RxBytes   uint64 `json:"rx_bytes"`
		TxBytes   uint64 `json:"tx_bytes"`
		RxPackets uint64 `json:"rx_packets"`
		TxPackets uint64 `json:"tx_packets"`
		RxErrors  uint64 `json:"rx_errors"`
		TxErrors  uint64 `json:"tx_errors"`
		RxDropped uint64 `json:"rx_dropped"`
		TxDropped uint64 `json:"tx_dropped"`
	} `json:"networks"`
	Blkio struct {
		IOServiceBytesRecursive []struct {
			Major uint64 `json:"major"`
			Minor uint64 `json:"minor"`
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
	PidsStats struct {
		Current *uint64 `json:"current"`
	} `json:"pids_stats"`
}
