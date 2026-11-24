package httpdelivery

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"homelab-manager/backend/internal/adapters/sqlite"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type noInventory struct{}

func (noInventory) Inventory(context.Context) (domain.PackageInventory, error) {
	return domain.PackageInventory{}, domain.ErrUnsupportedDistro
}

type noScanner struct{}

func (noScanner) Scan(context.Context, domain.PackageInventory) (domain.ScanResult, error) {
	return domain.ScanResult{}, &domain.ScanError{Failure: domain.FailureNotInstalled}
}

// newVulnerabilitiesRouter serves the module without its goroutine: the
// routes are tested on stored scans and findings.
func newVulnerabilitiesRouter(t *testing.T) (http.Handler, *sqlite.VulnerabilityRepository) {
	t.Helper()
	services, database := newTestServices(t, nil)
	repository := sqlite.NewVulnerabilityRepository(database.Client())
	services.Vulnerabilities = usecase.NewVulnerabilityUsecase(repository, noInventory{}, noScanner{}, domain.DefaultScanInterval, zap.NewNop())
	return NewRouter(zap.NewNop(), services, false, ""), repository
}

func storeScan(t *testing.T, repository *sqlite.VulnerabilityRepository) domain.Scan {
	t.Helper()
	ctx := context.Background()
	started := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	scan := domain.Scan{ID: uuid.New(), Trigger: domain.ScanStartup, State: domain.ScanRunning, StartedAt: started}
	if err := repository.StartScan(ctx, scan, nil); err != nil {
		t.Fatal(err)
	}
	finished, built := started.Add(2*time.Minute), started.Add(-5*time.Hour)
	scan.FinishedAt, scan.GrypeVersion, scan.DBBuiltAt, scan.PackageCount = &finished, "0.121.3", &built, 412
	scan.Distro = domain.Distro{ID: "debian", VersionID: "13", PrettyName: "Debian GNU/Linux 13 (trixie)"}
	matches := []domain.ScanMatch{
		{VulnerabilityID: "CVE-2026-1000", Severity: domain.VulnCritical, Package: "openssl", Version: "3.5.1-1", PackageType: "deb",
			FixState: domain.FixAvailable, FixedVersions: []string{"3.5.1-2"}, Description: "Overflow.",
			URLs: []string{"https://security-tracker.debian.org/tracker/CVE-2026-1000"}},
		{VulnerabilityID: "CVE-2026-2000", Severity: domain.VulnLow, Package: "bash", Version: "5.2.37-2", PackageType: "deb", FixState: domain.FixNotFixed},
		{VulnerabilityID: "CVE-2026-3000", Severity: domain.VulnHigh, Package: "curl", Version: "8.14.1-2", PackageType: "deb", FixState: domain.FixWontFix},
	}
	if err := repository.CompleteScan(ctx, scan, domain.MergeScanResult(nil, domain.ScanResult{Matches: matches}, scan.ID, finished)); err != nil {
		t.Fatal(err)
	}
	return scan
}

