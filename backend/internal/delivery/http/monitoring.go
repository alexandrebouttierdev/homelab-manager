package httpdelivery

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5"
)

var (
	// metricNamePattern matches the stable dotted metric names (cpu.percent).
	metricNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)
	// containerIDPattern accepts a Manager UUID or a Docker hexadecimal ID.
	containerIDPattern = regexp.MustCompile(`^([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[0-9a-f]{12,64})$`)
)

const maxMetricNameLength = 64

type settingsDTO struct {
	SystemCollectIntervalSeconds   int                               `json:"systemCollectIntervalSeconds"`
	DockerCollectIntervalSeconds   int                               `json:"dockerCollectIntervalSeconds"`
	DockerDiscoveryIntervalSeconds int                               `json:"dockerDiscoveryIntervalSeconds"`
	MetricsRetentionDays           int                               `json:"metricsRetentionDays"`
	Collectors                     map[string]domain.CollectorStatus `json:"collectors"`
}

type hostDTO struct {
	Hostname      string     `json:"hostname"`
	OS            osDTO      `json:"os"`
	KernelVersion string     `json:"kernelVersion"`
	BootTime      *time.Time `json:"bootTime"`
	UptimeSeconds *uint64    `json:"uptimeSeconds"`
	Collector     any        `json:"collector,omitempty"`
}

type osDTO struct {
	Distribution string `json:"distribution"`
	Version      string `json:"version"`
}

type containerDTO struct {
	ID            string           `json:"id"`
	DockerID      string           `json:"dockerId"`
	Name          string           `json:"name"`
	Image         string           `json:"image"`
	State         string           `json:"state"`
	Status        string           `json:"status"`
	CreatedAt     *time.Time       `json:"createdAt"`
	StartedAt     *time.Time       `json:"startedAt"`
	UptimeSeconds *uint64          `json:"uptimeSeconds"`
	RestartCount  uint64           `json:"restartCount"`
	RestartPolicy string           `json:"restartPolicy"`
	Ports         []map[string]any `json:"ports"`
	Networks      []string         `json:"networks"`
	Mounts        []map[string]any `json:"mounts"`
	LastObserved  time.Time        `json:"lastObservedAt"`
	RemovedAt     *time.Time       `json:"removedAt,omitempty"`
}

type metricDTO struct {
	Name       string            `json:"name"`
	Value      float64           `json:"value"`
	Unit       string            `json:"unit"`
	Labels     map[string]string `json:"labels"`
	RecordedAt time.Time         `json:"recordedAt"`
}

type eventDTO struct {
	ID          string    `json:"id"`
	ContainerID *string   `json:"containerId"`
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Message     string    `json:"message"`
	Metadata    any       `json:"metadata,omitempty"`
	OccurredAt  time.Time `json:"occurredAt"`
}

type dashboardDTO struct {
	Host       hostDTO                           `json:"host"`
	Containers usecase.ContainerCounts           `json:"containers"`
	Metrics    []metricDTO                       `json:"metrics"`
	Collectors map[string]domain.CollectorStatus `json:"collectors"`
}

func hostResponse(host domain.HostInfo, status any) hostDTO {
	return hostDTO{
		Hostname:      host.Hostname,
		OS:            osDTO{Distribution: host.Distribution, Version: host.OSVersion},
		KernelVersion: host.KernelVersion, BootTime: host.BootTime,
		UptimeSeconds: host.UptimeSeconds, Collector: status,
	}
}

func containerResponse(container domain.Container) containerDTO {
	created := container.CreatedAt
	var createdAt *time.Time
	if !created.IsZero() {
		createdAt = &created
	}
	return containerDTO{
		ID: container.ID.String(), DockerID: container.DockerID, Name: container.Name,
		Image: container.Image, State: container.State, Status: container.Status,
		CreatedAt: createdAt, StartedAt: container.StartedAt,
		UptimeSeconds: container.UptimeSeconds, RestartCount: container.RestartCount,
		RestartPolicy: container.RestartPolicy, Ports: container.Ports, Networks: container.Networks,
		Mounts: container.Mounts, LastObserved: container.LastSeenAt, RemovedAt: container.RemovedAt,
	}
}

