package system

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"monoops/backend/internal/domain"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	netstat "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
)

func TestCollectReturnsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := New().Collect(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context cancellation", err)
	}
}

func TestAppendCounterRatesCalculatesAndSkipsResets(t *testing.T) {
	start := time.Unix(100, 0)
	collector := &Collector{
		previousAt: start,
		previousNet: map[string]netstat.IOCountersStat{
			"eth0": {Name: "eth0", BytesRecv: 100, BytesSent: 300},
		},
		previousDisk: map[string]disk.IOCountersStat{
			"sda": {ReadCount: 10, ReadTime: 40, WriteCount: 5, WriteTime: 30},
		},
	}
	values := make(map[string]float64)
	collector.appendCounterRates(
		start.Add(2*time.Second),
		[]netstat.IOCountersStat{{Name: "eth0", BytesRecv: 300, BytesSent: 100}},
		map[string]disk.IOCountersStat{
			"sda": {ReadCount: 12, ReadTime: 46, WriteCount: 8, WriteTime: 36},
		},
		func(name string, value float64, _ string, _ map[string]string) { values[name] = value },
	)
	for name, want := range map[string]float64{
		"network.rx_bytes_per_second": 100,
		"disk.read_iops":              1,
		"disk.read_latency_ms":        3,
		"disk.write_iops":             1.5,
		"disk.write_latency_ms":       2,
	} {
		if got := values[name]; got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	if _, ok := values["network.tx_bytes_per_second"]; ok {
		t.Fatal("counter reset should not create a transmit rate")
	}
}

func TestCPUUsagePercentCountsIowaitAsIdle(t *testing.T) {
	previous := cpu.TimesStat{User: 100, System: 50, Idle: 800, Iowait: 50}
	current := cpu.TimesStat{User: 130, System: 60, Idle: 850, Iowait: 60}
	// Δtotal = 30+10+50+10 = 100, Δidle = 50+10 = 60 → 40 %.
	got, ok := cpuUsagePercent(previous, current)
	if !ok || got != 40 {
		t.Fatalf("cpuUsagePercent = (%v, %v), want (40, true)", got, ok)
	}
	if _, ok := cpuUsagePercent(current, current); ok {
		t.Fatal("a zero interval must not produce a CPU percentage")
	}
	if _, ok := cpuUsagePercent(current, previous); ok {
		t.Fatal("decreasing counters must not produce a CPU percentage")
	}
}

func TestAppendCPUUsageNeedsTwoCycles(t *testing.T) {
	collector := New()
	values := make(map[string]float64)
	record := func(name string, value float64, _ string, labels map[string]string) {
		values[name+"/"+labels["core"]] = value
	}
	collector.appendCPUUsage([]cpu.TimesStat{
		{CPU: "cpu-total", User: 10, Idle: 90},
		{CPU: "cpu0", User: 5, Idle: 45},
	}, record)
	if len(values) != 0 {
		t.Fatalf("first cycle produced %v, want no CPU sample", values)
	}
	collector.appendCPUUsage([]cpu.TimesStat{
		{CPU: "cpu-total", User: 30, Idle: 170},
		{CPU: "cpu0", User: 15, Idle: 85},
	}, record)
	if values["cpu.percent/all"] != 20 || values["cpu.percent/0"] != 20 {
		t.Fatalf("second cycle = %v, want 20 %% for all and core 0", values)
	}
}

// Temperatures are read every minute, not at every cycle.
func TestCollectReadsTemperaturesEveryMinute(t *testing.T) {
	collector := New()
	reads := 0
	collector.temperatures = func() ([]sensors.TemperatureStat, error) {
		reads++
		return []sensors.TemperatureStat{{SensorKey: "coretemp_core_0", Temperature: 50}}, nil
	}
	start := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	temperaturesAt := make([]int, 0)
	for cycle := range 13 {
		at := start.Add(time.Duration(cycle) * 10 * time.Second)
		// Ticks come a few milliseconds early or late.
		if cycle > 0 {
			at = at.Add(-5 * time.Millisecond)
		}
		collector.now = func() time.Time { return at }
		_, samples, err := collector.Collect(context.Background())
		if err != nil {
			t.Fatalf("collect: %v", err)
		}
		for _, sample := range samples {
			if sample.Name == domain.TemperatureMetric {
				temperaturesAt = append(temperaturesAt, cycle)
			}
		}
	}
	if want := []int{0, 6, 12}; !slices.Equal(temperaturesAt, want) || reads != 3 {
		t.Fatalf("temperatures in cycles %v (%d reads), want %v", temperaturesAt, reads, want)
	}
}

func TestSensorLabelsTellApartSensorsSharingAKey(t *testing.T) {
	readings := []sensors.TemperatureStat{
		{SensorKey: "dell_smm"}, {SensorKey: "nvme_composite"}, {SensorKey: "dell_smm"}, {SensorKey: "dell_smm"},
	}
	got := sensorLabels(readings)
	if want := []string{"dell_smm", "nvme_composite", "dell_smm_2", "dell_smm_3"}; !slices.Equal(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
}
