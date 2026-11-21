package dockerengine

import "testing"

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
