package httpdelivery

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5/middleware"
)

var (
	findingQueryParameters = map[string]bool{"state": true, "severity": true, "fixable": true, "q": true, "limit": true, "offset": true}
	scanQueryParameters    = map[string]bool{"limit": true}
)

type severityTotalsDTO struct {
	Critical   int `json:"critical"`
	High       int `json:"high"`
	Medium     int `json:"medium"`
	Low        int `json:"low"`
	Negligible int `json:"negligible"`
	Unknown    int `json:"unknown"`
}

type scanDTO struct {
	ID            string            `json:"id"`
	Trigger       string            `json:"trigger"`
	RequestedBy   *string           `json:"requestedBy"`
	State         string            `json:"state"`
	Failure       *string           `json:"failure"`
	StartedAt     time.Time         `json:"startedAt"`
	FinishedAt    *time.Time        `json:"finishedAt"`
	GrypeVersion  *string           `json:"grypeVersion"`
	DBBuiltAt     *time.Time        `json:"dbBuiltAt"`
	Distro        *distroDTO        `json:"distro"`
	PackageCount  int               `json:"packageCount"`
	Totals        severityTotalsDTO `json:"totals"`
	NewCount      int               `json:"newCount"`
	ResolvedCount int               `json:"resolvedCount"`
}

type distroDTO struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Name    string `json:"name"`
}

type findingDTO struct {
	ID              string     `json:"id"`
	VulnerabilityID string     `json:"vulnerabilityId"`
	Severity        string     `json:"severity"`
	Package         string     `json:"package"`
	Version         string     `json:"version"`
	PackageType     string     `json:"packageType"`
	FixState        string     `json:"fixState"`
	FixedVersions   []string   `json:"fixedVersions"`
	Description     *string    `json:"description"`
	URLs            []string   `json:"urls"`
	DataSource      *string    `json:"dataSource"`
	FirstSeenAt     time.Time  `json:"firstSeenAt"`
	LastSeenAt      time.Time  `json:"lastSeenAt"`
	ResolvedAt      *time.Time `json:"resolvedAt"`
}

func toTotalsDTO(totals domain.SeverityTotals) severityTotalsDTO {
	return severityTotalsDTO{
		Critical: totals[domain.VulnCritical], High: totals[domain.VulnHigh], Medium: totals[domain.VulnMedium],
		Low: totals[domain.VulnLow], Negligible: totals[domain.VulnNegligible], Unknown: totals[domain.VulnUnknown],
	}
}

func toScanDTO(scan *domain.Scan) *scanDTO {
	if scan == nil {
		return nil
	}
	dto := &scanDTO{
		ID: scan.ID.String(), Trigger: string(scan.Trigger), RequestedBy: optional(scan.RequestedBy), State: string(scan.State),
		Failure: optional(string(scan.Failure)), StartedAt: scan.StartedAt, FinishedAt: scan.FinishedAt,
		GrypeVersion: optional(scan.GrypeVersion), DBBuiltAt: scan.DBBuiltAt, PackageCount: scan.PackageCount,
		Totals: toTotalsDTO(scan.Totals), NewCount: scan.NewCount, ResolvedCount: scan.ResolvedCount,
	}
	if scan.Distro.ID != "" {
		dto.Distro = &distroDTO{ID: scan.Distro.ID, Version: scan.Distro.VersionID, Name: scan.Distro.PrettyName}
	}
	return dto
}

func toFindingDTO(finding domain.Finding) findingDTO {
	dto := findingDTO{
		ID: finding.ID.String(), VulnerabilityID: finding.VulnerabilityID, Severity: string(finding.Severity),
		Package: finding.Package, Version: finding.Version, PackageType: finding.PackageType, FixState: string(finding.FixState),
		FixedVersions: finding.FixedVersions, Description: optional(finding.Description), URLs: finding.URLs,
		DataSource: optional(finding.DataSource), FirstSeenAt: finding.FirstSeenAt, LastSeenAt: finding.LastSeenAt, ResolvedAt: finding.ResolvedAt,
	}
	if dto.FixedVersions == nil {
		dto.FixedVersions = []string{}
	}
	if dto.URLs == nil {
		dto.URLs = []string{}
	}
	return dto
}

// requireVulnerabilities answers 503 when the module is not running (no
// database).
func requireVulnerabilities(vulnerabilities *usecase.VulnerabilityUsecase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if vulnerabilities == nil {
				writeError(w, r, http.StatusServiceUnavailable, "VULNERABILITIES_UNAVAILABLE", "Vulnerability scans are unavailable.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func vulnerabilitySummaryHandler(vulnerabilities *usecase.VulnerabilityUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summary, err := vulnerabilities.Summary(r.Context())
		if err != nil {
			writeVulnerabilityError(w, r, err)
			return
		}
		recent := make([]findingDTO, 0, len(summary.Recent))
		for _, finding := range summary.Recent {
			recent = append(recent, toFindingDTO(finding))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"totals": toTotalsDTO(summary.Totals), "total": summary.Totals.Sum(), "fixable": summary.Fixable,
			"latestScan": toScanDTO(summary.LatestScan), "lastSuccessfulScan": toScanDTO(summary.LastSuccess),
			"running": summary.Running, "nextScanAt": summary.NextScanAt, "intervalHours": int(summary.Interval / time.Hour),
			"defaultIntervalHours": int(summary.DefaultInterval / time.Hour), "intervalSource": summary.Source,
			"recent": recent,
		})
	}
}

