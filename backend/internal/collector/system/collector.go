package system

import (
	"context"
	"fmt"
	"math"
	"net"
	"strings"
	"sync"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	netstat "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
	"golang.org/x/sys/unix"
)

type Snapshot struct {
	Host    domain.HostInfo
	Samples []domain.MetricSample
}

type Collector struct {
	now          func() time.Time
	mu           sync.Mutex
	previousAt   time.Time
	previousNet  map[string]netstat.IOCountersStat
	previousDisk map[string]disk.IOCountersStat
}

func New() *Collector {
	return &Collector{
		now: time.Now, previousNet: make(map[string]netstat.IOCountersStat),
		previousDisk: make(map[string]disk.IOCountersStat),
	}
}

func (c *Collector) Collect(ctx context.Context) (domain.HostInfo, []domain.MetricSample, error) {
	if err := ctx.Err(); err != nil {
		return domain.HostInfo{}, nil, err
	}
	now := c.now().UTC()
	snapshot := Snapshot{Samples: make([]domain.MetricSample, 0, 64)}
	appendSample := func(name string, value float64, unit string, labels map[string]string) {
		if !math.IsNaN(value) && !math.IsInf(value, 0) {
			snapshot.Samples = append(snapshot.Samples, domain.MetricSample{
				ID: uuid.New(), Name: name, Value: value, Unit: unit, Labels: labels, RecordedAt: now,
			})
		}
	}

	info, err := host.InfoWithContext(ctx)
	if err != nil {
		return domain.HostInfo{}, nil, fmt.Errorf("read host information: %w", err)
	}
	snapshot.Host = domain.HostInfo{
		Hostname: info.Hostname, Distribution: info.Platform, OSVersion: info.PlatformVersion,
		KernelVersion: info.KernelVersion,
	}
	if info.BootTime > 0 {
		boot := time.Unix(int64(info.BootTime), 0).UTC()
		snapshot.Host.BootTime = &boot
	}
	if info.Uptime > 0 {
		uptime := info.Uptime
		snapshot.Host.UptimeSeconds = &uptime
	}
	if cores, err := cpu.InfoWithContext(ctx); err == nil && len(cores) > 0 {
		appendSample("cpu.cores", float64(len(cores)), "count", nil)
	}
	if percentages, err := cpu.PercentWithContext(ctx, time.Second, false); err == nil && len(percentages) > 0 {
		appendSample("cpu.percent", percentages[0], "percent", map[string]string{"core": "all"})
	}
	if percentages, err := cpu.PercentWithContext(ctx, time.Second, true); err == nil {
		for i, value := range percentages {
			appendSample("cpu.percent", value, "percent", map[string]string{"core": fmt.Sprintf("%d", i)})
		}
	}
	if average, err := load.AvgWithContext(ctx); err == nil {
		appendSample("cpu.load1", average.Load1, "processes", nil)
		appendSample("cpu.load5", average.Load5, "processes", nil)
		appendSample("cpu.load15", average.Load15, "processes", nil)
	}
	if memory, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		appendSample("memory.total", float64(memory.Total), "bytes", nil)
		if memory.Total > 0 && memory.Available <= memory.Total {
			appendSample("memory.used", float64(memory.Total-memory.Available), "bytes", nil)
			appendSample("memory.available", float64(memory.Available), "bytes", nil)
		}
		appendSample("memory.cached", float64(memory.Cached+memory.Buffers), "bytes", nil)
	}
	if swap, err := mem.SwapMemoryWithContext(ctx); err == nil {
		appendSample("swap.total", float64(swap.Total), "bytes", nil)
		appendSample("swap.used", float64(swap.Used), "bytes", nil)
	}
	if partitions, err := disk.PartitionsWithContext(ctx, false); err == nil {
		for _, partition := range partitions {
			usage, err := disk.UsageWithContext(ctx, partition.Mountpoint)
			if err != nil || usage.Total == 0 {
				continue
			}
			var filesystem unix.Statfs_t
			if err := unix.Statfs(partition.Mountpoint, &filesystem); err != nil || filesystem.Bsize <= 0 {
				continue
			}
			available := uint64(filesystem.Bavail) * uint64(filesystem.Bsize)
			if available > usage.Total {
				available = usage.Total
			}
			used := usage.Total - available
			labels := map[string]string{"mountpoint": partition.Mountpoint}
			appendSample("filesystem.total", float64(usage.Total), "bytes", labels)
			appendSample("filesystem.used", float64(used), "bytes", labels)
			appendSample("filesystem.available", float64(available), "bytes", labels)
			appendSample("filesystem.percent", 100*float64(used)/float64(usage.Total), "percent", labels)
		}
	}
	var networkCounters []netstat.IOCountersStat
	if counters, err := netstat.IOCountersWithContext(ctx, true); err == nil {
		networkCounters = counters
		for _, counter := range counters {
			labels := map[string]string{"interface": counter.Name}
			appendSample("network.rx_bytes", float64(counter.BytesRecv), "bytes", labels)
			appendSample("network.tx_bytes", float64(counter.BytesSent), "bytes", labels)
			appendSample("network.rx_packets", float64(counter.PacketsRecv), "packets", labels)
			appendSample("network.tx_packets", float64(counter.PacketsSent), "packets", labels)
			appendSample("network.errors", float64(counter.Errin+counter.Errout), "count", labels)
			appendSample("network.drops", float64(counter.Dropin+counter.Dropout), "count", labels)
		}
	}
	var diskCounters map[string]disk.IOCountersStat
	if counters, err := disk.IOCountersWithContext(ctx); err == nil {
		diskCounters = counters
		for device, counter := range counters {
			labels := map[string]string{"device": device}
			appendSample("disk.read_bytes", float64(counter.ReadBytes), "bytes", labels)
			appendSample("disk.write_bytes", float64(counter.WriteBytes), "bytes", labels)
		}
	}
	if interfaces, err := net.Interfaces(); err == nil {
		for _, iface := range interfaces {
			addresses, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addresses {
				appendSample("network.interface", 1, "present", map[string]string{
					"name": iface.Name, "address": strings.Split(addr.String(), "/")[0], "mac": iface.HardwareAddr.String(),
				})
			}
		}
	}
	if readings, err := sensors.SensorsTemperatures(); err == nil {
		for _, reading := range readings {
			appendSample("temperature.celsius", reading.Temperature, "celsius", map[string]string{"sensor": reading.SensorKey})
		}
	}
	if err := ctx.Err(); err != nil {
		return domain.HostInfo{}, nil, err
	}
	c.appendCounterRates(now, networkCounters, diskCounters, appendSample)
	return snapshot.Host, snapshot.Samples, nil
}

