package usecase

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

// memorySessions is an in-memory ports.SessionRepository.
type memorySessions struct {
	sessions map[uuid.UUID]*domain.Session
	tokens   map[string]uuid.UUID
	revoked  map[uuid.UUID]bool
	touches  int
}

func newMemorySessions() *memorySessions {
	return &memorySessions{sessions: map[uuid.UUID]*domain.Session{}, tokens: map[string]uuid.UUID{}, revoked: map[uuid.UUID]bool{}}
}

func (m *memorySessions) add(user domain.User, token string, lastSeen time.Time) domain.Session {
	session := &domain.Session{ID: uuid.New(), UserID: user.ID, Username: user.Username, LastSeenAt: lastSeen, ExpiresAt: lastSeen.Add(12 * time.Hour)}
	m.sessions[session.ID] = session
	m.tokens[sha256Hex(token)] = session.ID
	return *session
}

func (m *memorySessions) Create(_ context.Context, userID uuid.UUID, tokenHash, _, _ string, expiresAt time.Time) (domain.Session, error) {
	session := &domain.Session{ID: uuid.New(), UserID: userID, LastSeenAt: time.Now(), ExpiresAt: expiresAt}
	m.sessions[session.ID] = session
	m.tokens[tokenHash] = session.ID
	return *session, nil
}

func (m *memorySessions) active(id uuid.UUID) (domain.Session, error) {
	session, ok := m.sessions[id]
	if !ok || m.revoked[id] {
		return domain.Session{}, domain.ErrSessionNotFound
	}
	return *session, nil
}

func (m *memorySessions) FindActiveByTokenHash(_ context.Context, tokenHash string) (domain.Session, error) {
	return m.active(m.tokens[tokenHash])
}

func (m *memorySessions) FindActiveByID(_ context.Context, id uuid.UUID) (domain.Session, error) {
	return m.active(id)
}

func (m *memorySessions) ListActive(_ context.Context, userID *uuid.UUID, _ time.Time, limit int) ([]domain.Session, error) {
	var sessions []domain.Session
	for id, session := range m.sessions {
		if !m.revoked[id] && (userID == nil || session.UserID == *userID) {
			sessions = append(sessions, *session)
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].Username < sessions[j].Username })
	if len(sessions) > limit {
		sessions = sessions[:limit]
	}
	return sessions, nil
}

func (m *memorySessions) Touch(_ context.Context, id uuid.UUID, at time.Time) error {
	m.touches++
	m.sessions[id].LastSeenAt = at
	return nil
}

func (m *memorySessions) RevokeByID(_ context.Context, id uuid.UUID) error {
	if _, err := m.active(id); err != nil {
		return err
	}
	m.revoked[id] = true
	return nil
}

func (m *memorySessions) RevokeByTokenHash(_ context.Context, tokenHash string) error {
	return m.RevokeByID(context.Background(), m.tokens[tokenHash])
}

func (m *memorySessions) RevokeAllForUser(_ context.Context, userID uuid.UUID) error {
	_, err := m.RevokeAllForUserExcept(context.Background(), userID, uuid.Nil)
	return err
}

func (m *memorySessions) RevokeAllForUserExcept(_ context.Context, userID, keep uuid.UUID) (int, error) {
	count := 0
	for id, session := range m.sessions {
		if session.UserID == userID && id != keep && !m.revoked[id] {
			m.revoked[id] = true
			count++
		}
	}
	return count, nil
}

// memoryFailures is an in-memory ports.LoginFailureRepository.
type memoryFailures struct {
	failures map[uuid.UUID]domain.LoginFailures
}

func (m *memoryFailures) LoginFailures(_ context.Context, userID uuid.UUID) (domain.LoginFailures, error) {
	return m.failures[userID], nil
}

func (m *memoryFailures) SaveLoginFailures(_ context.Context, userID uuid.UUID, failures domain.LoginFailures, _ time.Time) error {
	m.failures[userID] = failures
	return nil
}

func (m *memoryFailures) ClearLoginFailures(_ context.Context, userID uuid.UUID) error {
	delete(m.failures, userID)
	return nil
}

type fixedPolicy struct{ policy domain.SecurityPolicy }

func (f fixedPolicy) Policy() domain.SecurityPolicy { return f.policy }

// checkingHasher accepts only the password that was hashed.
type checkingHasher struct{ verified int }