func metricResponses(samples []domain.MetricSample) []metricDTO {
	result := make([]metricDTO, 0, len(samples))
	for _, sample := range samples {
		result = append(result, metricDTO{
			Name: sample.Name, Value: sample.Value, Unit: sample.Unit,
			Labels: sample.Labels, RecordedAt: sample.RecordedAt,
		})
	}
	return result
}

func dashboardHandler(monitoring *usecase.MonitoringUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := monitoring.Dashboard(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load dashboard.")
			return
		}
		writeJSON(w, http.StatusOK, dashboardDTO{
			Host:       hostResponse(result.Host, result.Collectors["system"]),
			Containers: result.Containers,
			Metrics:    metricResponses(result.Metrics),
			Collectors: result.Collectors,
		})
	}
}

func hostHandler(monitoring *usecase.MonitoringUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, err := monitoring.Host(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load host information.")
			return
		}
		systemStatus, _ := monitoring.Status()
		writeJSON(w, http.StatusOK, hostResponse(host, systemStatus))
	}
}

func settingsHandler(monitoring *usecase.MonitoringUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		settings := monitoring.Settings()
		writeJSON(w, http.StatusOK, settingsDTO{
			SystemCollectIntervalSeconds:   settings.SystemCollectIntervalSeconds,
			DockerCollectIntervalSeconds:   settings.DockerCollectIntervalSeconds,
			DockerDiscoveryIntervalSeconds: settings.DockerDiscoveryIntervalSeconds,
			MetricsRetentionDays:           settings.MetricsRetentionDays,
			Collectors:                     map[string]domain.CollectorStatus{"system": settings.System, "docker": settings.Docker},
		})
	}
}

// containerIDParam validates the {containerID} path parameter.
func containerIDParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	value := chi.URLParam(r, "containerID")
	if !containerIDPattern.MatchString(value) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "containerID must be a Manager UUID or a Docker ID.")
		return "", false
	}
	return value, true
}

func metricNameParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	value := chi.URLParam(r, "metricName")
	if len(value) > maxMetricNameLength || !metricNamePattern.MatchString(value) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "metricName is invalid.")
		return "", false
	}
	return value, true
}

func listContainersHandler(monitoring *usecase.MonitoringUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for key, values := range query {
			if key != "includeRemoved" || len(values) != 1 {
				writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Only one includeRemoved value is allowed.")
				return
			}
		}
		includeRemovedValue := query.Get("includeRemoved")
		if includeRemovedValue != "" && includeRemovedValue != "true" && includeRemovedValue != "false" {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "includeRemoved must be true or false.")
			return
		}
		containers, err := monitoring.Containers(r.Context(), includeRemovedValue == "true", 500)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not list containers.")
			return
		}
		response := make([]containerDTO, 0, len(containers))
		for _, container := range containers {
			response = append(response, containerResponse(container))
		}
		writeJSON(w, http.StatusOK, map[string]any{"containers": response, "nextCursor": nil})
	}
}

func getContainerHandler(monitoring *usecase.MonitoringUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		containerID, ok := containerIDParam(w, r)
		if !ok {
			return
		}
		container, err := monitoring.Container(r.Context(), containerID)
		if errors.Is(err, domain.ErrContainerNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Container not found.")
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load container.")
			return
		}
		writeJSON(w, http.StatusOK, containerResponse(container))
	}
}

func currentMetricsHandler(monitoring *usecase.MonitoringUsecase, container bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var id *string
		if container {
			value, ok := containerIDParam(w, r)
			if !ok {
				return
			}
			id = &value
		}
		samples, err := monitoring.CurrentMetrics(r.Context(), id)
		if errors.Is(err, domain.ErrContainerNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Container not found.")
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "METRICS_UNAVAILABLE", "Could not load current metrics.")
			return
		}
		systemStatus, dockerStatus := monitoring.Status()
		status := systemStatus
		if container {
			status = dockerStatus
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"samples": metricResponses(samples), "status": status.Status,
			"lastSuccessAt": status.LastSuccessAt,
		})
	}
}

