package httpdelivery

import (
	"errors"
	"net/http"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type silenceDTO struct {
	ID        string     `json:"id"`
	Scope     string     `json:"scope"`
	RuleID    *string    `json:"ruleId"`
	RuleName  *string    `json:"ruleName"`
	TargetKey *string    `json:"targetKey"`
	Reason    string     `json:"reason"`
	CreatedBy string     `json:"createdBy"`
	StartsAt  time.Time  `json:"startsAt"`
	EndsAt    time.Time  `json:"endsAt"`
	LiftedAt  *time.Time `json:"liftedAt"`
	LiftedBy  *string    `json:"liftedBy"`
	Active    bool       `json:"active"`
}

type silenceRequest struct {
	Scope           string  `json:"scope"`
	RuleID          *string `json:"ruleId"`
	TargetKey       string  `json:"targetKey"`
	DurationMinutes int     `json:"durationMinutes"`
	Reason          string  `json:"reason"`
}

type channelSettingsDTO struct {
	ChatID    string   `json:"chatId,omitempty"`
	ServerURL string   `json:"serverUrl,omitempty"`
	Topic     string   `json:"topic,omitempty"`
	Host      string   `json:"host,omitempty"`
	Port      int      `json:"port,omitempty"`
	Security  string   `json:"security,omitempty"`
	Username  string   `json:"username,omitempty"`
	From      string   `json:"from,omitempty"`
	To        []string `json:"to,omitempty"`
	URL       string   `json:"url,omitempty"`
}

type secretDTO struct {
	Defined     bool    `json:"defined"`
	Fingerprint *string `json:"fingerprint"`
}

type channelDTO struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	Type            string               `json:"type"`
	Enabled         bool                 `json:"enabled"`
	MinSeverity     string               `json:"minSeverity"`
	RenotifyMinutes int                  `json:"renotifyMinutes"`
	Settings        channelSettingsDTO   `json:"settings"`
	Secrets         map[string]secretDTO `json:"secrets"`
	LastSentAt      *time.Time           `json:"lastSentAt"`
	LastErrorAt     *time.Time           `json:"lastErrorAt"`
	LastError       *string              `json:"lastError"`
	LastErrorStatus *int                 `json:"lastErrorStatus"`
	CreatedAt       time.Time            `json:"createdAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
}

type channelRequest struct {
	Name            string             `json:"name"`
	Type            string             `json:"type"`
	Enabled         *bool              `json:"enabled"`
	MinSeverity     string             `json:"minSeverity"`
	RenotifyMinutes *int               `json:"renotifyMinutes"`
	Settings        channelSettingsDTO `json:"settings"`
	// Secrets: an absent name keeps its value, null removes it.
	Secrets map[string]*string `json:"secrets"`
}

func toSilenceDTO(view usecase.SilenceView) silenceDTO {
	s := view.Silence
	dto := silenceDTO{
		ID: s.ID.String(), Scope: string(s.Scope), RuleName: optional(view.RuleName), TargetKey: optional(s.TargetKey),
		Reason: s.Reason, CreatedBy: s.CreatedBy, StartsAt: s.StartsAt, EndsAt: s.EndsAt, LiftedAt: s.LiftedAt,
		LiftedBy: optional(s.LiftedBy), Active: view.Active,
	}
	if s.RuleID != nil {
		dto.RuleID = optional(s.RuleID.String())
	}
	return dto
}

func toChannelDTO(view usecase.ChannelView) channelDTO {
	c := view.Channel
	s := c.Settings
	dto := channelDTO{
		ID: c.ID.String(), Name: c.Name, Type: string(c.Type), Enabled: c.Enabled, MinSeverity: string(c.MinSeverity),
		RenotifyMinutes: c.RenotifyMinutes,
		Settings: channelSettingsDTO{
			ChatID: s.ChatID, ServerURL: s.ServerURL, Topic: s.Topic, Host: s.Host, Port: s.Port, Security: string(s.Security),
			Username: s.Username, From: s.From, To: s.To, URL: s.URL,
		},
		Secrets:    map[string]secretDTO{},
		LastSentAt: c.LastSentAt, LastErrorAt: c.LastErrorAt, LastError: optional(string(c.LastError)),
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
	if c.LastErrorStatus != 0 {
		status := c.LastErrorStatus
		dto.LastErrorStatus = &status
	}
	for name, secret := range view.Secrets {
		dto.Secrets[name] = secretDTO{Defined: secret.Defined, Fingerprint: optional(secret.Fingerprint)}
	}
	return dto
}

// requireNotifications answers 503 when the alert engine is not running.
func requireNotifications(notifications *usecase.NotificationUsecase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if notifications == nil {
				writeError(w, r, http.StatusServiceUnavailable, "ALERTS_UNAVAILABLE", "Alerts are unavailable.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func listSilencesHandler(notifications *usecase.NotificationUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		silences, err := notifications.Silences(r.Context())
		if err != nil {
			writeNotificationError(w, r, err)
			return
		}
		response := make([]silenceDTO, 0, len(silences))
		for _, silence := range silences {
			response = append(response, toSilenceDTO(silence))
		}
		writeJSON(w, http.StatusOK, map[string]any{"silences": response})
	}
}

func createSilenceHandler(notifications *usecase.NotificationUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request silenceRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The request body is invalid.")
			return
		}
		input := domain.SilenceRequest{
			Scope: domain.SilenceScope(request.Scope), TargetKey: request.TargetKey,
			Duration: time.Duration(request.DurationMinutes) * time.Minute, Reason: request.Reason,
		}
		if request.RuleID != nil {
			id, err := uuid.Parse(*request.RuleID)
			if err != nil || !lowercaseUUIDPattern.MatchString(*request.RuleID) {
				writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "ruleId must be a lowercase UUID.")
				return
			}
			input.RuleID = &id
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		silence, err := notifications.CreateSilence(r.Context(), actor, input, middleware.GetReqID(r.Context()))
		if err != nil {
			writeNotificationError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"silence": toSilenceDTO(silence)})
	}
}

func liftSilenceHandler(notifications *usecase.NotificationUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := uuidParam(w, r, "silenceID")
		if !ok {
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		silence, err := notifications.LiftSilence(r.Context(), actor, id, middleware.GetReqID(r.Context()))
		if err != nil {
			writeNotificationError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"silence": toSilenceDTO(silence)})
	}
}

func listChannelsHandler(notifications *usecase.NotificationUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channels, err := notifications.Channels(r.Context())
		if err != nil {
			writeNotificationError(w, r, err)
			return
		}
		response := make([]channelDTO, 0, len(channels))
		for _, channel := range channels {
			response = append(response, toChannelDTO(channel))
		}
		writeJSON(w, http.StatusOK, map[string]any{"channels": response, "secretsAvailable": notifications.SecretsAvailable()})
	}
}

// decodeChannel reads the request body; false means a response was written.
func decodeChannel(w http.ResponseWriter, r *http.Request) (usecase.ChannelInput, domain.User, bool) {
	var request channelRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The request body is invalid.")
		return usecase.ChannelInput{}, domain.User{}, false
	}
	if request.Enabled == nil || request.RenotifyMinutes == nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "enabled and renotifyMinutes are required.")
		return usecase.ChannelInput{}, domain.User{}, false
	}
	actor, ok := sessionUser(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
		return usecase.ChannelInput{}, domain.User{}, false
	}
	s := request.Settings
	return usecase.ChannelInput{
		Name: request.Name, Type: domain.ChannelType(request.Type), Enabled: *request.Enabled,
		MinSeverity: domain.AlertSeverity(request.MinSeverity), RenotifyMinutes: *request.RenotifyMinutes,
		Settings: domain.ChannelSettings{
			ChatID: s.ChatID, ServerURL: s.ServerURL, Topic: s.Topic, Host: s.Host, Port: s.Port, Security: domain.SMTPSecurity(s.Security),
			Username: s.Username, From: s.From, To: s.To, URL: s.URL,
		},
		Secrets: request.Secrets,
	}, actor, true
}

func createChannelHandler(notifications *usecase.NotificationUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		input, actor, ok := decodeChannel(w, r)
		if !ok {
			return
		}
		channel, err := notifications.CreateChannel(r.Context(), actor, input, middleware.GetReqID(r.Context()))
		if err != nil {
			writeNotificationError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"channel": toChannelDTO(channel)})
	}
}

func updateChannelHandler(notifications *usecase.NotificationUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := uuidParam(w, r, "channelID")
		if !ok {
			return
		}
		input, actor, ok := decodeChannel(w, r)
		if !ok {
			return
		}
		channel, err := notifications.UpdateChannel(r.Context(), actor, id, input, middleware.GetReqID(r.Context()))
		if err != nil {
			writeNotificationError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"channel": toChannelDTO(channel)})
	}
}

func deleteChannelHandler(notifications *usecase.NotificationUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := uuidParam(w, r, "channelID")
		if !ok {
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		if err := notifications.DeleteChannel(r.Context(), actor, id, middleware.GetReqID(r.Context())); err != nil {
			writeNotificationError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

func testChannelHandler(notifications *usecase.NotificationUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := uuidParam(w, r, "channelID")
		if !ok {
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		channel, err := notifications.TestChannel(r.Context(), actor, id, middleware.GetReqID(r.Context()))
		if err != nil {
			writeNotificationError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"channel": toChannelDTO(channel)})
	}
}

func writeNotificationError(w http.ResponseWriter, r *http.Request, err error) {
	var delivery *domain.DeliveryError
	switch {
	case errors.As(err, &delivery):
		// The code and status only: never the address nor the answer.
		writeError(w, r, http.StatusBadGateway, "NOTIFICATION_FAILED", delivery.Error()+".")
	case errors.Is(err, domain.ErrInvalidSilence), errors.Is(err, domain.ErrInvalidChannel):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, domain.ErrSilenceNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The alert silence does not exist.")
	case errors.Is(err, domain.ErrChannelNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The notification channel does not exist.")
	case errors.Is(err, domain.ErrAlertRuleNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The alert rule does not exist.")
	case errors.Is(err, domain.ErrTooManySilences):
		writeError(w, r, http.StatusConflict, "TOO_MANY_SILENCES", "At most 100 silences in force.")
	case errors.Is(err, domain.ErrTooManyChannels):
		writeError(w, r, http.StatusConflict, "TOO_MANY_CHANNELS", "At most 20 notification channels.")
	case errors.Is(err, domain.ErrSecretsUnavailable):
		writeError(w, r, http.StatusServiceUnavailable, "SECRETS_UNAVAILABLE", "The secrets key is unavailable.")
	default:
		writeError(w, r, http.StatusInternalServerError, "ALERTS_FAILED", "Could not read or update the notifications.")
	}
}