func (h *checkingHasher) Hash(password string) (string, error) { return "hash:" + password, nil }
func (h *checkingHasher) Verify(password, encodedHash string) (bool, error) {
	h.verified++
	return encodedHash == "hash:"+password, nil
}

type usersByName map[string]domain.User

func (u usersByName) FindByUsername(_ context.Context, username string) (domain.User, string, error) {
	user, ok := u[username]
	if !ok {
		return domain.User{}, "", domain.ErrInvalidCredentials
	}
	return user, "hash:right-password", nil
}
func (u usersByName) FindByID(_ context.Context, id uuid.UUID) (domain.User, error) {
	for _, user := range u {
		if user.ID == id {
			return user, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}
func (u usersByName) UpdatePasswordHash(context.Context, uuid.UUID, string) error     { return nil }
func (u usersByName) EnsureAdmin(context.Context, string, string) (bool, error)       { return false, nil }
func (u usersByName) RecordLogin(context.Context, uuid.UUID, string, time.Time) error { return nil }

var (
	securityAdmin = domain.User{ID: uuid.New(), Username: "admin", Role: domain.RoleAdmin}
	securityAlice = domain.User{ID: uuid.New(), Username: "alice", Role: domain.RoleViewer}
	securityBob   = domain.User{ID: uuid.New(), Username: "bob", Role: domain.RoleViewer}
)

func newSecureAuth(sessions *memorySessions, policy domain.SecurityPolicy) (*AuthUsecase, *memoryFailures, *fakeAudit, *checkingHasher, *time.Time) {
	users := usersByName{"admin": securityAdmin, "alice": securityAlice, "bob": securityBob}
	failures := &memoryFailures{failures: map[uuid.UUID]domain.LoginFailures{}}
	audit := &fakeAudit{}
	hasher := &checkingHasher{}
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	auth := NewAuthUsecase(users, sessions, hasher).WithSecurity(failures, audit, fixedPolicy{policy})
	auth.now = func() time.Time { return now }
	return auth, failures, audit, hasher, &now
}

func TestLoginLocksTheAccountAfterFiveFailures(t *testing.T) {
	auth, failures, audit, hasher, now := newSecureAuth(newMemorySessions(), domain.DefaultSecurityPolicy())
	ctx := context.Background()
	for i := 1; i <= domain.MaxConsecutiveFailures; i++ {
		if _, _, _, err := auth.Login(ctx, "alice", "wrong", "192.0.2.1", "test"); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("failure %d = %v", i, err)
		}
	}
	if !failures.failures[securityAlice.ID].Locked(*now) {
		t.Fatal("five consecutive failures lock the account")
	}
	want := domain.AuditEntry{Actor: "system", TargetType: "user", TargetID: securityAlice.ID.String(), Action: "lock", Outcome: domain.AuditSuccess}
	if len(audit.entries) != 1 || audit.entries[0] != want {
		t.Fatalf("audit = %+v, want the lock", audit.entries)
	}

	before := hasher.verified
	if _, _, _, err := auth.Login(ctx, "alice", "right-password", "192.0.2.1", "test"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("right password while locked = %v, want ErrInvalidCredentials", err)
	}
	if hasher.verified != before+1 {
		t.Fatal("a locked account still costs one password verification")
	}

	*now = now.Add(domain.LockoutDuration)
	if _, _, _, err := auth.Login(ctx, "alice", "right-password", "192.0.2.1", "test"); err != nil {
		t.Fatalf("login after the lockout = %v", err)
	}
	if _, ok := failures.failures[securityAlice.ID]; ok {
		t.Fatal("a successful login clears the failures")
	}
}

func TestSessionExpiresAfterInactivityAndIsTouchedOncePerMinute(t *testing.T) {
	sessions := newMemorySessions()
	auth, _, _, _, now := newSecureAuth(sessions, domain.DefaultSecurityPolicy())
	ctx := context.Background()

	recent := sessions.add(securityAlice, "recent", now.Add(-30*time.Second))
	if _, _, err := auth.Session(ctx, "recent"); err != nil || sessions.touches != 0 {
		t.Fatalf("recent session = %v with %d touches, want no write", err, sessions.touches)
	}
	*now = now.Add(time.Minute)
	if _, session, err := auth.Session(ctx, "recent"); err != nil || sessions.touches != 1 || !session.LastSeenAt.Equal(*now) {
		t.Fatalf("session after a minute = (%+v, %v) with %d touches, want one write", session, err, sessions.touches)
	}

	idle := sessions.add(securityAlice, "idle", now.Add(-61*time.Minute))
	if _, _, err := auth.Session(ctx, "idle"); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("idle session = %v, want ErrSessionNotFound", err)
	}
	if !sessions.revoked[idle.ID] || sessions.revoked[recent.ID] {
		t.Fatal("only the idle session is revoked")
	}
}

