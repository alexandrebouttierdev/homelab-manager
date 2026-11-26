package usecase

import (
	"context"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"
)

// maxRouteBytes bounds the route stored in a denied entry.
const maxRouteBytes = 255

// AuditUsecase reads the audit log for administrators and records denied
// accesses (SPEC_ADMINISTRATION.md §§ 2.2, 3.5).
type AuditUsecase struct {
	reader   ports.AuditReader
	recorder ports.AuditRecorder
}

func NewAuditUsecase(reader ports.AuditReader, recorder ports.AuditRecorder) *AuditUsecase {
	return &AuditUsecase{reader: reader, recorder: recorder}
}

// List returns at most domain.MaxAuditLimit records, newest first.
func (u *AuditUsecase) List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditRecord, error) {
	filter, err := filter.Validate(domain.MaxAuditLimit)
	if err != nil {
		return nil, err
	}
	return u.reader.ListAudit(ctx, filter)
}

// Export returns at most domain.MaxAuditExport records; a zero limit means
// the maximum.
func (u *AuditUsecase) Export(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditRecord, error) {
	if filter.Limit == 0 {
		filter.Limit = domain.MaxAuditExport
	}
	filter, err := filter.Validate(domain.MaxAuditExport)
	if err != nil {
		return nil, err
	}
	return u.reader.ListAudit(ctx, filter)
}

// RecordTokenAction traces an action requested with an API token, before
// it runs: the audit of the action itself follows with the same request id.
func (u *AuditUsecase) RecordTokenAction(ctx context.Context, actor domain.User, token domain.APIToken, requestID string) error {
	return u.recorder.RecordAudit(ctx, domain.AuditEntry{
		Actor: actor.Username, TargetType: "api_token", TargetID: token.ID.String(),
		Action: "act", RequestID: requestID, Outcome: domain.AuditRequested,
	})
}

// RecordDenied traces a request refused because of the role of its account.
func (u *AuditUsecase) RecordDenied(ctx context.Context, actor domain.User, method, path, requestID string) error {
	route := method + " " + path
	if len(route) > maxRouteBytes {
		route = route[:maxRouteBytes]
	}
	return u.recorder.RecordAudit(ctx, domain.AuditEntry{
		Actor: actor.Username, TargetType: "route", TargetID: route,
		Action: "access", RequestID: requestID, Outcome: domain.AuditDenied,
	})
}
