package httpdelivery

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"
)

// fakeUnits stands for the systemd adapter.
type fakeUnits struct {
	units []domain.SystemUnit
	err   error
}

func (f *fakeUnits) Units(context.Context) ([]domain.SystemUnit, error) { return f.units, f.err }

func (f *fakeUnits) Unit(_ context.Context, name string) (domain.SystemUnitDetail, error) {
	if name == "broken.service" {
		return domain.SystemUnitDetail{}, errors.New("org.freedesktop.DBus.Error.NoReply: /run/dbus/secret")
	}
	if name != "ssh.service" {
		return domain.SystemUnitDetail{}, domain.ErrUnitNotFound
	}
	nofile := uint64(1024)
	return domain.SystemUnitDetail{
		SystemUnit: f.units[1], FragmentPath: "/lib/systemd/system/ssh.service",
		ExecStart: []string{"/usr/sbin/sshd -D"}, Requires: []string{"sysinit.target"}, After: []string{"network.target"},
		Restart: "on-failure", Slice: "system.slice",
		TasksMax: domain.UnitLimit{Unlimited: true}, LimitNOFILE: domain.UnitLimit{Value: &nofile},
	}, nil
}

func sampleUnits() []domain.SystemUnit {
	at := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	pid, memory, cpu := uint32(812), uint64(4_194_304), uint64(2_000_000_000)
	return []domain.SystemUnit{
		{Name: "backup.service", Type: domain.UnitTypeService, LoadState: "loaded", ActiveState: "failed", SubState: "failed", Result: "exit-code"},
		{
			Name: "ssh.service", Description: "OpenBSD Secure Shell server", Type: domain.UnitTypeService,
			LoadState: "loaded", ActiveState: "active", SubState: "running", UnitFileState: "enabled",
			StateChangedAt: &at, MainPID: &pid, MemoryBytes: &memory, CPUNanoseconds: &cpu, Result: "success",
		},
		{Name: "broken.service", Type: domain.UnitTypeService, ActiveState: "active"},
	}
}