func TestSessionListingAndRevocationRules(t *testing.T) {
	sessions := newMemorySessions()
	auth, _, audit, _, now := newSecureAuth(sessions, domain.DefaultSecurityPolicy())
	ctx := context.Background()
	adminCurrent := sessions.add(securityAdmin, "admin-1", *now)
	sessions.add(securityAdmin, "admin-2", *now)
	aliceSession := sessions.add(securityAlice, "alice-1", *now)
	bobSession := sessions.add(securityBob, "bob-1", *now)

	if mine, err := auth.ListSessions(ctx, securityAlice, false); err != nil || len(mine) != 1 || mine[0].ID != aliceSession.ID {
		t.Fatalf("alice sessions = (%+v, %v)", mine, err)
	}
	if _, err := auth.ListSessions(ctx, securityAlice, true); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("viewer listing every session = %v, want ErrForbidden", err)
	}
	if all, err := auth.ListSessions(ctx, securityAdmin, true); err != nil || len(all) != 4 {
		t.Fatalf("admin listing = (%d, %v), want 4", len(all), err)
	}

	if err := auth.RevokeSession(ctx, securityAlice, bobSession.ID, "req-1"); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("revoking another account's session = %v, want ErrSessionNotFound", err)
	}
	if sessions.revoked[bobSession.ID] {
		t.Fatal("a viewer cannot revoke another account's session")
	}
	if err := auth.RevokeSession(ctx, securityAdmin, bobSession.ID, "req-2"); err != nil {
		t.Fatalf("admin revoking bob = %v", err)
	}
	want := domain.AuditEntry{Actor: "admin", TargetType: "session", TargetID: bobSession.ID.String(), Action: "revoke", RequestID: "req-2", Outcome: domain.AuditSuccess}
	if len(audit.entries) != 1 || audit.entries[0] != want {
		t.Fatalf("audit = %+v, want %+v", audit.entries, want)
	}
	revoked, err := auth.RevokeOtherSessions(ctx, securityAdmin, adminCurrent.ID, "req-3")
	if err != nil || revoked != 1 || sessions.revoked[adminCurrent.ID] {
		t.Fatalf("revoke others = (%d, %v), current revoked=%v", revoked, err, sessions.revoked[adminCurrent.ID])
	}
}

// memorySecurity is an in-memory ports.SecurityRepository.
type memorySecurity struct {
	policy *domain.SecurityPolicy
	audits []domain.AuditEntry
}

func (m *memorySecurity) SecurityPolicy(context.Context) (domain.SecurityPolicy, error) {
	if m.policy == nil {
		return domain.DefaultSecurityPolicy(), nil
	}
	return *m.policy, nil
}

func (m *memorySecurity) SetSecurityPolicy(_ context.Context, policy domain.SecurityPolicy, audit domain.AuditEntry) error {
	m.policy = &policy
	m.audits = append(m.audits, audit)
	return nil
}