func TestVulnerabilityRoutesByRole(t *testing.T) {
	resetLoginRateLimit(t)
	router, repository := newVulnerabilitiesRouter(t)
	admin := signIn(t, router, "admin", "admin-password-1")
	viewer := signInWithRole(t, router, admin, "vera", "viewer")
	operator := signInWithRole(t, router, admin, "otto", "operator")

	response := call(router, http.MethodGet, "/api/v1/vulnerabilities/summary", "", viewer)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"latestScan":null`) ||
		!strings.Contains(response.Body.String(), `"total":0`) || !strings.Contains(response.Body.String(), `"intervalHours":24`) {
		t.Fatalf("empty summary = %d %s", response.Code, response.Body)
	}
	scan := storeScan(t, repository)
	var summary struct {
		Totals             map[string]int   `json:"totals"`
		Total              int              `json:"total"`
		Fixable            int              `json:"fixable"`
		LastSuccessfulScan map[string]any   `json:"lastSuccessfulScan"`
		Running            bool             `json:"running"`
		NextScanAt         *time.Time       `json:"nextScanAt"`
		Recent             []map[string]any `json:"recent"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/vulnerabilities/summary", "", viewer), &summary)
	distro, _ := summary.LastSuccessfulScan["distro"].(map[string]any)
	if summary.Total != 3 || summary.Totals["critical"] != 1 || summary.Totals["negligible"] != 0 || summary.Fixable != 1 || summary.Running ||
		summary.NextScanAt == nil || !summary.NextScanAt.Equal(scan.FinishedAt.Add(24*time.Hour)) || len(summary.Recent) != 3 ||
		summary.LastSuccessfulScan["grypeVersion"] != "0.121.3" || distro["name"] != "Debian GNU/Linux 13 (trixie)" || summary.LastSuccessfulScan["newCount"] != 3.0 {
		t.Fatalf("summary = %+v", summary)
	}

	if response := call(router, http.MethodPost, "/api/v1/vulnerabilities/scans", "", viewer); response.Code != http.StatusForbidden {
		t.Fatalf("viewer scan = %d", response.Code)
	}
	response = call(router, http.MethodPost, "/api/v1/vulnerabilities/scans", "", operator)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"state":"running"`) || !strings.Contains(response.Body.String(), `"requestedBy":"otto"`) {
		t.Fatalf("operator scan = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/vulnerabilities/scans", "", admin); response.Code != http.StatusConflict || errorCode(t, response) != "SCAN_IN_PROGRESS" {
		t.Fatalf("second scan = %d %s", response.Code, response.Body)
	}
	response = call(router, http.MethodGet, "/api/v1/vulnerabilities/scans?limit=5", "", viewer)
	if response.Code != http.StatusOK || strings.Count(response.Body.String(), `"trigger"`) != 2 || !strings.Contains(response.Body.String(), `"trigger":"manual"`) {
		t.Fatalf("scans = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodGet, "/api/v1/audit?targetType=vulnerability_scan", "", admin); !strings.Contains(response.Body.String(), `"action":"start"`) {
		t.Fatalf("audit = %s", response.Body)
	}
}

func TestFindingsListFilterAndDetail(t *testing.T) {
	resetLoginRateLimit(t)
	router, repository := newVulnerabilitiesRouter(t)
	storeScan(t, repository)
	viewer := signIn(t, router, "admin", "admin-password-1")
	var page struct {
		Findings []map[string]any `json:"findings"`
		Total    int              `json:"total"`
		Limit    int              `json:"limit"`
		Offset   int              `json:"offset"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/vulnerabilities", "", viewer), &page)
	if page.Total != 3 || page.Limit != 100 || page.Findings[0]["vulnerabilityId"] != "CVE-2026-1000" || page.Findings[1]["severity"] != "high" {
		t.Fatalf("page = %+v", page)
	}
	first := page.Findings[0]
	if first["fixState"] != "fixed" || first["description"] != "Overflow." || first["resolvedAt"] != nil {
		t.Fatalf("first finding = %+v", first)
	}
	firstID := first["id"].(string)
	for query, want := range map[string]int{
		"?severity=critical,low": 2, "?fixable=true": 1, "?q=curl": 1, "?q=CVE-2026-2": 1, "?limit=1&offset=1": 3, "?state=resolved": 0,
	} {
		decodeBody(t, call(router, http.MethodGet, "/api/v1/vulnerabilities"+query, "", viewer), &page)
		if page.Total != want {
			t.Errorf("%s: total = %d, want %d", query, page.Total, want)
		}
	}
	for _, invalid := range []string{"?severity=severe", "?fixable=yes", "?limit=0", "?limit=501", "?offset=-1", "?state=all", "?sort=id", "?q=a&q=b"} {
		if response := call(router, http.MethodGet, "/api/v1/vulnerabilities"+invalid, "", viewer); response.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", invalid, response.Code)
		}
	}
	response := call(router, http.MethodGet, "/api/v1/vulnerabilities/"+firstID, "", viewer)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"fixedVersions":["3.5.1-2"]`) {
		t.Fatalf("detail = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodGet, "/api/v1/vulnerabilities/00000000-0000-0000-0000-000000000000", "", viewer); response.Code != http.StatusNotFound {
		t.Fatalf("unknown finding = %d", response.Code)
	}
	if response := call(router, http.MethodGet, "/api/v1/vulnerabilities/scans?limit=101", "", viewer); response.Code != http.StatusBadRequest {
		t.Fatalf("scans limit = %d", response.Code)
	}
}

func TestVulnerabilityRoutesWithoutTheModule(t *testing.T) {
	resetLoginRateLimit(t)
	services, _ := newTestServices(t, nil)
	router := NewRouter(zap.NewNop(), services, false, "")
	admin := signIn(t, router, "admin", "admin-password-1")
	if response := call(router, http.MethodGet, "/api/v1/vulnerabilities/summary", "", admin); response.Code != http.StatusServiceUnavailable || errorCode(t, response) != "VULNERABILITIES_UNAVAILABLE" {
		t.Fatalf("summary without the module = %d %s", response.Code, response.Body)
	}
}