func (c *Collector) appendCounterRates(
	now time.Time,
	networkCounters []netstat.IOCountersStat,
	diskCounters map[string]disk.IOCountersStat,
	appendSample func(string, float64, string, map[string]string),
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	seconds := now.Sub(c.previousAt).Seconds()
	if seconds > 0 && !c.previousAt.IsZero() {
		for _, current := range networkCounters {
			previous, exists := c.previousNet[current.Name]
			if !exists {
				continue
			}
			labels := map[string]string{"interface": current.Name}
			if current.BytesRecv >= previous.BytesRecv {
				appendSample("network.rx_bytes_per_second", float64(current.BytesRecv-previous.BytesRecv)/seconds, "bytes_per_second", labels)
			}
			if current.BytesSent >= previous.BytesSent {
				appendSample("network.tx_bytes_per_second", float64(current.BytesSent-previous.BytesSent)/seconds, "bytes_per_second", labels)
			}
		}
		for device, current := range diskCounters {
			previous, exists := c.previousDisk[device]
			if !exists {
				continue
			}
			labels := map[string]string{"device": device}
			if current.ReadCount >= previous.ReadCount {
				reads := current.ReadCount - previous.ReadCount
				appendSample("disk.read_iops", float64(reads)/seconds, "iops", labels)
				if reads > 0 && current.ReadTime >= previous.ReadTime {
					appendSample("disk.read_latency_ms", float64(current.ReadTime-previous.ReadTime)/float64(reads), "milliseconds", labels)
				}
			}
			if current.WriteCount >= previous.WriteCount {
				writes := current.WriteCount - previous.WriteCount
				appendSample("disk.write_iops", float64(writes)/seconds, "iops", labels)
				if writes > 0 && current.WriteTime >= previous.WriteTime {
					appendSample("disk.write_latency_ms", float64(current.WriteTime-previous.WriteTime)/float64(writes), "milliseconds", labels)
				}
			}
		}
	}
	c.previousAt = now
	c.previousNet = make(map[string]netstat.IOCountersStat, len(networkCounters))
	for _, counter := range networkCounters {
		c.previousNet[counter.Name] = counter
	}
	c.previousDisk = make(map[string]disk.IOCountersStat, len(diskCounters))
	for device, counter := range diskCounters {
		c.previousDisk[device] = counter
	}
}