// newServicesRouter runs one collection of the fake inventory.
func newServicesRouter(t *testing.T, reader *fakeUnits) http.Handler {
	t.Helper()
	services, _ := newTestServices(t, nil)
	units := usecase.NewServiceUsecase(reader, nil, usecase.NewJournalUsecase(newFakeJournal()), zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		units.Run(ctx)
	}()
	for deadline := time.Now().Add(5 * time.Second); units.Inventory().Status == domain.UnitsPending; {
		if time.Now().After(deadline) {
			t.Fatal("no collection")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	services.Units = units
	return NewRouter(zap.NewNop(), services, false, "")
}

func signInViewer(t *testing.T, router http.Handler) *http.Cookie {
	t.Helper()
	admin := signIn(t, router, "admin", "admin-password-1")
	if response := call(router, http.MethodPost, "/api/v1/users", `{"username":"vera","displayName":"Vera","role":"viewer","password":"initial-password-1"}`, admin); response.Code != http.StatusCreated {
		t.Fatalf("create viewer = %d %s", response.Code, response.Body)
	}
	vera := signIn(t, router, "vera", "initial-password-1")
	if response := call(router, http.MethodPost, "/api/v1/auth/password", `{"currentPassword":"initial-password-1","newPassword":"vera-own-password"}`, vera); response.Code != http.StatusOK {
		t.Fatalf("viewer password = %d", response.Code)
	}
	return signIn(t, router, "vera", "vera-own-password")
}

func TestSystemUnitRoutesForEveryRole(t *testing.T) {
	resetLoginRateLimit(t)
	router := newServicesRouter(t, &fakeUnits{units: sampleUnits()})
	if response := call(router, http.MethodGet, "/api/v1/system/units", "", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", response.Code)
	}
	vera := signInViewer(t, router)

	response := call(router, http.MethodGet, "/api/v1/system/units", "", vera)
	if response.Code != http.StatusOK {
		t.Fatalf("list = %d %s", response.Code, response.Body)
	}
	var list struct {
		Status      string  `json:"status"`
		CollectedAt *string `json:"collectedAt"`
		Counts      struct {
			Total, Active, Failed, ActiveTimers int
		} `json:"counts"`
		Units []map[string]any `json:"units"`
	}
	decodeBody(t, response, &list)
	if list.Status != "ok" || list.CollectedAt == nil || list.Counts.Total != 3 || list.Counts.Failed != 1 || list.Counts.Active != 2 {
		t.Fatalf("list = %+v", list)
	}
	failed := list.Units[0]
	for _, key := range []string{"mainPid", "memoryBytes", "cpuNanoseconds", "stateChangedAt", "nextElapse", "lastTrigger", "unitFileState"} {
		if value, present := failed[key]; !present || value != nil {
			t.Errorf("failed unit %s = %v (present %v), want null", key, value, present)
		}
	}
	if triggers, ok := failed["triggers"].([]any); !ok || len(triggers) != 0 {
		t.Errorf("triggers = %v, want []", failed["triggers"])
	}
	ssh := list.Units[1]
	if ssh["name"] != "ssh.service" || ssh["type"] != "service" || ssh["mainPid"] != float64(812) ||
		ssh["memoryBytes"] != float64(4_194_304) || ssh["stateChangedAt"] != "2026-11-23T09:00:00Z" || ssh["unitFileState"] != "enabled" {
		t.Errorf("ssh = %v", ssh)
	}

	response = call(router, http.MethodGet, "/api/v1/system/units/ssh.service", "", vera)
	if response.Code != http.StatusOK {
		t.Fatalf("detail = %d %s", response.Code, response.Body)
	}
	var detail struct {
		Unit       map[string]any `json:"unit"`
		Definition struct {
			FragmentPath string   `json:"fragmentPath"`
			ExecStart    []string `json:"execStart"`
			Requires     []string `json:"requires"`
			After        []string `json:"after"`
			Restart      string   `json:"restart"`
			Slice        string   `json:"slice"`
			TasksMax     struct {
				Value     *uint64 `json:"value"`
				Unlimited bool    `json:"unlimited"`
			} `json:"tasksMax"`
			LimitNOFILE struct {
				Value     *uint64 `json:"value"`
				Unlimited bool    `json:"unlimited"`
			} `json:"limitNofile"`
			MemoryMax struct {
				Value     *uint64 `json:"value"`
				Unlimited bool    `json:"unlimited"`
			} `json:"memoryMax"`
		} `json:"definition"`
		Journal struct {
			Available bool `json:"available"`
			Entries   []struct {
				Unit    string `json:"unit"`
				Message string `json:"message"`
			} `json:"entries"`
		} `json:"journal"`
	}
	decodeBody(t, response, &detail)
	definition := detail.Definition
	if detail.Unit["name"] != "ssh.service" || definition.FragmentPath != "/lib/systemd/system/ssh.service" ||
		len(definition.ExecStart) != 1 || definition.ExecStart[0] != "/usr/sbin/sshd -D" || definition.Restart != "on-failure" {
		t.Fatalf("detail = %+v", detail)
	}
	if !definition.TasksMax.Unlimited || definition.TasksMax.Value != nil ||
		definition.LimitNOFILE.Value == nil || *definition.LimitNOFILE.Value != 1024 ||
		definition.MemoryMax.Unlimited || definition.MemoryMax.Value != nil {
		t.Fatalf("limits = %+v %+v %+v", definition.TasksMax, definition.LimitNOFILE, definition.MemoryMax)
	}
	if !detail.Journal.Available || len(detail.Journal.Entries) != 5 || detail.Journal.Entries[0].Message != "message 4" {
		t.Fatalf("journal = %+v", detail.Journal)
	}
	if strings.Contains(response.Body.String(), "hunter2") {
		t.Fatal("the command line of a journal entry leaked")
	}
}

func TestSystemUnitDetailErrors(t *testing.T) {
	resetLoginRateLimit(t)
	router := newServicesRouter(t, &fakeUnits{units: sampleUnits()})
	admin := signIn(t, router, "admin", "admin-password-1")
	for path, want := range map[string]struct {
		status int
		code   string
	}{
		"/api/v1/system/units/ssh.mount":          {http.StatusBadRequest, "VALIDATION_ERROR"},
		"/api/v1/system/units/a%5Cx2d.service":    {http.StatusBadRequest, "VALIDATION_ERROR"},
		"/api/v1/system/units/unknown.service":    {http.StatusNotFound, "NOT_FOUND"},
		"/api/v1/system/units/broken.service":     {http.StatusInternalServerError, "SYSTEMD_READ_FAILED"},
		"/api/v1/system/units/getty%40tty1.timer": {http.StatusNotFound, "NOT_FOUND"},
	} {
		response := call(router, http.MethodGet, path, "", admin)
		if response.Code != want.status || !strings.Contains(response.Body.String(), `"code":"`+want.code+`"`) {
			t.Errorf("%s = %d %s, want %d %s", path, response.Code, response.Body, want.status, want.code)
		}
		if strings.Contains(response.Body.String(), "/run/dbus") {
			t.Errorf("%s leaked an internal detail: %s", path, response.Body)
		}
	}
}

func TestSystemUnitsWithoutSystemd(t *testing.T) {
	resetLoginRateLimit(t)
	router := newServicesRouter(t, &fakeUnits{err: domain.ErrSystemdUnavailable})
	admin := signIn(t, router, "admin", "admin-password-1")

	response := call(router, http.MethodGet, "/api/v1/system/units", "", admin)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"unavailable","collectedAt":null`) ||
		!strings.Contains(response.Body.String(), `"units":[]`) {
		t.Fatalf("list = %d %s", response.Code, response.Body)
	}
	response = call(router, http.MethodGet, "/api/v1/system/units/ssh.service", "", admin)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"SYSTEMD_UNAVAILABLE"`) {
		t.Fatalf("detail = %d %s", response.Code, response.Body)
	}
}
