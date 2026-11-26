package dockerengine

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestInventoryWithARealDocker reads the inventory through a real proxy
// when MONOOPS_TEST_DOCKER_SOCKET names its socket; skipped otherwise.
func TestInventoryWithARealDocker(t *testing.T) {
	socket := os.Getenv("MONOOPS_TEST_DOCKER_SOCKET")
	if socket == "" {
		t.Skip("MONOOPS_TEST_DOCKER_SOCKET is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := NewClient(socket)
	defer client.CloseIdleConnections()
	inventory, err := client.Inventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	predefined := 0
	for _, network := range inventory.Networks {
		if network.Predefined {
			predefined++
		}
		t.Logf("network %s driver=%s containers=%d subnets=%v", network.Name, network.Driver, network.Containers, network.Subnets)
	}
	for _, image := range inventory.Images {
		t.Logf("image %v size=%d containers=%d", image.Tags, image.SizeBytes, image.Containers)
	}
	for _, volume := range inventory.Volumes {
		t.Logf("volume %s containers=%d created=%v", volume.Name, volume.Containers, volume.CreatedAt)
	}
	if predefined != 3 {
		t.Fatalf("%d predefined networks, want bridge, host and none", predefined)
	}
}
