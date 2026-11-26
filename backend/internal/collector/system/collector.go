package system

import (
	"context"
	"fmt"
	"math"
	"net"
	"strings"
	"sync"
	"time"

	"monoops/backend/internal/domain"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	netstat "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
)

type Snapshot struct {
	Host    domain.HostInfo
	Samples []domain.MetricSample
}

// Collector reads local Linux interfaces. Rates and CPU usage are computed
// from the counter deltas between two successive Collect calls, so the first
// cycle after start produces no rate or CPU percentage.
type Collector struct {
	now          func() time.Time
	mu           sync.Mutex
	previousAt   time.Time
	previousNet  map[string]netstat.IOCountersStat
	previousDisk map[string]disk.IOCountersStat
	previousCPU  map[string]cpu.TimesStat
	// temperatures reads the sensors; they are read every
	// domain.TemperatureInterval only, at lastTemperatureAt last.
	temperatures      func() ([]sensors.TemperatureStat, error)
	lastTemperatureAt time.Time
}

func New() *Collector {
	return &Collector{
		now: time.Now, previousNet: make(map[string]netstat.IOCountersStat),
		previousDisk: make(map[string]disk.IOCountersStat),
		previousCPU:  make(map[string]cpu.TimesStat),
		temperatures: sensors.SensorsTemperatures,
	}
}

// temperatureTolerance lets a cycle that comes a little early still read
// the sensors: with a 10 s interval, every sixth cycle reads them.
const temperatureTolerance = time.Second

// dueTemperatures tells whether the sensors must be read at now, and then
// records the reading.
func (c *Collector) dueTemperatures(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.lastTemperatureAt.IsZero() && now.Sub(c.lastTemperatureAt) < domain.TemperatureInterval-temperatureTolerance {
		return false
	}
	c.lastTemperatureAt = now
	return true
}

// sensorLabels gives each reading its own sensor label: several sensors can
// share a key (for example the fans and probes of dell_smm), the second one
// becomes key_2, the third key_3, in the order of the kernel.
func sensorLabels(readings []sensors.TemperatureStat) []string {
	labels := make([]string, len(readings))
	seen := make(map[string]int, len(readings))
	for i, reading := range readings {
		seen[reading.SensorKey]++
		labels[i] = reading.SensorKey
		if count := seen[reading.SensorKey]; count > 1 {
			labels[i] = fmt.Sprintf("%s_%d", reading.SensorKey, count)
		}
	}
	return labels
}

// cpuUsagePercent applies 100 × (Δtotal − Δidle) / Δtotal, where idle includes
// iowait. Guest time is already accounted in user/nice by the kernel and is
// therefore not added to the total.
func cpuUsagePercent(previous, current cpu.TimesStat) (float64, bool) {
	total := func(t cpu.TimesStat) float64 {
		return t.User + t.Nice + t.System + t.Idle + t.Iowait + t.Irq + t.Softirq + t.Steal
	}
	deltaTotal := total(current) - total(previous)
	deltaIdle := (current.Idle + current.Iowait) - (previous.Idle + previous.Iowait)
	if deltaTotal <= 0 || deltaIdle < 0 || deltaIdle > deltaTotal {
		return 0, false
	}
	return 100 * (deltaTotal - deltaIdle) / deltaTotal, true
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
				Name: name, Value: value, Unit: unit, Labels: labels, RecordedAt: now,
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
	cpuTimes := make([]cpu.TimesStat, 0, 64)
	if total, err := cpu.TimesWithContext(ctx, false); err == nil && len(total) == 1 {
		total[0].CPU = "cpu-total"
		cpuTimes = append(cpuTimes, total[0])
	}
	if perCore, err := cpu.TimesWithContext(ctx, true); err == nil {
		cpuTimes = append(cpuTimes, perCore...)
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
			// usage.Free is f_bavail × f_bsize: space available to the service user.
			available := usage.Free
			if available > usage.Total {
				available = usage.Total
			}
			used := usage.Total - available
			// device identifies the volume: several mountpoints (btrfs
			// subvolumes, bind mounts) can share one, and totals must count
			// it once.
			labels := map[string]string{"mountpoint": partition.Mountpoint, "device": partition.Device}
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
	if c.dueTemperatures(now) {
		if readings, err := c.temperatures(); err == nil {
			for i, label := range sensorLabels(readings) {
				appendSample(domain.TemperatureMetric, readings[i].Temperature, "celsius", map[string]string{"sensor": label})
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return domain.HostInfo{}, nil, err
	}
	c.appendCPUUsage(cpuTimes, appendSample)
	c.appendCounterRates(now, networkCounters, diskCounters, appendSample)
	return snapshot.Host, snapshot.Samples, nil
}

func (c *Collector) appendCPUUsage(times []cpu.TimesStat, appendSample func(string, float64, string, map[string]string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	current := make(map[string]cpu.TimesStat, len(times))
	for _, sample := range times {
		current[sample.CPU] = sample
		previous, exists := c.previousCPU[sample.CPU]
		if !exists {
			continue
		}
		value, ok := cpuUsagePercent(previous, sample)
		if !ok {
			continue
		}
		core := "all"
		if sample.CPU != "cpu-total" {
			core = strings.TrimPrefix(sample.CPU, "cpu")
		}
		appendSample("cpu.percent", value, "percent", map[string]string{"core": core})
	}
	c.previousCPU = current
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
