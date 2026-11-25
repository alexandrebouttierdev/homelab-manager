package httpdelivery

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// maxUserAgentBytes bounds the browser description returned for a session.
const maxUserAgentBytes = 256

// denyRecorder traces a request refused because of the role of its account.
type denyRecorder func(r *http.Request, user domain.User)

type currentSessionKey struct{}

// currentSession returns the session authenticated by requireSession.
func currentSession(r *http.Request) (domain.Session, bool) {
	session, ok := r.Context().Value(currentSessionKey{}).(domain.Session)
	return session, ok
}

// allowedNetworks refuses clients outside the networks of the security
// policy. /health stays reachable for the local supervision.
func allowedNetworks(security *usecase.SecurityUsecase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if security != nil && r.URL.Path != "/health" && !security.AllowsAddress(clientIP(r)) {
				writeError(w, r, http.StatusForbidden, "NETWORK_NOT_ALLOWED", "The client address is not allowed.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type sessionDTO struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"userId"`
	Username   string    `json:"username"`
	IPAddress  string    `json:"ipAddress"`
	UserAgent  string    `json:"userAgent"`
	AuthMethod string    `json:"authMethod"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Current    bool      `json:"current"`
}

func listSessionsHandler(auth *usecase.AuthUsecase, deny denyRecorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("scope")
		if scope != "" && scope != "all" {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "scope must be all or absent.")
			return
		}
		user, ok := sessionUser(r)
		current, hasCurrent := currentSession(r)
		if !ok || !hasCurrent {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		sessions, err := auth.ListSessions(r.Context(), user, scope == "all")
		if errors.Is(err, domain.ErrForbidden) {
			deny(r, user)
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "The account role does not allow this action.")
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "SESSIONS_UNAVAILABLE", "Could not list the sessions.")
			return
		}
		dtos := make([]sessionDTO, 0, len(sessions))
		for _, session := range sessions {
			dtos = append(dtos, sessionDTO{
				ID: session.ID, UserID: session.UserID, Username: session.Username,
				IPAddress: session.IPAddress, UserAgent: truncateUTF8(session.UserAgent, maxUserAgentBytes),
				AuthMethod: session.AuthMethod, CreatedAt: session.CreatedAt,
				LastSeenAt: session.LastSeenAt, ExpiresAt: session.ExpiresAt,
				Current: session.ID == current.ID,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": dtos})
	}
}

func revokeSessionHandler(auth *usecase.AuthUsecase, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := uuidParam(w, r, "sessionID")
		if !ok {
			return
		}
		user, ok := sessionUser(r)
		current, hasCurrent := currentSession(r)
		if !ok || !hasCurrent {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		err := auth.RevokeSession(r.Context(), user, id, middleware.GetReqID(r.Context()))
		if errors.Is(err, domain.ErrSessionNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The session does not exist.")
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "SESSION_REVOKE_FAILED", "Could not revoke the session.")
			return
		}
		if id == current.ID {
			http.SetCookie(w, sessionCookie(sessionCookieName, "", time.Unix(0, 0), secure))
		}
		writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
	}
}

func revokeOtherSessionsHandler(auth *usecase.AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := sessionUser(r)
		current, hasCurrent := currentSession(r)
		if !ok || !hasCurrent {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		count, err := auth.RevokeOtherSessions(r.Context(), user, current.ID, middleware.GetReqID(r.Context()))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "SESSION_REVOKE_FAILED", "Could not revoke the sessions.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"revoked": count})
	}
}

type securityPolicyDTO struct {
	IdleTimeoutMinutes       int      `json:"idleTimeoutMinutes"`
	AllowedNetworks          []string `json:"allowedNetworks"`
	RequireAdminSecondFactor bool     `json:"requireAdminSecondFactor"`
	AllowPasskeyLogin        bool     `json:"allowPasskeyLogin"`
	// CurrentAddress lets the interface warn before a self-exclusion.
	CurrentAddress string `json:"currentAddress"`
}

type securityPolicyRequest struct {
	IdleTimeoutMinutes *int      `json:"idleTimeoutMinutes"`
	AllowedNetworks    *[]string `json:"allowedNetworks"`
	// RequireAdminSecondFactor keeps its current value when absent.
	RequireAdminSecondFactor *bool `json:"requireAdminSecondFactor"`
	// AllowPasskeyLogin keeps its current value when absent.
	AllowPasskeyLogin *bool `json:"allowPasskeyLogin"`
}

func securityPolicyHandler(security *usecase.SecurityUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, toSecurityPolicyDTO(security.Policy(), clientIP(r)))
	}
}

func updateSecurityPolicyHandler(security *usecase.SecurityUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request securityPolicyRequest
		if err := decodeJSON(r, &request); err != nil || request.IdleTimeoutMinutes == nil || request.AllowedNetworks == nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "idleTimeoutMinutes and allowedNetworks are required.")
			return
		}
		if _, err := domain.NewSecurityPolicy(*request.IdleTimeoutMinutes, *request.AllowedNetworks); err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The security policy values are invalid.")
			return
		}
		user, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		address := clientIP(r)
		current := security.Policy()
		requireSecondFactor, allowPasskey := current.RequireAdminSecondFactor, current.AllowPasskeyLogin
		if request.RequireAdminSecondFactor != nil {
			requireSecondFactor = *request.RequireAdminSecondFactor
		}
		if request.AllowPasskeyLogin != nil {
			allowPasskey = *request.AllowPasskeyLogin
		}
		policy, err := security.UpdatePolicy(r.Context(), user, address, *request.IdleTimeoutMinutes, *request.AllowedNetworks, requireSecondFactor, allowPasskey, middleware.GetReqID(r.Context()))
		switch {
		case errors.Is(err, domain.ErrSecondFactorNotEnrolled):
			writeError(w, r, http.StatusConflict, "SECOND_FACTOR_NOT_ENROLLED", "Enroll your own second factor before requiring it.")
		case errors.Is(err, domain.ErrSecondFactorUnavailable):
			writeError(w, r, http.StatusServiceUnavailable, "SECOND_FACTOR_UNAVAILABLE", "The second factor is unavailable.")
		case errors.Is(err, domain.ErrPolicyExcludesCurrentAddress):
			writeError(w, r, http.StatusConflict, "POLICY_EXCLUDES_CURRENT_ADDRESS", "The policy would exclude the current address.")
		case errors.Is(err, domain.ErrInvalidSecurityPolicy):
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The security policy values are invalid.")
		case err != nil:
			writeError(w, r, http.StatusInternalServerError, "SECURITY_POLICY_UPDATE_FAILED", "Could not save the security policy.")
		default:
			writeJSON(w, http.StatusOK, toSecurityPolicyDTO(policy, address))
		}
	}
}

func toSecurityPolicyDTO(policy domain.SecurityPolicy, address string) securityPolicyDTO {
	return securityPolicyDTO{
		IdleTimeoutMinutes: policy.IdleTimeoutMinutes, AllowedNetworks: policy.Networks(),
		RequireAdminSecondFactor: policy.RequireAdminSecondFactor, AllowPasskeyLogin: policy.AllowPasskeyLogin,
		CurrentAddress: address,
	}
}

type auditDTO struct {
	ID         uuid.UUID `json:"id"`
	Actor      string    `json:"actor"`
	TargetType string    `json:"targetType"`
	TargetID   string    `json:"targetId"`
	Action     string    `json:"action"`
	Outcome    string    `json:"outcome"`
	RequestID  *string   `json:"requestId"`
	CreatedAt  time.Time `json:"createdAt"`
}

func toAuditDTOs(records []domain.AuditRecord) []auditDTO {
	dtos := make([]auditDTO, 0, len(records))
	for _, record := range records {
		dto := auditDTO{
			ID: record.ID, Actor: record.Actor, TargetType: record.TargetType, TargetID: record.TargetID,
			Action: record.Action, Outcome: record.Outcome, CreatedAt: record.CreatedAt,
		}
		if record.RequestID != "" {
			requestID := record.RequestID
			dto.RequestID = &requestID
		}
		dtos = append(dtos, dto)
	}
	return dtos
}

func listAuditHandler(audit *usecase.AuditUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := auditFilter(r, domain.MaxAuditLimit)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The audit filter is invalid.")
			return
		}
		records, err := audit.List(r.Context(), filter)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "AUDIT_UNAVAILABLE", "Could not read the audit log.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"entries": toAuditDTOs(records)})
	}
}

var auditCSVHeader = []string{"createdAt", "actor", "targetType", "targetId", "action", "outcome", "requestId"}

func exportAuditHandler(audit *usecase.AuditUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")
		if format != "csv" && format != "json" {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "format must be csv or json.")
			return
		}
		filter, err := auditFilter(r, domain.MaxAuditExport)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The audit filter is invalid.")
			return
		}
		records, err := audit.Export(r.Context(), filter)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "AUDIT_UNAVAILABLE", "Could not read the audit log.")
			return
		}
		name := "audit-" + time.Now().UTC().Format("20060102T150405Z") + "." + format
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
		if format == "json" {
			writeJSON(w, http.StatusOK, toAuditDTOs(records))
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		writer := csv.NewWriter(w)
		_ = writer.Write(auditCSVHeader)
		for _, record := range records {
			_ = writer.Write([]string{
				record.CreatedAt.UTC().Format(time.RFC3339Nano), csvCell(record.Actor), csvCell(record.TargetType),
				csvCell(record.TargetID), csvCell(record.Action), csvCell(record.Outcome), csvCell(record.RequestID),
			})
		}
		writer.Flush()
	}
}

// auditFilter reads and validates the filter query parameters; a missing
// limit means the default of the list, or maxLimit for an export.
func auditFilter(r *http.Request, maxLimit int) (domain.AuditFilter, error) {
	query := r.URL.Query()
	filter := domain.AuditFilter{
		Actor: query.Get("actor"), TargetType: query.Get("targetType"),
		Action: query.Get("action"), Outcome: query.Get("outcome"),
	}
	for name, destination := range map[string]**time.Time{"from": &filter.From, "to": &filter.To} {
		if value := query.Get(name); value != "" {
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return domain.AuditFilter{}, err
			}
			*destination = &parsed
		}
	}
	if value := query.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil {
			return domain.AuditFilter{}, err
		}
		filter.Limit = limit
	} else if maxLimit == domain.MaxAuditExport {
		filter.Limit = maxLimit
	}
	return filter.Validate(maxLimit)
}

// csvCell neutralises a value a spreadsheet would read as a formula.
func csvCell(value string) string {
	if value != "" && strings.ContainsRune("=+-@\t\r", rune(value[0])) {
		return "'" + value
	}
	return value
}

// uuidParam validates a path parameter holding a lowercase UUID.
func uuidParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	value := chi.URLParam(r, name)
	id, err := uuid.Parse(value)
	if !userIDPattern.MatchString(value) || err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", name+" must be a lowercase UUID.")
		return uuid.UUID{}, false
	}
	return id, true
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
