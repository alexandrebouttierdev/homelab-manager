package sqlite

import (
	"context"
	"sort"
	"strconv"
	"testing"
	"time"

	"monoops/backend/internal/domain"
)

// TestContainerAndEventPagesAreStable pages through containers that share a
// name and events of the same instant, one or two at a time: every item
// comes once, in (name, id) order for containers and in (occurredAt, id)
// descending order for events.
func TestContainerAndEventPagesAreStable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository := NewMonitoringRepository(openTestDatabase(t, ctx))

	discovered := time.Date(2026, 11, 25, 8, 0, 0, 123456789, time.UTC)
	var inventory []domain.Container
	for index, name := range []string{"web", "db", "web", "cache", "db"} {
		inventory = append(inventory, domain.Container{
			DockerID: "docker-" + strconv.Itoa(index), Name: name, Image: "test:latest",
			State: "running", Status: "Up", CreatedAt: discovered,
		})
	}
	if _, err := repository.ReconcileContainers(ctx, inventory, discovered); err != nil {
		t.Fatal(err)
	}
	// One container stops a second later: its event comes first.
	inventory[0].State, inventory[0].Status = "exited", "Exited"
	if _, err := repository.ReconcileContainers(ctx, inventory, discovered.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	all, err := repository.ListContainersAfter(ctx, false, nil, 100)
	if err != nil || len(all) != len(inventory) {
		t.Fatalf("containers = %d, %v", len(all), err)
	}
	if !sort.SliceIsSorted(all, func(i, j int) bool {
		return all[i].Name < all[j].Name || (all[i].Name == all[j].Name && all[i].ID.String() < all[j].ID.String())
	}) {
		t.Fatalf("containers are not in (name, id) order: %+v", all)
	}
	for _, size := range []int{1, 2} {
		var paged []domain.Container
		var after *domain.ContainerCursor
		for range len(all) + 1 {
			page, err := repository.ListContainersAfter(ctx, false, after, size)
			if err != nil {
				t.Fatal(err)
			}
			if len(page) == 0 {
				break
			}
			paged = append(paged, page...)
			last := page[len(page)-1]
			after = &domain.ContainerCursor{Name: last.Name, ID: last.ID}
		}
		if len(paged) != len(all) {
			t.Fatalf("size %d: %d containers, want %d", size, len(paged), len(all))
		}
		for index := range all {
			if paged[index].ID != all[index].ID {
				t.Fatalf("size %d: container %d = %s, want %s", size, index, paged[index].Name, all[index].Name)
			}
		}
	}

	from, to := discovered.Add(-time.Hour), discovered.Add(time.Hour)
	events, err := repository.ListEvents(ctx, nil, from, to, nil, 100)
	if err != nil || len(events) != len(inventory)+1 {
		t.Fatalf("events = %d, %v", len(events), err)
	}
	if !events[0].OccurredAt.Equal(discovered.Add(time.Second)) {
		t.Fatalf("first event at %s, want the most recent", events[0].OccurredAt)
	}
	for index := 2; index < len(events); index++ {
		if !events[index].OccurredAt.Equal(discovered) || events[index-1].ID.String() < events[index].ID.String() {
			t.Fatalf("events of the same instant are not in descending id order: %+v", events)
		}
	}
	for _, size := range []int{1, 2} {
		var paged []domain.MonitorEvent
		var before *domain.EventCursor
		for range len(events) + 1 {
			page, err := repository.ListEvents(ctx, nil, from, to, before, size)
			if err != nil {
				t.Fatal(err)
			}
			if len(page) == 0 {
				break
			}
			paged = append(paged, page...)
			last := page[len(page)-1]
			before = &domain.EventCursor{OccurredAt: last.OccurredAt, ID: last.ID}
		}
		if len(paged) != len(events) {
			t.Fatalf("size %d: %d events, want %d", size, len(paged), len(events))
		}
		for index := range events {
			if paged[index].ID != events[index].ID {
				t.Fatalf("size %d: event %d differs", size, index)
			}
		}
	}
}

// TestEventTimesCompareInRealOrder checks that an event recorded with an
// offset is compared by its instant, which the event cursor depends on.
func TestEventTimesCompareInRealOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository := NewMonitoringRepository(openTestDatabase(t, ctx))
	paris := time.FixedZone("UTC+1", 3600)
	at := time.Date(2026, 11, 25, 9, 30, 0, 0, paris)
	if _, err := repository.ReconcileContainers(ctx, []domain.Container{{
		DockerID: "docker-zone", Name: "zone", Image: "test:latest", State: "running", Status: "Up", CreatedAt: at,
	}}, at); err != nil {
		t.Fatal(err)
	}
	// 08:30 UTC is before 09:00 UTC, although "09:30+01:00" sorts after it.
	events, err := repository.ListEvents(ctx, nil, at.Add(-time.Hour).UTC(), time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC), nil, 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("events = %+v, %v", events, err)
	}
}
