package system

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	netstat "github.com/shirou/gopsutil/v4/net"
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
