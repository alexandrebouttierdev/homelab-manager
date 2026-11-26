package httpdelivery

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"monoops/backend/internal/adapters/sqlite"
	"monoops/backend/internal/domain"
	"monoops/backend/internal/usecase"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type pageResponse struct {
	Containers []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"containers"`
	Events []struct {
		ID string `json:"id"`
	} `json:"events"`
	NextCursor *string `json:"nextCursor"`
}

func getPage(t *testing.T, router http.Handler, cookie *http.Cookie, path string) pageResponse {
	t.Helper()
	response := call(router, http.MethodGet, path, "", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", path, response.Code, response.Body)
	}
	var page pageResponse
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestContainerAndEventCursors(t *testing.T) {
	resetLoginRateLimit(t)
	services, database := newTestServices(t, nil)
	repository := sqlite.NewMonitoringRepository(database)
	services.Monitoring = usecase.NewMonitoringUsecase(nil, nil, repository, usecase.MonitoringConfig{}, zap.NewNop())
	router := NewRouter(zap.NewNop(), services, false, "")
	admin := signIn(t, router, "admin", "admin-password-1")

	at := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	inventory := make([]domain.Container, 0, containerPageSize+1)
	for index := range containerPageSize + 1 {
		inventory = append(inventory, domain.Container{
			DockerID: fmt.Sprintf("docker-%04d", index), Name: fmt.Sprintf("app-%04d", index),
			Image: "test:latest", State: "running", Status: "Up", CreatedAt: at,
		})
	}
	if _, err := repository.ReconcileContainers(context.Background(), inventory, at); err != nil {
		t.Fatal(err)
	}

	first := getPage(t, router, admin, "/api/v1/containers")
	if len(first.Containers) != containerPageSize || first.NextCursor == nil {
		t.Fatalf("first page = %d containers, next %v", len(first.Containers), first.NextCursor)
	}
	second := getPage(t, router, admin, "/api/v1/containers?cursor="+url.QueryEscape(*first.NextCursor))
	if len(second.Containers) != 1 || second.NextCursor != nil || second.Containers[0].Name != fmt.Sprintf("app-%04d", containerPageSize) {
		t.Fatalf("second page = %+v", second)
	}

	window := "from=2026-11-25T07:00:00Z&to=2026-11-25T09:00:00Z"
	seen := map[string]bool{}
	path := "/api/v1/events?" + window + "&limit=200"
	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("the event cursor does not end")
		}
		page := getPage(t, router, admin, path)
		for _, event := range page.Events {
			if seen[event.ID] {
				t.Fatalf("event %s is repeated", event.ID)
			}
			seen[event.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		path = "/api/v1/events?" + window + "&limit=200&cursor=" + url.QueryEscape(*page.NextCursor)
	}
	if len(seen) != len(inventory) {
		t.Fatalf("%d events, want %d", len(seen), len(inventory))
	}
	exact := getPage(t, router, admin, "/api/v1/events?"+window+fmt.Sprintf("&limit=%d", len(inventory)))
	if len(exact.Events) != len(inventory) || exact.NextCursor != nil {
		t.Fatalf("a page holding every event has a next cursor: %d, %v", len(exact.Events), exact.NextCursor)
	}

	eventCursor := encodeCursor(at.Format(time.RFC3339Nano), uuid.New())
	for _, path := range []string{
		"/api/v1/containers?cursor=not-base64!",
		"/api/v1/containers?cursor=" + encodeCursor("x", uuid.New()) + "&cursor=" + encodeCursor("y", uuid.New()),
		"/api/v1/containers?page=2",
		"/api/v1/events?" + window + "&cursor=" + encodeCursor("not a time", uuid.New()),
		"/api/v1/events?" + window + "&cursor=" + *first.NextCursor,
		"/api/v1/host/metrics/cpu.percent?" + window + "&cursor=" + eventCursor,
	} {
		if response := call(router, http.MethodGet, path, "", admin); response.Code != http.StatusBadRequest {
			t.Fatalf("GET %s = %d, want 400", path, response.Code)
		}
	}
}

func TestDecodeCursor(t *testing.T) {
	id := uuid.New()
	raw := func(text string) string { return base64.RawURLEncoding.EncodeToString([]byte(text)) }
	key, decoded, err := decodeCursor(encodeCursor("web", id))
	if err != nil || key != "web" || decoded != id {
		t.Fatalf("round trip = %q, %s, %v", key, decoded, err)
	}
	for _, value := range []string{
		"",
		"%%%",
		encodeCursor("", id),
		raw(`{"k":"web"}`),
		raw(`{"k":"web","id":"x"}`),
		raw(`{"k":"web","id":"` + strings.ToUpper(id.String()) + `"}`),
		raw(`{"k":"web","id":"` + id.String() + `","x":1}`),
		raw(`{"k":"web","id":"` + id.String() + `"}{}`),
		string(make([]byte, maxCursorLength+1)),
	} {
		if _, _, err := decodeCursor(value); err == nil {
			t.Fatalf("decodeCursor(%.30q) accepted", value)
		}
	}
}
