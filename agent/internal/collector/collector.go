package collector

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
)

type Snapshot struct {
	CollectedAt  time.Time             `json:"collectedAt"`
	CPUPercent   float64               `json:"cpuPercent"`
	RAM          MemorySnapshot        `json:"ram"`
	Disks        []DiskSnapshot        `json:"disks"`
	Network      []NetworkSnapshot     `json:"network"`
	Temperatures []TemperatureSnapshot `json:"temperatures"`
}

type MemorySnapshot struct {
	TotalBytes uint64 `json:"totalBytes"`
	UsedBytes  uint64 `json:"usedBytes"`
}

type DiskSnapshot struct {
	MountPoint  string  `json:"mountPoint"`
	UsedBytes   uint64  `json:"usedBytes"`
	TotalBytes  uint64  `json:"totalBytes"`
	UsedPercent float64 `json:"usedPercent"`
}

type NetworkSnapshot struct {
	Name      string `json:"name"`
	BytesSent uint64 `json:"bytesSent"`
	BytesRecv uint64 `json:"bytesRecv"`
}

type TemperatureSnapshot struct {
	Name        string  `json:"name"`
	Temperature float64 `json:"temperature"`
}

func Collect(ctx context.Context) (Snapshot, error) {
	snapshot := Snapshot{CollectedAt: time.Now().UTC()}

	cpuValues, err := cpu.PercentWithContext(ctx, time.Second, false)
	if err == nil && len(cpuValues) > 0 {
		snapshot.CPUPercent = cpuValues[0]
	}

	if virtualMemory, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		snapshot.RAM = MemorySnapshot{TotalBytes: virtualMemory.Total, UsedBytes: virtualMemory.Used}
	}

	if partitions, err := disk.PartitionsWithContext(ctx, false); err == nil {
		for _, partition := range partitions {
			if usage, err := disk.UsageWithContext(ctx, partition.Mountpoint); err == nil {
				snapshot.Disks = append(snapshot.Disks, DiskSnapshot{
					MountPoint: partition.Mountpoint, UsedBytes: usage.Used,
					TotalBytes: usage.Total, UsedPercent: usage.UsedPercent,
				})
			}
		}
	}

	if counters, err := net.IOCountersWithContext(ctx, true); err == nil {
		for _, counter := range counters {
			snapshot.Network = append(snapshot.Network, NetworkSnapshot{
				Name: counter.Name, BytesSent: counter.BytesSent, BytesRecv: counter.BytesRecv,
			})
		}
	}

	if readings, err := sensors.SensorsTemperatures(); err == nil {
		for _, reading := range readings {
			snapshot.Temperatures = append(snapshot.Temperatures, TemperatureSnapshot{
				Name: reading.SensorKey, Temperature: reading.Temperature,
			})
		}
	}
	return snapshot, nil
}