func TestSecurityPolicyUpdate(t *testing.T) {
	stored, err := domain.NewSecurityPolicy(30, nil)
	if err != nil {
		t.Fatal(err)
	}
	repository := &memorySecurity{policy: &stored}
	security := NewSecurityUsecase(repository)
	ctx := context.Background()
	if err := security.Load(ctx); err != nil || security.Policy().IdleTimeoutMinutes != 30 {
		t.Fatalf("Load = %v, policy %+v", err, security.Policy())
	}

	if _, err := security.UpdatePolicy(ctx, securityAdmin, "192.168.1.5", 60, []string{"10.0.0.0/8"}, ""); !errors.Is(err, domain.ErrPolicyExcludesCurrentAddress) {
		t.Fatalf("self-exclusion = %v, want ErrPolicyExcludesCurrentAddress", err)
	}
	if _, err := security.UpdatePolicy(ctx, securityAdmin, "10.1.2.3", 4, nil, ""); !errors.Is(err, domain.ErrInvalidSecurityPolicy) {
		t.Fatalf("invalid policy = %v, want ErrInvalidSecurityPolicy", err)
	}
	if len(repository.audits) != 0 {
		t.Fatal("refused changes are not stored")
	}
	updated, err := security.UpdatePolicy(ctx, securityAdmin, "10.1.2.3", 45, []string{"10.0.0.0/8"}, "req-1")
	if err != nil || updated.IdleTimeoutMinutes != 45 {
		t.Fatalf("UpdatePolicy = (%+v, %v)", updated, err)
	}
	if security.AllowsAddress("192.168.1.5") || !security.AllowsAddress("10.9.9.9") {
		t.Fatal("the new networks apply at once")
	}
	want := domain.AuditEntry{Actor: "admin", TargetType: "security_policy", TargetID: "local", Action: "update", RequestID: "req-1", Outcome: domain.AuditSuccess}
	if len(repository.audits) != 1 || repository.audits[0] != want {
		t.Fatalf("audit = %+v", repository.audits)
	}
}

type recordingAuditReader struct{ filters []domain.AuditFilter }

func (r *recordingAuditReader) ListAudit(_ context.Context, filter domain.AuditFilter) ([]domain.AuditRecord, error) {
	r.filters = append(r.filters, filter)
	return nil, nil
}

func TestAuditUsecaseBoundsAndDeniedEntries(t *testing.T) {
	reader := &recordingAuditReader{}
	recorder := &fakeAudit{}
	audit := NewAuditUsecase(reader, recorder)
	ctx := context.Background()

	if _, err := audit.List(ctx, domain.AuditFilter{}); err != nil || reader.filters[0].Limit != domain.DefaultAuditLimit {
		t.Fatalf("List = %v, filter %+v", err, reader.filters)
	}
	if _, err := audit.List(ctx, domain.AuditFilter{Limit: domain.MaxAuditLimit + 1}); !errors.Is(err, domain.ErrInvalidAuditFilter) {
		t.Fatalf("List over the limit = %v", err)
	}
	if _, err := audit.Export(ctx, domain.AuditFilter{}); err != nil || reader.filters[1].Limit != domain.MaxAuditExport {
		t.Fatalf("Export = %v, filter %+v", err, reader.filters)
	}

	long := "/api/v1/" + strings.Repeat("x", 400)
	if err := audit.RecordDenied(ctx, securityAlice, "GET", long, "req-9"); err != nil {
		t.Fatal(err)
	}
	entry := recorder.entries[0]
	if entry.Actor != "alice" || entry.TargetType != "route" || entry.Action != "access" || entry.Outcome != domain.AuditDenied || entry.RequestID != "req-9" || len(entry.TargetID) > 255 || !strings.HasPrefix(entry.TargetID, "GET /api/v1/") {
		t.Fatalf("denied entry = %+v", entry)
	}
}

func TestLastAdminRefusalIsAudited(t *testing.T) {
	recorder := &fakeAudit{}
	accounts := NewAccountUsecase(newFakeAccounts(testAdmin), &revokingSessions{}, &mockHasher{}).WithAudit(recorder)
	viewer := domain.RoleViewer
	if _, err := accounts.Update(context.Background(), testAdmin, testAdmin.ID, domain.UserChange{Role: &viewer}, "req-1"); !errors.Is(err, domain.ErrLastAdmin) {
		t.Fatalf("Update = %v, want ErrLastAdmin", err)
	}
	if err := accounts.Delete(context.Background(), testAdmin, testAdmin.ID, "req-2"); !errors.Is(err, domain.ErrLastAdmin) {
		t.Fatalf("Delete = %v, want ErrLastAdmin", err)
	}
	if len(recorder.entries) != 2 {
		t.Fatalf("denied entries = %+v, want 2", recorder.entries)
	}
	want := domain.AuditEntry{Actor: "admin", TargetType: "user", TargetID: testAdmin.ID.String(), Action: "update", RequestID: "req-1", Outcome: domain.AuditDenied}
	if recorder.entries[0] != want || recorder.entries[1].Action != "delete" {
		t.Fatalf("denied entries = %+v", recorder.entries)
	}
}
