package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"monoops/backend/internal/domain"

	"go.uber.org/zap"
)

type inventoryDocker struct {
	unavailableDocker
	inventory domain.DockerInventory
	err       error
}

func (d *inventoryDocker) Inventory(context.Context) (domain.DockerInventory, error) {
	return d.inventory, d.err
}

func TestDockerInventoryKeepsTheLastSuccess(t *testing.T) {
	docker := &inventoryDocker{inventory: domain.DockerInventory{
		Images: []domain.DockerImage{{ID: "sha256:a", Tags: []string{"nginx:stable"}}},
	}}
	monitoring := NewMonitoringUsecase(nil, docker, testMonitoringRepository{}, MonitoringConfig{}, zap.NewNop())
	now := time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)
	monitoring.now = func() time.Time { return now }

	if inventory, status := monitoring.DockerInventory(); inventory != nil || status.Status != StatusStarting {
		t.Fatalf("before the first cycle = %v, %+v", inventory, status)
	}
	if err := monitoring.collectInventory(context.Background()); err != nil {
		t.Fatal(err)
	}
	inventory, status := monitoring.DockerInventory()
	if inventory == nil || len(inventory.Images) != 1 || status.Status != StatusOK || !status.LastSuccessAt.Equal(now) {
		t.Fatalf("after a success = %+v, %+v", inventory, status)
	}

	docker.err = errors.New("Docker unavailable")
	now = now.Add(time.Minute)
	if err := monitoring.collectInventory(context.Background()); err == nil {
		t.Fatal("the failure is not reported")
	}
	inventory, status = monitoring.DockerInventory()
	if inventory == nil || len(inventory.Images) != 1 || status.Status != StatusUnavailable || status.LastSuccessAt == nil {
		t.Fatalf("after a failure = %+v, %+v", inventory, status)
	}

	docker.err = nil
	if err := monitoring.collectInventory(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(staleAfterCycles*dockerInventoryInterval + time.Second)
	if _, status := monitoring.DockerInventory(); status.Status != StatusStale {
		t.Fatalf("an old inventory = %+v, want stale", status)
	}
}