func findingFilterFromQuery(r *http.Request) (domain.FindingFilter, bool) {
	query := r.URL.Query()
	for key, values := range query {
		if !findingQueryParameters[key] || len(values) != 1 {
			return domain.FindingFilter{}, false
		}
	}
	filter := domain.FindingFilter{State: query.Get("state"), Query: query.Get("q")}
	if value := query.Get("severity"); value != "" {
		for _, severity := range strings.Split(value, ",") {
			filter.Severities = append(filter.Severities, domain.VulnSeverity(severity))
		}
	}
	switch query.Get("fixable") {
	case "", "false":
	case "true":
		filter.FixableOnly = true
	default:
		return domain.FindingFilter{}, false
	}
	for name, destination := range map[string]*int{"limit": &filter.Limit, "offset": &filter.Offset} {
		if value := query.Get(name); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return domain.FindingFilter{}, false
			}
			*destination = parsed
		}
	}
	if query.Get("limit") == "0" {
		return domain.FindingFilter{}, false
	}
	return filter, true
}

func listFindingsHandler(vulnerabilities *usecase.VulnerabilityUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, ok := findingFilterFromQuery(r)
		if !ok {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Unknown or repeated query parameter, or invalid fixable, limit or offset.")
			return
		}
		page, err := vulnerabilities.Findings(r.Context(), filter)
		if err != nil {
			writeVulnerabilityError(w, r, err)
			return
		}
		findings := make([]findingDTO, 0, len(page.Findings))
		for _, finding := range page.Findings {
			findings = append(findings, toFindingDTO(finding))
		}
		limit := filter.Limit
		if limit == 0 {
			limit = domain.DefaultFindingLimit
		}
		writeJSON(w, http.StatusOK, map[string]any{"findings": findings, "total": page.Total, "limit": limit, "offset": filter.Offset})
	}
}

func findingHandler(vulnerabilities *usecase.VulnerabilityUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := uuidParam(w, r, "findingID")
		if !ok {
			return
		}
		finding, err := vulnerabilities.Finding(r.Context(), id)
		if err != nil {
			writeVulnerabilityError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"finding": toFindingDTO(finding)})
	}
}

func listScansHandler(vulnerabilities *usecase.VulnerabilityUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		limit := 0
		for key, values := range query {
			if !scanQueryParameters[key] || len(values) != 1 {
				writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Only limit is accepted, once.")
				return
			}
		}
		if value := query.Get("limit"); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 {
				writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be 1 to 100.")
				return
			}
			limit = parsed
		}
		scans, err := vulnerabilities.Scans(r.Context(), limit)
		if err != nil {
			writeVulnerabilityError(w, r, err)
			return
		}
		response := make([]*scanDTO, 0, len(scans))
		for i := range scans {
			response = append(response, toScanDTO(&scans[i]))
		}
		writeJSON(w, http.StatusOK, map[string]any{"scans": response})
	}
}

func requestScanHandler(vulnerabilities *usecase.VulnerabilityUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		scan, err := vulnerabilities.RequestScan(r.Context(), actor, middleware.GetReqID(r.Context()))
		if err != nil {
			writeVulnerabilityError(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"scan": toScanDTO(&scan)})
	}
}

type scanIntervalRequest struct {
	IntervalHours *int `json:"intervalHours"`
}

func updateScanIntervalHandler(vulnerabilities *usecase.VulnerabilityUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request scanIntervalRequest
		if err := decodeJSON(r, &request); err != nil || request.IntervalHours == nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "intervalHours is required.")
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		settings, err := vulnerabilities.UpdateInterval(r.Context(), actor, *request.IntervalHours, middleware.GetReqID(r.Context()))
		if err != nil {
			writeVulnerabilityError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"intervalHours": int(settings.Interval / time.Hour), "defaultIntervalHours": int(settings.DefaultInterval / time.Hour),
			"intervalSource": settings.Source,
		})
	}
}

func writeVulnerabilityError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidScanInterval):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "intervalHours must be 6 to 168.")
	case errors.Is(err, domain.ErrInvalidFindingFilter):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The vulnerability filter is invalid.")
	case errors.Is(err, domain.ErrFindingNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The vulnerability does not exist.")
	case errors.Is(err, domain.ErrScanInProgress):
		writeError(w, r, http.StatusConflict, "SCAN_IN_PROGRESS", "A vulnerability scan is already running.")
	case errors.Is(err, domain.ErrScanRateLimited):
		writeError(w, r, http.StatusTooManyRequests, "SCAN_RATE_LIMITED", "Wait 5 minutes between two manual scans.")
	default:
		writeError(w, r, http.StatusInternalServerError, "VULNERABILITIES_FAILED", "Could not read or start the vulnerability scans.")
	}
}