func metricHistoryHandler(monitoring *usecase.MonitoringUsecase, container bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		from, to, limit, err := timeRange(r)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		var id *string
		if container {
			value, ok := containerIDParam(w, r)
			if !ok {
				return
			}
			id = &value
		}
		metricName, ok := metricNameParam(w, r)
		if !ok {
			return
		}
		samples, err := monitoring.Metrics(r.Context(), id, metricName, from, to, limit)
		if errors.Is(err, domain.ErrContainerNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Container not found.")
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "METRICS_UNAVAILABLE", "Could not load metric history.")
			return
		}
		resolution := r.URL.Query().Get("step")
		if resolution == "" {
			resolution = "raw"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"samples": metricResponses(samples), "nextCursor": nil, "resolution": resolution,
		})
	}
}

func listEventsHandler(monitoring *usecase.MonitoringUsecase, container bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		from, to, limit, err := timeRange(r)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		var id *string
		if container {
			value, ok := containerIDParam(w, r)
			if !ok {
				return
			}
			id = &value
		}
		events, err := monitoring.Events(r.Context(), id, from, to, limit)
		if errors.Is(err, domain.ErrContainerNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Container not found.")
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "EVENTS_UNAVAILABLE", "Could not load events.")
			return
		}
		response := make([]eventDTO, 0, len(events))
		for _, event := range events {
			var containerID *string
			if event.ContainerID != nil {
				value := event.ContainerID.String()
				containerID = &value
			}
			response = append(response, eventDTO{
				ID: event.ID.String(), ContainerID: containerID, Type: event.Type,
				Severity: event.Severity, Message: event.Message,
				Metadata: event.Metadata, OccurredAt: event.OccurredAt,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"events": response, "nextCursor": nil})
	}
}

func hostTemperaturesHandler(monitoring *usecase.MonitoringUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		samples, err := monitoring.CurrentMetrics(r.Context(), nil)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "METRICS_UNAVAILABLE", "Could not load temperatures.")
			return
		}
		temperatures := make([]domain.MetricSample, 0)
		for _, sample := range samples {
			if sample.Name == "temperature.celsius" {
				temperatures = append(temperatures, sample)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"samples": metricResponses(temperatures)})
	}
}

func timeRange(r *http.Request) (time.Time, time.Time, int, error) {
	allowed := map[string]struct{}{"from": {}, "to": {}, "step": {}, "limit": {}, "cursor": {}}
	for key, values := range r.URL.Query() {
		if _, ok := allowed[key]; !ok {
			return time.Time{}, time.Time{}, 0, errors.New("unknown query parameter")
		}
		if len(values) != 1 {
			return time.Time{}, time.Time{}, 0, errors.New("query parameters must not be repeated")
		}
	}
	from, err := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	if err != nil {
		return time.Time{}, time.Time{}, 0, errors.New("from must be an RFC 3339 timestamp")
	}
	to, err := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if err != nil {
		return time.Time{}, time.Time{}, 0, errors.New("to must be an RFC 3339 timestamp")
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, 0, errors.New("from must precede to")
	}
	if to.Sub(from) > 365*24*time.Hour {
		return time.Time{}, time.Time{}, 0, errors.New("time range exceeds 365 days")
	}
	limit := 500
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 2000 {
			return time.Time{}, time.Time{}, 0, errors.New("limit must be between 1 and 2000")
		}
		limit = parsed
	}
	step := r.URL.Query().Get("step")
	if step != "" && step != "raw" {
		return time.Time{}, time.Time{}, 0, errors.New("only raw metric samples are currently supported")
	}
	if r.URL.Query().Get("cursor") != "" {
		return time.Time{}, time.Time{}, 0, errors.New("cursor pagination is not currently supported")
	}
	return from.UTC(), to.UTC(), limit, nil
}
