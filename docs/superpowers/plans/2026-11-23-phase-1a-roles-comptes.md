# Phase 1A — Rôles et comptes multiples : plan d'implémentation

> **Pour les agents :** sous-skill requis : `superpowers:subagent-driven-development` (recommandé) ou `superpowers:executing-plans` pour exécuter ce plan tâche par tâche. Les étapes utilisent des cases à cocher (`- [ ]`) pour le suivi.

**Objectif :** permettre plusieurs comptes Homelab Manager avec les rôles `viewer`, `operator` et `admin`, gérés par les administrateurs, avec un mot de passe initial à changer à la première connexion.

**Architecture :** la politique rôle → permission vit dans le domaine (`Role.Can`) et un middleware Chi l'applique aux routes. Un nouveau `AccountUsecase` porte les règles (validation, dernier administrateur actif, révocation des sessions à la désactivation) et sérialise les modifications de comptes par un mutex ; un `AccountRepository` SQLite écrit chaque modification et sa ligne d'audit dans une même transaction. L'interface ajoute une page « Utilisateurs & accès » réservée aux administrateurs et un écran de changement de mot de passe imposé.

**Stack :** Go, Chi v5, Ent v0.14.6, SQLite `modernc.org/sqlite` ; React, TypeScript strict, TanStack Query, React Hook Form, Zod, Vitest.

**Feuille de route :** [2026-11-23-feuille-de-route-v3.md](./2026-11-23-feuille-de-route-v3.md). Spécification : `docs/SPEC_ADMINISTRATION.md` §§ 3.1–3.2.

## Contraintes globales

Copiées de `AGENTS.md`, `docs/CODE_RULES.md` et `docs/SPEC_ADMINISTRATION.md` ; elles s'appliquent à toutes les tâches.

- Comptes de Homelab Manager uniquement : aucun compte Linux, groupe, PAM, `sudo` ni SSH n'est lu ou modifié.
- Identifiant d'un compte créé depuis l'interface : `^[a-z0-9._-]{1,64}$`. Mot de passe : 12 à 1 024 octets. Le dernier administrateur actif ne peut être ni supprimé, ni rétrogradé, ni désactivé.
- Rôles (§ 3.2) : `viewer` consulte ; `operator` consulte (ses actions arrivent en phases 5 et 6) ; `admin` consulte, règle les paramètres et gère les comptes. Chacun gère son propre mot de passe.
- Hexagonal : le domaine n'importe ni Chi, ni Ent, ni SQLite ; Ent reste dans `internal/adapters/sqlite` ; aucune règle métier dans les handlers ; jamais d'entité Ent en DTO.
- Nouvelle migration numérotée `003_user_accounts.sql` ; ne jamais modifier `001` ni `002`. Régénérer Ent avec `go run entgo.io/ent/cmd/ent@v0.14.6 generate ./ent/schema` (depuis `backend/`).
- Audit : chaque modification de compte réussie écrit une ligne `audit_logs` dans la transaction qui la fait (`target_type = user`, actions `create`, `update`, `delete`, `outcome = success`). Les refus sont tracés dans les journaux zap en 1A (question Q-3 de la feuille de route).
- Code, identifiants, commentaires, messages d'API et commits en anglais ; documentation et interface en français. Conventional Commits.
- Interface : aucun code métier dans les `.tsx` ; un hook `use<Page>.ts` par page ; fonctions pures dans `src/lib/` avec tests ; couleurs par jetons Tailwind ; chaque champ a un `label` ; erreurs en `role="alert"`, confirmations en `role="status"`.
- Aucune dépendance nouvelle.
- `scripts/verify.sh` passe avant chaque commit. `API_CONTRACTS.md`, `DATA_MODEL.md`, `SECURITY.md` et `TESTING.md` sont mis à jour dans le commit du code qu'ils décrivent.
- Hors périmètre 1A (feuille de route) : sessions consultables, verrouillage, politique, audit consultable (1B) ; second facteur (1C) ; réinitialisation du mot de passe d'un autre compte (Q-1) ; changement forcé pour `admin`/`admin` (Q-2).

## Carte des fichiers

| Fichier | Rôle |
|---|---|
| `backend/internal/domain/role.go` (nouveau) | `Role`, `Permission`, politique `Role.Can` |
| `backend/internal/domain/account.go` (nouveau) | `NewUser`, `UserChange`, validations, erreurs de saisie |
| `backend/internal/domain/auth.go` | `User` enrichi, erreurs `ErrForbidden`, `ErrUserNotFound`, `ErrUsernameTaken`, `ErrLastAdmin` |
| `backend/migrations/003_user_accounts.sql` (nouveau) | Colonnes de compte sur `users` |
| `backend/ent/schema/user.go` + code Ent régénéré | Champs Ent correspondants |
| `backend/internal/adapters/sqlite/transaction.go` (nouveau) | `withTx`, `writeAudit` partagés |
| `backend/internal/adapters/sqlite/account_repository.go` (nouveau) | CRUD des comptes + audit transactionnel |
| `backend/internal/adapters/sqlite/auth_repository.go` | Mappage complet, `RecordLogin`, `EnsureAdmin` administrateur |
| `backend/internal/adapters/sqlite/settings_repository.go` | Utilise `writeAudit` |
| `backend/internal/ports/auth.go`, `ports/accounts.go` (nouveau) | `RecordLogin`, `AccountRepository` |
| `backend/internal/usecase/auth.go` | Comptes désactivés refusés, dernière connexion |
| `backend/internal/usecase/accounts.go` (nouveau) | `AccountUsecase` |
| `backend/internal/delivery/http/access.go` (nouveau) | Middlewares `requirePermission`, `requirePasswordChanged`, DTO de session |
| `backend/internal/delivery/http/users.go` (nouveau) | Handlers `/api/v1/users` |
| `backend/internal/delivery/http/auth.go`, `router.go` | Réponses de session, routes, journal avec statut |
| `backend/cmd/manager/main.go` | Composition |
| `frontend/src/lib/api/auth.ts`, `lib/api/users.ts` (nouveau), `lib/api/client.ts` | Types, client des comptes, messages d'erreur |
| `frontend/src/lib/roles.ts`, `lib/accounts.ts` (nouveaux) | Libellés, permissions d'affichage, lignes du tableau |
| `frontend/src/schemas/account.ts` (nouveau) | Formulaire de création |
| `frontend/src/hooks/useSession.ts` | Rôle, permissions, changement imposé |
| `frontend/src/app/RequireAuth.tsx`, `app/router.tsx` | Redirection, routes |
| `frontend/src/features/auth/ForcedPasswordChangePage.tsx` + `useForcedPasswordChange.ts` (nouveaux) | Écran imposé |
| `frontend/src/features/users/UsersPage.tsx` + `useUsersPage.ts` (nouveaux) | Page d'administration |
| `frontend/src/components/layout/AppSidebar.tsx`, `features/settings/*` | Navigation, paramètres en lecture seule |
| `docs/*.md`, `README.md` | Contrat, modèle, sécurité, tests, état |

---

### Tâche 1 : domaine — rôles, permissions et règles de compte

**Fichiers :**
- Créer : `backend/internal/domain/role.go`, `backend/internal/domain/account.go`
- Modifier : `backend/internal/domain/auth.go`
- Tests : `backend/internal/domain/role_test.go`, `backend/internal/domain/account_test.go`

**Interfaces :**
- Consomme : rien.
- Produit :
  - `type Role string` ; `RoleViewer`, `RoleOperator`, `RoleAdmin` ; `func (r Role) Valid() bool` ; `func (r Role) Can(p Permission) bool`.
  - `type Permission string` ; `PermManageSettings`, `PermManageUsers`.
  - `type User struct { ID uuid.UUID; Username, DisplayName string; Role Role; Disabled, MustChangePassword bool; LastLoginAt *time.Time; LastLoginIP string; CreatedAt time.Time }` ; `func (u User) IsActiveAdmin() bool`.
  - `type NewUser struct { Username, DisplayName string; Role Role }` ; `func (n NewUser) Normalize() (NewUser, error)`.
  - `type UserChange struct { DisplayName *string; Role *Role; Disabled *bool }` ; `Normalize() (UserChange, error)` ; `Apply(User) User`.
  - `func ValidateNewPassword(password string) error` ; `func IsValidationError(err error) bool`.
  - Erreurs : `ErrForbidden`, `ErrUserNotFound`, `ErrUsernameTaken`, `ErrLastAdmin`, `ErrInvalidUsername`, `ErrInvalidDisplayName`, `ErrInvalidRole`, `ErrInvalidPassword`, `ErrEmptyChange`.

- [ ] **Étape 1 : écrire les tests qui échouent**

`backend/internal/domain/role_test.go` :

```go
package domain

import "testing"

func TestRolePolicy(t *testing.T) {
	cases := []struct {
		role       Role
		permission Permission
		want       bool
	}{
		{RoleViewer, PermManageSettings, false},
		{RoleViewer, PermManageUsers, false},
		{RoleOperator, PermManageSettings, false},
		{RoleOperator, PermManageUsers, false},
		{RoleAdmin, PermManageSettings, true},
		{RoleAdmin, PermManageUsers, true},
		{Role("root"), PermManageUsers, false},
	}
	for _, c := range cases {
		if got := c.role.Can(c.permission); got != c.want {
			t.Errorf("%s.Can(%s) = %v, want %v", c.role, c.permission, got, c.want)
		}
	}
}

func TestRoleValid(t *testing.T) {
	for _, role := range []Role{RoleViewer, RoleOperator, RoleAdmin} {
		if !role.Valid() {
			t.Errorf("%q should be valid", role)
		}
	}
	for _, role := range []Role{"", "root", "Admin"} {
		if role.Valid() {
			t.Errorf("%q should be invalid", role)
		}
	}
}
```

`backend/internal/domain/account_test.go` :

```go
package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewUserNormalizeFillsTheDisplayName(t *testing.T) {
	user, err := NewUser{Username: "alice", DisplayName: "  ", Role: RoleViewer}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if user.DisplayName != "alice" {
		t.Fatalf("display name = %q, want the username", user.DisplayName)
	}
	trimmed, err := NewUser{Username: "bob", DisplayName: "  Bob  ", Role: RoleAdmin}.Normalize()
	if err != nil || trimmed.DisplayName != "Bob" {
		t.Fatalf("Normalize = (%+v, %v), want trimmed display name", trimmed, err)
	}
}

func TestNewUserNormalizeRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name  string
		input NewUser
		want  error
	}{
		{"uppercase username", NewUser{Username: "Alice", Role: RoleViewer}, ErrInvalidUsername},
		{"empty username", NewUser{Role: RoleViewer}, ErrInvalidUsername},
		{"username too long", NewUser{Username: strings.Repeat("a", 65), Role: RoleViewer}, ErrInvalidUsername},
		{"unknown role", NewUser{Username: "alice", Role: "root"}, ErrInvalidRole},
		{"control character", NewUser{Username: "alice", DisplayName: "Al\x00ice", Role: RoleViewer}, ErrInvalidDisplayName},
		{"display name too long", NewUser{Username: "alice", DisplayName: strings.Repeat("é", 65), Role: RoleViewer}, ErrInvalidDisplayName},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.input.Normalize()
			if !errors.Is(err, c.want) {
				t.Fatalf("Normalize error = %v, want %v", err, c.want)
			}
			if !IsValidationError(err) {
				t.Fatalf("%v should be a validation error", err)
			}
		})
	}
}

func TestUserChangeNormalize(t *testing.T) {
	if _, err := (UserChange{}).Normalize(); !errors.Is(err, ErrEmptyChange) {
		t.Fatalf("empty change error = %v, want ErrEmptyChange", err)
	}
	blank := "   "
	if _, err := (UserChange{DisplayName: &blank}).Normalize(); !errors.Is(err, ErrInvalidDisplayName) {
		t.Fatalf("blank display name error = %v, want ErrInvalidDisplayName", err)
	}
	root := Role("root")
	if _, err := (UserChange{Role: &root}).Normalize(); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("unknown role error = %v, want ErrInvalidRole", err)
	}
	padded := "  Alice "
	change, err := (UserChange{DisplayName: &padded}).Normalize()
	if err != nil || *change.DisplayName != "Alice" {
		t.Fatalf("Normalize = (%v, %v), want trimmed display name", change.DisplayName, err)
	}
}

func TestUserChangeApplyAndActiveAdmin(t *testing.T) {
	admin := User{Username: "admin", Role: RoleAdmin}
	if !admin.IsActiveAdmin() {
		t.Fatal("an enabled admin is an active admin")
	}
	viewer := RoleViewer
	if (UserChange{Role: &viewer}).Apply(admin).IsActiveAdmin() {
		t.Fatal("a demoted admin is no longer an active admin")
	}
	disabled := true
	if (UserChange{Disabled: &disabled}).Apply(admin).IsActiveAdmin() {
		t.Fatal("a disabled admin is no longer an active admin")
	}
	name := "Root"
	if got := (UserChange{DisplayName: &name}).Apply(admin); !got.IsActiveAdmin() || got.DisplayName != "Root" {
		t.Fatalf("renaming must keep the role: %+v", got)
	}
}

func TestValidateNewPassword(t *testing.T) {
	cases := []struct {
		password string
		valid    bool
	}{
		{strings.Repeat("a", 11), false},
		{strings.Repeat("a", 12), true},
		{strings.Repeat("a", 1024), true},
		{strings.Repeat("a", 1025), false},
	}
	for _, c := range cases {
		err := ValidateNewPassword(c.password)
		if (err == nil) != c.valid {
			t.Errorf("ValidateNewPassword(%d bytes) = %v, want valid=%v", len(c.password), err, c.valid)
		}
	}
}
```

- [ ] **Étape 2 : lancer les tests et constater l'échec**

Commande : `go -C backend test ./internal/domain/ -run 'TestRole|TestNewUser|TestUserChange|TestValidateNewPassword' -count=1`
Attendu : échec de compilation, `undefined: Role`, `undefined: NewUser`, etc.

- [ ] **Étape 3 : écrire l'implémentation minimale**

`backend/internal/domain/role.go` :

```go
package domain

// Role is the access level of a Manager account (SPEC_ADMINISTRATION.md § 3.2).
type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

// Permission names an action reserved to some roles. Reading the
// monitoring data and managing one's own password need no permission.
type Permission string

const (
	// PermManageSettings covers the settings changed from the interface.
	PermManageSettings Permission = "settings:manage"
	// PermManageUsers covers the accounts of other users.
	PermManageUsers Permission = "users:manage"
)

// rolePermissions is the single source of the role policy. Operators gain
// their service and backup permissions with phases 5 and 6.
var rolePermissions = map[Role]map[Permission]bool{
	RoleViewer:   {},
	RoleOperator: {},
	RoleAdmin:    {PermManageSettings: true, PermManageUsers: true},
}

// Valid reports whether r is one of the defined roles.
func (r Role) Valid() bool {
	_, ok := rolePermissions[r]
	return ok
}

// Can reports whether r grants p; an unknown role grants nothing.
func (r Role) Can(p Permission) bool {
	return rolePermissions[r][p]
}
```

`backend/internal/domain/account.go` :

```go
package domain

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Bounds of the account fields (SPEC_ADMINISTRATION.md § 3.1).
const (
	MinPasswordBytes    = 12
	MaxPasswordBytes    = 1024
	MaxDisplayNameBytes = 128
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

var (
	ErrInvalidUsername    = errors.New("username must match ^[a-z0-9._-]{1,64}$")
	ErrInvalidDisplayName = errors.New("display name must be at most 128 bytes without control characters")
	ErrInvalidRole        = errors.New("role must be viewer, operator or admin")
	ErrInvalidPassword    = errors.New("password must be 12 to 1024 bytes")
	ErrEmptyChange        = errors.New("at least one field must change")
)

// NewUser is an account created by an administrator.
type NewUser struct {
	Username    string
	DisplayName string
	Role        Role
}

// Normalize validates the account and uses the username as display name
// when none is given.
func (n NewUser) Normalize() (NewUser, error) {
	if !usernamePattern.MatchString(n.Username) {
		return NewUser{}, ErrInvalidUsername
	}
	if !n.Role.Valid() {
		return NewUser{}, ErrInvalidRole
	}
	name, err := normalizeDisplayName(n.DisplayName)
	if err != nil {
		return NewUser{}, err
	}
	if name == "" {
		name = n.Username
	}
	n.DisplayName = name
	return n, nil
}

// UserChange lists the account fields to modify; nil leaves a field as is.
type UserChange struct {
	DisplayName *string
	Role        *Role
	Disabled    *bool
}

// Normalize validates the change; a display name cannot be blanked.
func (c UserChange) Normalize() (UserChange, error) {
	if c.DisplayName == nil && c.Role == nil && c.Disabled == nil {
		return UserChange{}, ErrEmptyChange
	}
	if c.Role != nil && !c.Role.Valid() {
		return UserChange{}, ErrInvalidRole
	}
	if c.DisplayName != nil {
		name, err := normalizeDisplayName(*c.DisplayName)
		if err != nil {
			return UserChange{}, err
		}
		if name == "" {
			return UserChange{}, ErrInvalidDisplayName
		}
		c.DisplayName = &name
	}
	return c, nil
}

// Apply returns u with the change applied.
func (c UserChange) Apply(u User) User {
	if c.DisplayName != nil {
		u.DisplayName = *c.DisplayName
	}
	if c.Role != nil {
		u.Role = *c.Role
	}
	if c.Disabled != nil {
		u.Disabled = *c.Disabled
	}
	return u
}

// IsActiveAdmin reports whether u can administer the Manager now.
func (u User) IsActiveAdmin() bool {
	return u.Role == RoleAdmin && !u.Disabled
}

// ValidateNewPassword checks the length of a password being set; logging
// in accepts shorter passwords such as the default admin/admin.
func ValidateNewPassword(password string) error {
	if len(password) < MinPasswordBytes || len(password) > MaxPasswordBytes {
		return ErrInvalidPassword
	}
	return nil
}

// IsValidationError reports whether err rejects user input.
func IsValidationError(err error) bool {
	for _, target := range []error{ErrInvalidUsername, ErrInvalidDisplayName, ErrInvalidRole, ErrInvalidPassword, ErrEmptyChange} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func normalizeDisplayName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if len(name) > MaxDisplayNameBytes || !utf8.ValidString(name) {
		return "", ErrInvalidDisplayName
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", ErrInvalidDisplayName
		}
	}
	return name, nil
}
```

Dans `backend/internal/domain/auth.go`, remplacer le bloc `var (...)` et le type `User` par :

```go
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionNotFound    = errors.New("session not found")
	ErrForbidden          = errors.New("the account role does not allow this action")
	ErrUserNotFound       = errors.New("user not found")
	ErrUsernameTaken      = errors.New("username already taken")
	ErrLastAdmin          = errors.New("the last active administrator cannot be removed, demoted or disabled")
)

// User is a Manager account. Accounts are never Linux accounts.
type User struct {
	ID                 uuid.UUID
	Username           string
	DisplayName        string
	Role               Role
	Disabled           bool
	MustChangePassword bool
	LastLoginAt        *time.Time
	LastLoginIP        string
	CreatedAt          time.Time
}
```

- [ ] **Étape 4 : lancer les tests et constater le succès**

Commande : `go -C backend test ./internal/domain/ -count=1 && go -C backend build ./...`
Attendu : `ok  homelab-manager/backend/internal/domain`, build sans erreur (les champs ajoutés sont nommés, les littéraux existants compilent).

- [ ] **Étape 5 : commit**

```bash
gofmt -l backend   # doit être vide
scripts/verify.sh
git add backend/internal/domain
git commit -m "feat(domain): add account roles, permissions and validation rules"
```

---

### Tâche 2 : persistance — migration 003, schéma Ent et dépôt des comptes

**Fichiers :**
- Créer : `backend/migrations/003_user_accounts.sql`, `backend/internal/adapters/sqlite/transaction.go`, `backend/internal/adapters/sqlite/account_repository.go`
- Modifier : `backend/ent/schema/user.go` (+ code régénéré sous `backend/ent/`), `backend/internal/adapters/sqlite/auth_repository.go`, `backend/internal/adapters/sqlite/settings_repository.go`, `docs/DATA_MODEL.md`
- Tests : `backend/internal/adapters/sqlite/account_repository_test.go`

**Interfaces :**
- Consomme : `domain.User`, `domain.NewUser`, `domain.UserChange`, `domain.AuditEntry`, `domain.Role*`, `domain.ErrUserNotFound`, `domain.ErrUsernameTaken` (tâche 1).
- Produit :
  - `sqlite.NewAccountRepository(client *ent.Client) *AccountRepository` avec `ListUsers(ctx, limit int) ([]domain.User, error)` (tri par identifiant), `GetUser(ctx, id uuid.UUID) (domain.User, error)`, `CountActiveAdmins(ctx) (int, error)`, `CreateUser(ctx, domain.NewUser, passwordHash string, domain.AuditEntry) (domain.User, error)`, `UpdateUser(ctx, id uuid.UUID, domain.UserChange, domain.AuditEntry) (domain.User, error)`, `DeleteUser(ctx, id uuid.UUID, domain.AuditEntry) error`. Le dépôt renseigne lui-même `AuditEntry.TargetID` avec l'UUID du compte.
  - `(*AuthRepository).RecordLogin(ctx, userID uuid.UUID, ip string, at time.Time) error`.
  - `UpdatePasswordHash` remet `must_change_password` à faux ; `EnsureAdmin` crée un compte `admin` avec nom affiché = identifiant.

- [ ] **Étape 1 : écrire les tests qui échouent**

`backend/internal/adapters/sqlite/account_repository_test.go` :

```go
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"homelab-manager/backend/ent/auditlog"
	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

func TestAccountRepositoryLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	auth := NewAuthRepository(database.Client())
	accounts := NewAccountRepository(database.Client())

	if _, err := auth.EnsureAdmin(ctx, "admin", "admin-hash"); err != nil {
		t.Fatal(err)
	}
	admin, _, err := auth.FindByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if admin.Role != domain.RoleAdmin || admin.DisplayName != "admin" || admin.MustChangePassword || admin.Disabled {
		t.Fatalf("bootstrap administrator = %+v", admin)
	}

	audit := domain.AuditEntry{Actor: "admin", TargetType: "user", Action: "create", RequestID: "req-1", Outcome: "success"}
	alice, err := accounts.CreateUser(ctx, domain.NewUser{Username: "alice", DisplayName: "Alice", Role: domain.RoleViewer}, "alice-hash", audit)
	if err != nil {
		t.Fatal(err)
	}
	if !alice.MustChangePassword || alice.Role != domain.RoleViewer || alice.DisplayName != "Alice" {
		t.Fatalf("created account = %+v", alice)
	}
	if _, err := accounts.CreateUser(ctx, domain.NewUser{Username: "alice", DisplayName: "Other", Role: domain.RoleViewer}, "hash", audit); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatalf("duplicate username error = %v, want ErrUsernameTaken", err)
	}
	if count, err := accounts.CountActiveAdmins(ctx); err != nil || count != 1 {
		t.Fatalf("CountActiveAdmins = (%d, %v), want 1", count, err)
	}

	if err := auth.UpdatePasswordHash(ctx, alice.ID, "alice-own-hash"); err != nil {
		t.Fatal(err)
	}
	if reloaded, err := accounts.GetUser(ctx, alice.ID); err != nil || reloaded.MustChangePassword {
		t.Fatalf("after a password change = (%+v, %v), want MustChangePassword false", reloaded, err)
	}

	loginAt := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	if err := auth.RecordLogin(ctx, alice.ID, "192.0.2.10", loginAt); err != nil {
		t.Fatal(err)
	}
	operator, disabled := domain.RoleOperator, true
	audit.Action = "update"
	updated, err := accounts.UpdateUser(ctx, alice.ID, domain.UserChange{Role: &operator, Disabled: &disabled}, audit)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Role != domain.RoleOperator || !updated.Disabled || updated.LastLoginIP != "192.0.2.10" || updated.LastLoginAt == nil || !updated.LastLoginAt.Equal(loginAt) {
		t.Fatalf("updated account = %+v", updated)
	}
	if _, err := accounts.UpdateUser(ctx, uuid.New(), domain.UserChange{Role: &operator}, audit); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("update of an unknown account error = %v, want ErrUserNotFound", err)
	}

	if _, err := auth.Create(ctx, alice.ID, "alice-token", "127.0.0.1", "test", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	audit.Action = "delete"
	if err := accounts.DeleteUser(ctx, alice.ID, audit); err != nil {
		t.Fatalf("delete an account with sessions: %v", err)
	}
	if _, err := accounts.GetUser(ctx, alice.ID); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("deleted account error = %v, want ErrUserNotFound", err)
	}
	if err := accounts.DeleteUser(ctx, alice.ID, audit); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("second delete error = %v, want ErrUserNotFound", err)
	}

	users, err := accounts.ListUsers(ctx, 10)
	if err != nil || len(users) != 1 || users[0].Username != "admin" {
		t.Fatalf("ListUsers = (%+v, %v), want only admin", users, err)
	}
	// create, update and delete committed; the refused calls rolled back.
	audits, err := database.Client().AuditLog.Query().Where(auditlog.TargetType("user"), auditlog.TargetID(alice.ID.String())).Count(ctx)
	if err != nil || audits != 3 {
		t.Fatalf("audit rows for alice = (%d, %v), want 3", audits, err)
	}
}

func TestMigration003PromotesExistingAccountsToAdministrators(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	if err := prepareFile(path); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_initial.sql", "002_app_settings.sql"} {
		if err := applyMigration(ctx, db, name); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (?, 'admin', 'hash', ?, ?)`, uuid.NewString(), now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	user, _, err := NewAuthRepository(database.Client()).FindByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != domain.RoleAdmin || user.DisplayName != "admin" || user.Disabled || user.MustChangePassword {
		t.Fatalf("upgraded account = %+v, want an active admin", user)
	}
}
```

- [ ] **Étape 2 : lancer les tests et constater l'échec**

Commande : `go -C backend test ./internal/adapters/sqlite/ -run 'TestAccountRepository|TestMigration003' -count=1`
Attendu : échec de compilation, `undefined: NewAccountRepository`, `auth.RecordLogin undefined`.

- [ ] **Étape 3 : migration**

`backend/migrations/003_user_accounts.sql` :

```sql
-- Several Manager accounts with roles (SPEC_ADMINISTRATION.md § 3.1-3.2).
-- These are Homelab Manager accounts only, never Linux accounts.
-- Before this migration the only account was the administrator, so every
-- existing row becomes an administrator.
ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'viewer'
    CHECK (role IN ('viewer', 'operator', 'admin'));
ALTER TABLE users ADD COLUMN disabled_at DATETIME;
ALTER TABLE users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN last_login_at DATETIME;
ALTER TABLE users ADD COLUMN last_login_ip TEXT;
UPDATE users SET role = 'admin', display_name = username;
```

- [ ] **Étape 4 : schéma Ent et régénération**

Dans `backend/ent/schema/user.go`, remplacer `Fields()` par :

```go
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("username").Unique().NotEmpty(),
		field.String("password_hash").Sensitive(),
		field.String("display_name").Default(""),
		field.Enum("role").Values("viewer", "operator", "admin").Default("viewer"),
		field.Time("disabled_at").Optional().Nillable(),
		field.Bool("must_change_password").Default(false),
		field.Time("last_login_at").Optional().Nillable(),
		field.String("last_login_ip").Optional(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}
```

Commande : `cd backend && GOTOOLCHAIN=go1.26.10 go run entgo.io/ent/cmd/ent@v0.14.6 generate ./ent/schema && cd ..`
Attendu : fichiers `backend/ent/user*.go`, `backend/ent/user/*.go`, `backend/ent/mutation.go`, `backend/ent/migrate/schema.go`, `backend/ent/runtime.go` modifiés ; `go -C backend build ./...` réussit. Le paquet `ent/user` expose `user.Role`, `user.RoleAdmin`, `user.RoleEQ`, `user.DisabledAtIsNil`.

- [ ] **Étape 5 : helpers de transaction et d'audit**

`backend/internal/adapters/sqlite/transaction.go` :

```go
package sqlite

import (
	"context"
	"fmt"

	"homelab-manager/backend/ent"
	"homelab-manager/backend/internal/domain"
)

// withTx runs fn in a transaction, committed only when fn succeeds. fn must
// only use tx: the database has a single connection.
func withTx(ctx context.Context, client *ent.Client, fn func(tx *ent.Tx) error) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// writeAudit records an administrative change in the transaction that
// makes it, so that the change and its trace commit together.
func writeAudit(ctx context.Context, tx *ent.Tx, audit domain.AuditEntry) error {
	create := tx.AuditLog.Create().
		SetActor(audit.Actor).SetTargetType(audit.TargetType).SetTargetID(audit.TargetID).
		SetAction(audit.Action).SetOutcome(audit.Outcome)
	if audit.RequestID != "" {
		create.SetRequestID(audit.RequestID)
	}
	if _, err := create.Save(ctx); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}
```

Dans `backend/internal/adapters/sqlite/settings_repository.go`, à la fin de `setRetention`, remplacer :

```go
	create := tx.AuditLog.Create().
		SetActor(audit.Actor).SetTargetType(audit.TargetType).SetTargetID(audit.TargetID).
		SetAction(audit.Action).SetOutcome(audit.Outcome)
	if audit.RequestID != "" {
		create.SetRequestID(audit.RequestID)
	}
	if _, err := create.Save(ctx); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}
```

par :

```go
	return writeAudit(ctx, tx, audit)
}
```

- [ ] **Étape 6 : dépôt des comptes**

`backend/internal/adapters/sqlite/account_repository.go` :

```go
package sqlite

import (
	"context"
	"fmt"
	"time"

	"homelab-manager/backend/ent"
	"homelab-manager/backend/ent/session"
	"homelab-manager/backend/ent/user"
	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

// AccountRepository stores the Manager accounts managed by administrators.
// Every change writes its audit entry in the same transaction; the
// repository fills AuditEntry.TargetID with the account UUID.
type AccountRepository struct {
	client *ent.Client
}

func NewAccountRepository(client *ent.Client) *AccountRepository {
	return &AccountRepository{client: client}
}

// ListUsers returns at most limit accounts ordered by username.
func (r *AccountRepository) ListUsers(ctx context.Context, limit int) ([]domain.User, error) {
	records, err := r.client.User.Query().Order(ent.Asc(user.FieldUsername)).Limit(limit).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	users := make([]domain.User, 0, len(records))
	for _, record := range records {
		users = append(users, toDomainUser(record))
	}
	return users, nil
}

func (r *AccountRepository) GetUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	record, err := r.client.User.Get(ctx, id)
	if ent.IsNotFound(err) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("get user: %w", err)
	}
	return toDomainUser(record), nil
}

func (r *AccountRepository) CountActiveAdmins(ctx context.Context) (int, error) {
	count, err := r.client.User.Query().Where(user.RoleEQ(user.RoleAdmin), user.DisabledAtIsNil()).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count active administrators: %w", err)
	}
	return count, nil
}

// CreateUser stores an account that must change its password at first login.
func (r *AccountRepository) CreateUser(ctx context.Context, input domain.NewUser, passwordHash string, audit domain.AuditEntry) (domain.User, error) {
	var created *ent.User
	err := withTx(ctx, r.client, func(tx *ent.Tx) error {
		record, err := tx.User.Create().
			SetUsername(input.Username).
			SetDisplayName(input.DisplayName).
			SetRole(user.Role(input.Role)).
			SetPasswordHash(passwordHash).
			SetMustChangePassword(true).
			Save(ctx)
		if ent.IsConstraintError(err) {
			return domain.ErrUsernameTaken
		}
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		created = record
		audit.TargetID = record.ID.String()
		return writeAudit(ctx, tx, audit)
	})
	if err != nil {
		return domain.User{}, err
	}
	return toDomainUser(created), nil
}

func (r *AccountRepository) UpdateUser(ctx context.Context, id uuid.UUID, change domain.UserChange, audit domain.AuditEntry) (domain.User, error) {
	var updated *ent.User
	err := withTx(ctx, r.client, func(tx *ent.Tx) error {
		update := tx.User.UpdateOneID(id)
		if change.DisplayName != nil {
			update.SetDisplayName(*change.DisplayName)
		}
		if change.Role != nil {
			update.SetRole(user.Role(*change.Role))
		}
		if change.Disabled != nil {
			if *change.Disabled {
				update.SetDisabledAt(time.Now().UTC())
			} else {
				update.ClearDisabledAt()
			}
		}
		record, err := update.Save(ctx)
		if ent.IsNotFound(err) {
			return domain.ErrUserNotFound
		}
		if err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		updated = record
		audit.TargetID = id.String()
		return writeAudit(ctx, tx, audit)
	})
	if err != nil {
		return domain.User{}, err
	}
	return toDomainUser(updated), nil
}

// DeleteUser removes the account and its sessions; audit rows keep the
// actor username as text and therefore survive the account.
func (r *AccountRepository) DeleteUser(ctx context.Context, id uuid.UUID, audit domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		if _, err := tx.Session.Delete().Where(session.UserID(id)).Exec(ctx); err != nil {
			return fmt.Errorf("delete sessions: %w", err)
		}
		err := tx.User.DeleteOneID(id).Exec(ctx)
		if ent.IsNotFound(err) {
			return domain.ErrUserNotFound
		}
		if err != nil {
			return fmt.Errorf("delete user: %w", err)
		}
		audit.TargetID = id.String()
		return writeAudit(ctx, tx, audit)
	})
}

func toDomainUser(record *ent.User) domain.User {
	return domain.User{
		ID:                 record.ID,
		Username:           record.Username,
		DisplayName:        record.DisplayName,
		Role:               domain.Role(record.Role),
		Disabled:           record.DisabledAt != nil,
		MustChangePassword: record.MustChangePassword,
		LastLoginAt:        record.LastLoginAt,
		LastLoginIP:        record.LastLoginIP,
		CreatedAt:          record.CreatedAt,
	}
}
```

- [ ] **Étape 7 : dépôt d'authentification**

Dans `backend/internal/adapters/sqlite/auth_repository.go` :

1. Dans `FindByUsername`, remplacer `return domain.User{ID: record.ID, Username: record.Username}, record.PasswordHash, nil` par `return toDomainUser(record), record.PasswordHash, nil`.
2. Remplacer `FindByID` par :

```go
func (r *AuthRepository) FindByID(ctx context.Context, userID uuid.UUID) (domain.User, error) {
	record, err := r.client.User.Get(ctx, userID)
	if ent.IsNotFound(err) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	return toDomainUser(record), nil
}
```

3. Remplacer `UpdatePasswordHash` par :

```go
// UpdatePasswordHash stores a password chosen by the user, which also ends
// the obligation to change the initial password.
func (r *AuthRepository) UpdatePasswordHash(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	return r.client.User.UpdateOneID(userID).SetPasswordHash(passwordHash).SetMustChangePassword(false).Exec(ctx)
}

// RecordLogin stores the time and source address of a successful login.
func (r *AuthRepository) RecordLogin(ctx context.Context, userID uuid.UUID, ip string, at time.Time) error {
	return r.client.User.UpdateOneID(userID).SetLastLoginAt(at.UTC()).SetLastLoginIP(ip).Exec(ctx)
}
```

4. Dans `EnsureAdmin`, remplacer `r.client.User.Create().SetUsername(username).SetPasswordHash(passwordHash).Save(ctx)` par :

```go
r.client.User.Create().SetUsername(username).SetDisplayName(username).SetRole(user.RoleAdmin).SetPasswordHash(passwordHash).Save(ctx)
```

- [ ] **Étape 8 : lancer les tests et constater le succès**

Commande : `go -C backend test ./internal/adapters/sqlite/ -count=1 -v -run 'TestAccountRepository|TestMigration003|TestSQLiteIntegration|TestSettingsRepository'`
Attendu : `PASS` pour les quatre tests (les tests existants vérifient que l'extraction de `writeAudit` n'a rien changé).

- [ ] **Étape 9 : documentation du modèle**

Dans `docs/DATA_MODEL.md`, remplacer la section `### User` par :

```markdown
### User
- UUID, identifiant de connexion unique (motif `^[a-z0-9._-]{1,64}$` pour un compte créé depuis l'interface), nom affiché, hash du mot de passe (Argon2id), timestamps.
- `role` : `viewer`, `operator` ou `admin` (contrainte `CHECK`). `disabled_at` : NULL si le compte est actif. `must_change_password` : vrai pour un compte créé par un administrateur, jusqu'à ce que son titulaire choisisse son mot de passe. `last_login_at`, `last_login_ip` : dernière connexion réussie.
- Comptes de Homelab Manager uniquement, jamais des comptes Linux. Le compte initial est créé administrateur ; la migration `003_user_accounts.sql` a promu administrateurs les comptes existants.
- Suppression : les sessions du compte sont supprimées dans la même transaction ; les lignes d'audit, qui désignent l'acteur par son identifiant texte, sont conservées.
```

Dans la section `### AuditLog`, remplacer la phrase commençant par « Écrit dans la même transaction » par :

```markdown
- Écrit dans la même transaction que la modification qu'il trace : modification de la rétention (`settings` / `metrics_retention_days` / `update`) et des comptes (`user` / UUID du compte / `create`, `update`, `delete`). Les autres modules d'administration v3 y journaliseront chaque action (SPEC_ADMINISTRATION.md § 2.2).
```

Dans « Intégrité et migrations », remplacer « `001_initial.sql` crée le schéma SQLite initial, `002_app_settings.sql` la table `app_settings` ; » par « `001_initial.sql` crée le schéma SQLite initial, `002_app_settings.sql` la table `app_settings`, `003_user_accounts.sql` les colonnes de rôle et d'état des comptes ; ».

- [ ] **Étape 10 : commit**

```bash
scripts/verify.sh
git add backend/migrations/003_user_accounts.sql backend/ent backend/internal/adapters/sqlite docs/DATA_MODEL.md
git commit -m "feat(storage): store account roles, state and last login"
```

---

### Tâche 3 : authentification — comptes désactivés et dernière connexion

**Fichiers :**
- Modifier : `backend/internal/ports/auth.go`, `backend/internal/usecase/auth.go`, `backend/internal/usecase/auth_test.go`

**Interfaces :**
- Consomme : `(*sqlite.AuthRepository).RecordLogin` (tâche 2), `domain.User.Disabled` (tâche 1).
- Produit : `ports.UserRepository.RecordLogin(ctx context.Context, userID uuid.UUID, ip string, at time.Time) error`. `AuthUsecase.Login` refuse un compte désactivé (`ErrInvalidCredentials`, après vérification Argon2id pour un temps constant) et enregistre la connexion ; `AuthUsecase.Session` refuse la session d'un compte désactivé (`ErrSessionNotFound`) et renvoie l'utilisateur complet (rôle, `MustChangePassword`).

- [ ] **Étape 1 : adapter les faux objets et écrire les tests qui échouent**

Dans `backend/internal/usecase/auth_test.go` :

1. Ajouter `"errors"` aux imports.
2. Remplacer `mockUserRepository` et `mockSessionRepository` (types et méthodes `FindActiveByTokenHash`) par :

```go
type mockUserRepository struct {
	user    domain.User
	hash    string
	missing bool
	loginIP string
	loginAt time.Time
}

func (m *mockUserRepository) FindByUsername(context.Context, string) (domain.User, string, error) {
	if m.missing {
		return domain.User{}, "", domain.ErrInvalidCredentials
	}
	return m.user, m.hash, nil
}
func (m *mockUserRepository) FindByID(context.Context, uuid.UUID) (domain.User, error) {
	return m.user, nil
}
func (m *mockUserRepository) UpdatePasswordHash(context.Context, uuid.UUID, string) error {
	return nil
}
func (m *mockUserRepository) EnsureAdmin(context.Context, string, string) (bool, error) {
	return false, nil
}
func (m *mockUserRepository) RecordLogin(_ context.Context, _ uuid.UUID, ip string, at time.Time) error {
	m.loginIP, m.loginAt = ip, at
	return nil
}

// mockSessionRepository finds active when it is set.
type mockSessionRepository struct {
	active *domain.Session
}

func (m *mockSessionRepository) Create(_ context.Context, userID uuid.UUID, _ string, _ string, _ string, expiresAt time.Time) (domain.Session, error) {
	return domain.Session{ID: uuid.New(), UserID: userID, ExpiresAt: expiresAt}, nil
}
func (m *mockSessionRepository) FindActiveByTokenHash(context.Context, string) (domain.Session, error) {
	if m.active != nil {
		return *m.active, nil
	}
	return domain.Session{}, domain.ErrSessionNotFound
}
```

(Les méthodes `RevokeByTokenHash` et `RevokeAllForUser` restent inchangées.)

3. Ajouter à la fin du fichier :

```go
func TestLoginRejectsADisabledAccount(t *testing.T) {
	users := &mockUserRepository{
		user: domain.User{ID: uuid.New(), Username: "bob", Role: domain.RoleViewer, Disabled: true},
		hash: "stored-hash",
	}
	hasher := &mockHasher{valid: true}
	auth := NewAuthUsecase(users, &mockSessionRepository{}, hasher)

	_, _, _, err := auth.Login(context.Background(), "bob", "a-valid-password", "192.0.2.1", "test")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("Login error = %v, want ErrInvalidCredentials", err)
	}
	if len(hasher.verified) != 1 {
		t.Fatal("the password must still be verified so that the response time does not reveal the state")
	}
	if users.loginIP != "" {
		t.Fatal("a refused login must not be recorded")
	}
}

func TestLoginRecordsTheLastLogin(t *testing.T) {
	users := &mockUserRepository{
		user: domain.User{ID: uuid.New(), Username: "alice", Role: domain.RoleViewer},
		hash: "stored-hash",
	}
	auth := NewAuthUsecase(users, &mockSessionRepository{}, &mockHasher{valid: true})
	fixed := time.Date(2026, 11, 23, 9, 30, 0, 0, time.UTC)
	auth.now = func() time.Time { return fixed }

	if _, _, _, err := auth.Login(context.Background(), "alice", "a-valid-password", "192.0.2.7", "test"); err != nil {
		t.Fatal(err)
	}
	if users.loginIP != "192.0.2.7" || !users.loginAt.Equal(fixed) {
		t.Fatalf("recorded login = (%q, %s), want (192.0.2.7, %s)", users.loginIP, users.loginAt, fixed)
	}
}

func TestSessionRejectsADisabledAccount(t *testing.T) {
	userID := uuid.New()
	sessions := &mockSessionRepository{active: &domain.Session{ID: uuid.New(), UserID: userID, ExpiresAt: time.Now().Add(time.Hour)}}
	users := &mockUserRepository{user: domain.User{ID: userID, Username: "bob", Role: domain.RoleAdmin, Disabled: true}}

	_, _, err := NewAuthUsecase(users, sessions, &mockHasher{}).Session(context.Background(), "token")
	if !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("Session error = %v, want ErrSessionNotFound", err)
	}
}
```

- [ ] **Étape 2 : lancer les tests et constater l'échec**

Commande : `go -C backend test ./internal/usecase/ -run 'TestLogin|TestSession' -count=1`
Attendu : `TestLoginRejectsADisabledAccount`, `TestLoginRecordsTheLastLogin` et `TestSessionRejectsADisabledAccount` échouent (le compte désactivé est accepté, aucune connexion enregistrée).

- [ ] **Étape 3 : implémentation**

Dans `backend/internal/ports/auth.go`, ajouter à `UserRepository` :

```go
	// RecordLogin stores the time and source address of a successful login.
	RecordLogin(ctx context.Context, userID uuid.UUID, ip string, at time.Time) error
```

Dans `backend/internal/usecase/auth.go`, remplacer le début de `Login` jusqu'à la création de session incluse par :

```go
func (u *AuthUsecase) Login(ctx context.Context, username, password, ip, userAgent string) (string, domain.User, time.Time, error) {
	user, storedHash, err := u.users.FindByUsername(ctx, username)
	if errors.Is(err, domain.ErrInvalidCredentials) {
		u.verifyDummy(password)
		return "", domain.User{}, time.Time{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	// The password is verified before the state is checked, so that a
	// disabled account costs the same Argon2id work as any other refusal.
	valid, err := u.hasher.Verify(password, storedHash)
	if err != nil || !valid || user.Disabled {
		return "", domain.User{}, time.Time{}, domain.ErrInvalidCredentials
	}

	token, err := randomToken()
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	now := u.now().UTC()
	if err := u.users.RecordLogin(ctx, user.ID, ip, now); err != nil {
		return "", domain.User{}, time.Time{}, fmt.Errorf("record login: %w", err)
	}
	expiresAt := now.Add(12 * time.Hour)
	tokenHash := sha256Hex(token)
	session, err := u.sessions.Create(ctx, user.ID, tokenHash, ip, userAgent, expiresAt)
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	return token, user, session.ExpiresAt, nil
}
```

Ajouter `"fmt"` aux imports. Dans `Session`, remplacer la fin :

```go
	user, err := u.users.FindByID(ctx, session.UserID)
	if err != nil {
		return domain.User{}, domain.Session{}, err
	}
	return user, session, nil
```

par :

```go
	user, err := u.users.FindByID(ctx, session.UserID)
	if errors.Is(err, domain.ErrUserNotFound) {
		return domain.User{}, domain.Session{}, domain.ErrSessionNotFound
	}
	if err != nil {
		return domain.User{}, domain.Session{}, err
	}
	// Disabling an account revokes its sessions; this check also covers a
	// revocation that failed after the account was disabled.
	if user.Disabled {
		return domain.User{}, domain.Session{}, domain.ErrSessionNotFound
	}
	return user, session, nil
```

- [ ] **Étape 4 : lancer les tests et constater le succès**

Commande : `go -C backend test ./internal/usecase/ -count=1 && go -C backend build ./...`
Attendu : `ok  homelab-manager/backend/internal/usecase` ; build réussi (`*sqlite.AuthRepository` implémente désormais `RecordLogin`).

- [ ] **Étape 5 : commit**

```bash
scripts/verify.sh
git add backend/internal/ports/auth.go backend/internal/usecase/auth.go backend/internal/usecase/auth_test.go
git commit -m "feat(auth): refuse disabled accounts and record the last login"
```

---

### Tâche 4 : usecase de gestion des comptes

**Fichiers :**
- Créer : `backend/internal/ports/accounts.go`, `backend/internal/usecase/accounts.go`
- Tests : `backend/internal/usecase/accounts_test.go`

**Interfaces :**
- Consomme : `ports.SessionRepository.RevokeAllForUser`, `ports.PasswordHasher.Hash`, domaine (tâche 1), méthodes du dépôt de la tâche 2.
- Produit :
  - `ports.AccountRepository` (mêmes méthodes que `sqlite.AccountRepository`).
  - `usecase.NewAccountUsecase(accounts ports.AccountRepository, sessions ports.SessionRepository, hasher ports.PasswordHasher) *AccountUsecase`.
  - `List(ctx) ([]domain.User, error)` ; `Create(ctx, actor domain.User, input domain.NewUser, password, requestID string) (domain.User, error)` ; `Update(ctx, actor domain.User, id uuid.UUID, change domain.UserChange, requestID string) (domain.User, error)` ; `Delete(ctx, actor domain.User, id uuid.UUID, requestID string) error`. L'appelant doit détenir `domain.PermManageUsers` (appliqué par le middleware HTTP, tâche 5).

- [ ] **Étape 1 : écrire les tests qui échouent**

`backend/internal/usecase/accounts_test.go` :

```go
package usecase

import (
	"context"
	"errors"
	"sort"
	"testing"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

// fakeAccounts is an in-memory ports.AccountRepository.
type fakeAccounts struct {
	users  map[uuid.UUID]domain.User
	hashes map[uuid.UUID]string
	audits []domain.AuditEntry
}

func newFakeAccounts(users ...domain.User) *fakeAccounts {
	fake := &fakeAccounts{users: map[uuid.UUID]domain.User{}, hashes: map[uuid.UUID]string{}}
	for _, user := range users {
		fake.users[user.ID] = user
	}
	return fake
}

func (f *fakeAccounts) ListUsers(_ context.Context, limit int) ([]domain.User, error) {
	users := make([]domain.User, 0, len(f.users))
	for _, user := range f.users {
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
	if len(users) > limit {
		users = users[:limit]
	}
	return users, nil
}

func (f *fakeAccounts) GetUser(_ context.Context, id uuid.UUID) (domain.User, error) {
	user, ok := f.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return user, nil
}

func (f *fakeAccounts) CountActiveAdmins(context.Context) (int, error) {
	count := 0
	for _, user := range f.users {
		if user.IsActiveAdmin() {
			count++
		}
	}
	return count, nil
}

func (f *fakeAccounts) CreateUser(_ context.Context, input domain.NewUser, passwordHash string, audit domain.AuditEntry) (domain.User, error) {
	for _, user := range f.users {
		if user.Username == input.Username {
			return domain.User{}, domain.ErrUsernameTaken
		}
	}
	user := domain.User{ID: uuid.New(), Username: input.Username, DisplayName: input.DisplayName, Role: input.Role, MustChangePassword: true}
	f.users[user.ID] = user
	f.hashes[user.ID] = passwordHash
	audit.TargetID = user.ID.String()
	f.audits = append(f.audits, audit)
	return user, nil
}

func (f *fakeAccounts) UpdateUser(_ context.Context, id uuid.UUID, change domain.UserChange, audit domain.AuditEntry) (domain.User, error) {
	user, ok := f.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	user = change.Apply(user)
	f.users[id] = user
	audit.TargetID = id.String()
	f.audits = append(f.audits, audit)
	return user, nil
}

func (f *fakeAccounts) DeleteUser(_ context.Context, id uuid.UUID, audit domain.AuditEntry) error {
	if _, ok := f.users[id]; !ok {
		return domain.ErrUserNotFound
	}
	delete(f.users, id)
	audit.TargetID = id.String()
	f.audits = append(f.audits, audit)
	return nil
}

// revokingSessions records the accounts whose sessions were revoked.
type revokingSessions struct {
	mockSessionRepository
	revoked []uuid.UUID
}

func (r *revokingSessions) RevokeAllForUser(_ context.Context, userID uuid.UUID) error {
	r.revoked = append(r.revoked, userID)
	return nil
}

var testAdmin = domain.User{ID: uuid.New(), Username: "admin", DisplayName: "admin", Role: domain.RoleAdmin}

func TestCreateAccountNormalisesHashesAndAudits(t *testing.T) {
	accounts := newFakeAccounts(testAdmin)
	usecase := NewAccountUsecase(accounts, &revokingSessions{}, &mockHasher{})

	created, err := usecase.Create(context.Background(), testAdmin, domain.NewUser{Username: "alice", DisplayName: " ", Role: domain.RoleViewer}, "initial-password-1", "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if created.DisplayName != "alice" || !created.MustChangePassword {
		t.Fatalf("created = %+v", created)
	}
	if accounts.hashes[created.ID] != "hash:initial-password-1" {
		t.Fatal("the stored password must be the hasher output")
	}
	want := domain.AuditEntry{Actor: "admin", TargetType: "user", TargetID: created.ID.String(), Action: "create", RequestID: "req-1", Outcome: "success"}
	if len(accounts.audits) != 1 || accounts.audits[0] != want {
		t.Fatalf("audits = %+v, want [%+v]", accounts.audits, want)
	}
}

func TestCreateAccountRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name     string
		input    domain.NewUser
		password string
	}{
		{"username", domain.NewUser{Username: "Alice", Role: domain.RoleViewer}, "initial-password-1"},
		{"role", domain.NewUser{Username: "alice", Role: "root"}, "initial-password-1"},
		{"password", domain.NewUser{Username: "alice", Role: domain.RoleViewer}, "short"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			accounts := newFakeAccounts(testAdmin)
			_, err := NewAccountUsecase(accounts, &revokingSessions{}, &mockHasher{}).Create(context.Background(), testAdmin, c.input, c.password, "")
			if !domain.IsValidationError(err) {
				t.Fatalf("Create error = %v, want a validation error", err)
			}
			if len(accounts.users) != 1 || len(accounts.audits) != 0 {
				t.Fatal("an invalid account must not be stored")
			}
		})
	}
}

func TestTheLastActiveAdminIsProtected(t *testing.T) {
	viewer := domain.User{ID: uuid.New(), Username: "viewer", Role: domain.RoleViewer}
	accounts := newFakeAccounts(testAdmin, viewer)
	usecase := NewAccountUsecase(accounts, &revokingSessions{}, &mockHasher{})
	ctx := context.Background()

	demote, disable := domain.RoleOperator, true
	if _, err := usecase.Update(ctx, testAdmin, testAdmin.ID, domain.UserChange{Role: &demote}, ""); !errors.Is(err, domain.ErrLastAdmin) {
		t.Fatalf("demote error = %v, want ErrLastAdmin", err)
	}
	if _, err := usecase.Update(ctx, testAdmin, testAdmin.ID, domain.UserChange{Disabled: &disable}, ""); !errors.Is(err, domain.ErrLastAdmin) {
		t.Fatalf("disable error = %v, want ErrLastAdmin", err)
	}
	if err := usecase.Delete(ctx, testAdmin, testAdmin.ID, ""); !errors.Is(err, domain.ErrLastAdmin) {
		t.Fatalf("delete error = %v, want ErrLastAdmin", err)
	}
	name := "Root"
	if _, err := usecase.Update(ctx, testAdmin, testAdmin.ID, domain.UserChange{DisplayName: &name}, ""); err != nil {
		t.Fatalf("renaming the last admin must be allowed: %v", err)
	}
	if len(accounts.audits) != 1 {
		t.Fatalf("only the rename is audited, got %d entries", len(accounts.audits))
	}
}

func TestAnAdminCanBeDemotedWhenAnotherRemains(t *testing.T) {
	second := domain.User{ID: uuid.New(), Username: "second", Role: domain.RoleAdmin}
	accounts := newFakeAccounts(testAdmin, second)
	viewer := domain.RoleViewer

	updated, err := NewAccountUsecase(accounts, &revokingSessions{}, &mockHasher{}).Update(context.Background(), testAdmin, second.ID, domain.UserChange{Role: &viewer}, "")
	if err != nil || updated.Role != domain.RoleViewer {
		t.Fatalf("Update = (%+v, %v), want a viewer", updated, err)
	}
}

func TestDisablingAnAccountRevokesItsSessions(t *testing.T) {
	viewer := domain.User{ID: uuid.New(), Username: "viewer", Role: domain.RoleViewer}
	accounts := newFakeAccounts(testAdmin, viewer)
	sessions := &revokingSessions{}
	usecase := NewAccountUsecase(accounts, sessions, &mockHasher{})
	ctx := context.Background()

	disable, enable := true, false
	if _, err := usecase.Update(ctx, testAdmin, viewer.ID, domain.UserChange{Disabled: &disable}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := usecase.Update(ctx, testAdmin, viewer.ID, domain.UserChange{Disabled: &enable}, ""); err != nil {
		t.Fatal(err)
	}
	if len(sessions.revoked) != 1 || sessions.revoked[0] != viewer.ID {
		t.Fatalf("revoked = %v, want only the disabling to revoke", sessions.revoked)
	}
}

func TestUpdateAndDeleteReportUnknownAccounts(t *testing.T) {
	usecase := NewAccountUsecase(newFakeAccounts(testAdmin), &revokingSessions{}, &mockHasher{})
	viewer := domain.RoleViewer
	if _, err := usecase.Update(context.Background(), testAdmin, uuid.New(), domain.UserChange{Role: &viewer}, ""); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("Update error = %v, want ErrUserNotFound", err)
	}
	if err := usecase.Delete(context.Background(), testAdmin, uuid.New(), ""); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("Delete error = %v, want ErrUserNotFound", err)
	}
}
```

- [ ] **Étape 2 : lancer les tests et constater l'échec**

Commande : `go -C backend test ./internal/usecase/ -run 'Account|LastActiveAdmin|AnAdmin|Disabling|UpdateAndDelete' -count=1`
Attendu : échec de compilation, `undefined: NewAccountUsecase`.

- [ ] **Étape 3 : implémentation**

`backend/internal/ports/accounts.go` :

```go
package ports

import (
	"context"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

// AccountRepository stores the Manager accounts. Each change writes its
// audit entry atomically; the repository sets AuditEntry.TargetID.
type AccountRepository interface {
	// ListUsers returns at most limit accounts ordered by username.
	ListUsers(ctx context.Context, limit int) ([]domain.User, error)
	// GetUser returns domain.ErrUserNotFound for an unknown account.
	GetUser(ctx context.Context, id uuid.UUID) (domain.User, error)
	CountActiveAdmins(ctx context.Context) (int, error)
	// CreateUser returns domain.ErrUsernameTaken for a duplicate username;
	// the account must change its password at first login.
	CreateUser(ctx context.Context, user domain.NewUser, passwordHash string, audit domain.AuditEntry) (domain.User, error)
	UpdateUser(ctx context.Context, id uuid.UUID, change domain.UserChange, audit domain.AuditEntry) (domain.User, error)
	// DeleteUser also deletes the sessions of the account.
	DeleteUser(ctx context.Context, id uuid.UUID, audit domain.AuditEntry) error
}
```

`backend/internal/usecase/accounts.go` :

```go
package usecase

import (
	"context"
	"fmt"
	"sync"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
)

// maxAccounts bounds the account list; a homelab has a handful of accounts.
const maxAccounts = 500

const auditTargetUser = "user"

// AccountUsecase manages the Manager accounts (SPEC_ADMINISTRATION.md § 3.1).
// Callers must hold domain.PermManageUsers; the HTTP layer enforces it.
type AccountUsecase struct {
	accounts ports.AccountRepository
	sessions ports.SessionRepository
	hasher   ports.PasswordHasher

	// mu serialises account changes: the last-administrator check and the
	// write that follows must not interleave with another change. The
	// Manager is the only writer of the accounts.
	mu sync.Mutex
}

func NewAccountUsecase(accounts ports.AccountRepository, sessions ports.SessionRepository, hasher ports.PasswordHasher) *AccountUsecase {
	return &AccountUsecase{accounts: accounts, sessions: sessions, hasher: hasher}
}

func (u *AccountUsecase) List(ctx context.Context) ([]domain.User, error) {
	return u.accounts.ListUsers(ctx, maxAccounts)
}

// Create stores an account whose initial password must be changed at
// first login.
func (u *AccountUsecase) Create(ctx context.Context, actor domain.User, input domain.NewUser, password, requestID string) (domain.User, error) {
	normalized, err := input.Normalize()
	if err != nil {
		return domain.User{}, err
	}
	if err := domain.ValidateNewPassword(password); err != nil {
		return domain.User{}, err
	}
	// Hashing is slow on purpose: do it before taking the lock.
	hash, err := u.hasher.Hash(password)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.accounts.CreateUser(ctx, normalized, hash, auditEntry(actor, "create", requestID))
}

// Update changes an account. Disabling it revokes its sessions; the last
// active administrator can be neither demoted nor disabled.
func (u *AccountUsecase) Update(ctx context.Context, actor domain.User, id uuid.UUID, change domain.UserChange, requestID string) (domain.User, error) {
	change, err := change.Normalize()
	if err != nil {
		return domain.User{}, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	target, err := u.accounts.GetUser(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	if target.IsActiveAdmin() && !change.Apply(target).IsActiveAdmin() {
		if err := u.requireAnotherAdmin(ctx); err != nil {
			return domain.User{}, err
		}
	}
	updated, err := u.accounts.UpdateUser(ctx, id, change, auditEntry(actor, "update", requestID))
	if err != nil {
		return domain.User{}, err
	}
	if updated.Disabled && !target.Disabled {
		if err := u.sessions.RevokeAllForUser(ctx, id); err != nil {
			return domain.User{}, fmt.Errorf("revoke sessions: %w", err)
		}
	}
	return updated, nil
}

// Delete removes an account and its sessions, keeping its audit entries.
func (u *AccountUsecase) Delete(ctx context.Context, actor domain.User, id uuid.UUID, requestID string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	target, err := u.accounts.GetUser(ctx, id)
	if err != nil {
		return err
	}
	if target.IsActiveAdmin() {
		if err := u.requireAnotherAdmin(ctx); err != nil {
			return err
		}
	}
	return u.accounts.DeleteUser(ctx, id, auditEntry(actor, "delete", requestID))
}

func (u *AccountUsecase) requireAnotherAdmin(ctx context.Context) error {
	count, err := u.accounts.CountActiveAdmins(ctx)
	if err != nil {
		return err
	}
	if count <= 1 {
		return domain.ErrLastAdmin
	}
	return nil
}

func auditEntry(actor domain.User, action, requestID string) domain.AuditEntry {
	return domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetUser, Action: action,
		RequestID: requestID, Outcome: "success",
	}
}
```

Vérifier que `*sqlite.AccountRepository` satisfait le port en ajoutant en tête de `backend/internal/adapters/sqlite/account_repository.go`, après les imports :

```go
var _ ports.AccountRepository = (*AccountRepository)(nil)
```

avec l'import `"homelab-manager/backend/internal/ports"` (l'adaptateur dépend déjà des ports par le domaine ; `ports` n'importe pas `sqlite`, pas de cycle).

- [ ] **Étape 4 : lancer les tests et constater le succès**

Commande : `go -C backend test ./internal/usecase/ ./internal/adapters/sqlite/ -count=1 && go -C backend vet ./...`
Attendu : `ok` pour les deux paquets, `go vet` silencieux.

- [ ] **Étape 5 : commit**

```bash
scripts/verify.sh
git add backend/internal/ports/accounts.go backend/internal/usecase/accounts.go backend/internal/usecase/accounts_test.go backend/internal/adapters/sqlite/account_repository.go
git commit -m "feat(accounts): manage accounts and protect the last administrator"
```

---

### Tâche 5 : HTTP — autorisation, routes `/api/v1/users` et contrat

**Fichiers :**
- Créer : `backend/internal/delivery/http/access.go`, `backend/internal/delivery/http/users.go`
- Modifier : `backend/internal/delivery/http/auth.go`, `backend/internal/delivery/http/router.go`, `backend/internal/delivery/http/router_test.go`, `backend/cmd/manager/main.go`, `docs/API_CONTRACTS.md`, `docs/SECURITY.md`
- Tests : `backend/internal/delivery/http/users_test.go`

**Interfaces :**
- Consomme : `usecase.AccountUsecase` (tâche 4), `domain.Role.Can`, `domain.Perm*`, `domain.NewUser.Normalize`, `domain.UserChange.Normalize`, `domain.ValidateNewPassword`, `domain.IsValidationError`.
- Produit (contrat consommé par l'interface, tâches 6–8) :
  - `NewRouter(logger, health, auth, monitoring, accounts *usecase.AccountUsecase, sessionSecure bool, webRoot string)`.
  - Utilisateur de session : `{ "id", "username", "displayName", "role", "mustChangePassword" }` dans les réponses de `POST /auth/login` et `GET /auth/session`.
  - Compte : `{ "id", "username", "displayName", "role", "disabled", "mustChangePassword", "lastLoginAt": string|null, "lastLoginIp": string|null, "createdAt" }`.
  - `GET /api/v1/users` → `200 { "users": [Compte] }` ; `POST /api/v1/users` → `201 { "user": Compte }` ; `PATCH /api/v1/users/{userID}` → `200 { "user": Compte }` ; `DELETE /api/v1/users/{userID}` → `200 { "deleted": true }`.
  - Codes : `403 FORBIDDEN`, `403 PASSWORD_CHANGE_REQUIRED`, `409 USERNAME_TAKEN`, `409 LAST_ADMIN`, `500 USERS_UNAVAILABLE`, `500 ACCOUNT_UPDATE_FAILED`.

- [ ] **Étape 1 : écrire les tests qui échouent**

`backend/internal/delivery/http/users_test.go` :

```go
package httpdelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	argon2adapter "homelab-manager/backend/internal/adapters/argon2"
	"homelab-manager/backend/internal/adapters/sqlite"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func withSessionUser(r *http.Request, user domain.User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), sessionUserKey{}, user))
}

var noContent = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

func TestRequirePermissionAppliesTheRolePolicy(t *testing.T) {
	cases := []struct {
		role domain.Role
		want int
	}{
		{domain.RoleViewer, http.StatusForbidden},
		{domain.RoleOperator, http.StatusForbidden},
		{domain.RoleAdmin, http.StatusNoContent},
	}
	for _, c := range cases {
		recorder := httptest.NewRecorder()
		request := withSessionUser(httptest.NewRequest(http.MethodGet, "/api/v1/users", nil), domain.User{Username: "u", Role: c.role})
		requirePermission(domain.PermManageUsers)(noContent).ServeHTTP(recorder, request)
		if recorder.Code != c.want {
			t.Errorf("role %s got %d, want %d", c.role, recorder.Code, c.want)
		}
	}
	recorder := httptest.NewRecorder()
	requirePermission(domain.PermManageUsers)(noContent).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("without a session got %d, want 401", recorder.Code)
	}
}

func TestRequirePasswordChangedBlocksUntilChanged(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := withSessionUser(httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil), domain.User{Role: domain.RoleAdmin, MustChangePassword: true})
	requirePasswordChanged(noContent).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || errorCode(t, recorder) != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatalf("got %d %s, want 403 PASSWORD_CHANGE_REQUIRED", recorder.Code, recorder.Body)
	}
	recorder = httptest.NewRecorder()
	request = withSessionUser(httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil), domain.User{Role: domain.RoleViewer})
	requirePasswordChanged(noContent).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("got %d, want 204", recorder.Code)
	}
}

func TestCreateUserValidatesTheBody(t *testing.T) {
	// A nil usecase proves validation happens before any change.
	handler := createUserHandler(nil)
	for _, body := range []string{
		``,
		`{}`,
		`{"username":"Alice","displayName":"","role":"viewer","password":"initial-password-1"}`,
		`{"username":"alice","displayName":"","role":"root","password":"initial-password-1"}`,
		`{"username":"alice","displayName":"","role":"viewer","password":"short"}`,
		`{"username":"alice","displayName":"","role":"viewer","password":"initial-password-1","extra":1}`,
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("body %q returned %d, want 400", body, recorder.Code)
		}
	}
}

func TestUpdateUserValidatesPathAndBody(t *testing.T) {
	router := chi.NewRouter()
	router.Patch("/api/v1/users/{userID}", updateUserHandler(nil))
	const id = "9369be23-5ad3-467d-b954-470b622a1442"
	cases := []struct{ path, body string }{
		{"/api/v1/users/not-a-uuid", `{"role":"viewer"}`},
		{"/api/v1/users/9369BE23-5AD3-467D-B954-470B622A1442", `{"role":"viewer"}`},
		{"/api/v1/users/" + id, `{}`},
		{"/api/v1/users/" + id, `{"role":"root"}`},
		{"/api/v1/users/" + id, `{"displayName":"   "}`},
		{"/api/v1/users/" + id, `{"disabled":"yes"}`},
	}
	for _, c := range cases {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, c.path, strings.NewReader(c.body)))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("PATCH %s %s returned %d, want 400", c.path, c.body, recorder.Code)
		}
	}
}

func TestAccountAdministrationEndToEnd(t *testing.T) {
	router := newAccountsTestRouter(t)
	admin := signIn(t, router, "admin", "admin-password-1")

	const aliceBody = `{"username":"alice","displayName":"Alice","role":"viewer","password":"initial-password-1"}`
	created := call(router, http.MethodPost, "/api/v1/users", aliceBody, admin)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body)
	}
	var createdBody struct {
		User userDTO `json:"user"`
	}
	decodeBody(t, created, &createdBody)
	if !createdBody.User.MustChangePassword || createdBody.User.Role != "viewer" || createdBody.User.LastLoginAt != nil {
		t.Fatalf("created account = %+v", createdBody.User)
	}
	aliceID := createdBody.User.ID.String()
	if response := call(router, http.MethodPost, "/api/v1/users", aliceBody, admin); response.Code != http.StatusConflict || errorCode(t, response) != "USERNAME_TAKEN" {
		t.Fatalf("duplicate = %d %s, want 409 USERNAME_TAKEN", response.Code, response.Body)
	}

	alice := signIn(t, router, "alice", "initial-password-1")
	if response := call(router, http.MethodGet, "/api/v1/settings", "", alice); errorCode(t, response) != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatalf("before the change = %d %s, want PASSWORD_CHANGE_REQUIRED", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/password", `{"currentPassword":"initial-password-1","newPassword":"alice-own-password"}`, alice); response.Code != http.StatusOK {
		t.Fatalf("password change = %d %s", response.Code, response.Body)
	}
	alice = signIn(t, router, "alice", "alice-own-password")
	if response := call(router, http.MethodGet, "/api/v1/users", "", alice); response.Code != http.StatusForbidden || errorCode(t, response) != "FORBIDDEN" {
		t.Fatalf("viewer listing accounts = %d %s, want 403 FORBIDDEN", response.Code, response.Body)
	}
	if response := call(router, http.MethodPut, "/api/v1/settings/retention", `{"metricsRetentionDays":3}`, alice); response.Code != http.StatusForbidden {
		t.Fatalf("viewer changing the retention = %d, want 403", response.Code)
	}

	var list struct {
		Users []userDTO `json:"users"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/users", "", admin), &list)
	if len(list.Users) != 2 || list.Users[0].Username != "admin" {
		t.Fatalf("listed accounts = %+v", list.Users)
	}
	adminID := list.Users[0].ID.String()
	if response := call(router, http.MethodPatch, "/api/v1/users/"+adminID, `{"role":"viewer"}`, admin); response.Code != http.StatusConflict || errorCode(t, response) != "LAST_ADMIN" {
		t.Fatalf("demoting the last admin = %d %s, want 409 LAST_ADMIN", response.Code, response.Body)
	}

	if response := call(router, http.MethodPatch, "/api/v1/users/"+aliceID, `{"disabled":true}`, admin); response.Code != http.StatusOK {
		t.Fatalf("disable = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodGet, "/api/v1/auth/session", "", alice); response.Code != http.StatusUnauthorized {
		t.Fatalf("session of a disabled account = %d, want 401", response.Code)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"alice-own-password"}`, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("login of a disabled account = %d, want 401", response.Code)
	}

	if response := call(router, http.MethodDelete, "/api/v1/users/"+aliceID, "", admin); response.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodDelete, "/api/v1/users/"+aliceID, "", admin); response.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d, want 404", response.Code)
	}
}

func newAccountsTestRouter(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	hasher := argon2adapter.New()
	hash, err := hasher.Hash("admin-password-1")
	if err != nil {
		t.Fatal(err)
	}
	authRepository := sqlite.NewAuthRepository(database.Client())
	if _, err := authRepository.EnsureAdmin(ctx, "admin", hash); err != nil {
		t.Fatal(err)
	}
	auth := usecase.NewAuthUsecase(authRepository, authRepository, hasher)
	accounts := usecase.NewAccountUsecase(sqlite.NewAccountRepository(database.Client()), authRepository, hasher)
	return NewRouter(zap.NewNop(), usecase.NewHealthUsecase(time.Now()), auth, nil, accounts, false, "")
}

// call sends a request from a fixed address so that the login rate limit
// of this test does not depend on the other tests of the package.
func call(handler http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.RemoteAddr = "198.51.100.7:4000"
	if cookie != nil {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func signIn(t *testing.T, handler http.Handler, username, password string) *http.Cookie {
	t.Helper()
	response := call(handler, http.MethodPost, "/api/v1/auth/login", fmt.Sprintf(`{"username":%q,"password":%q}`, username, password), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("login %s = %d %s", username, response.Code, response.Body)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie
		}
	}
	t.Fatalf("login %s set no session cookie", username)
	return nil
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), destination); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body, err)
	}
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body errorResponse
	decodeBody(t, recorder, &body)
	return body.Error.Code
}
```

- [ ] **Étape 2 : lancer les tests et constater l'échec**

Commande : `go -C backend test ./internal/delivery/http/ -count=1`
Attendu : échec de compilation, `undefined: requirePermission`, `undefined: createUserHandler`, `undefined: userDTO`, et `too many arguments in call to NewRouter`.

- [ ] **Étape 3 : middlewares et DTO de session**

`backend/internal/delivery/http/access.go` :

```go
package httpdelivery

import (
	"net/http"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

// sessionUserDTO is the signed-in user returned by login and session.
type sessionUserDTO struct {
	ID                 uuid.UUID `json:"id"`
	Username           string    `json:"username"`
	DisplayName        string    `json:"displayName"`
	Role               string    `json:"role"`
	MustChangePassword bool      `json:"mustChangePassword"`
}

func toSessionUserDTO(user domain.User) sessionUserDTO {
	return sessionUserDTO{
		ID: user.ID, Username: user.Username, DisplayName: user.DisplayName,
		Role: string(user.Role), MustChangePassword: user.MustChangePassword,
	}
}

// requirePermission refuses the request unless the role of the session
// grants p. The policy itself is domain.Role.Can.
func requirePermission(p domain.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := sessionUser(r)
			if !ok {
				writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
				return
			}
			if !user.Role.Can(p) {
				writeError(w, r, http.StatusForbidden, "FORBIDDEN", "The account role does not allow this action.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requirePasswordChanged blocks the routes it guards until an account
// created by an administrator has chosen its own password. Logout and the
// password change itself stay outside this guard.
func requirePasswordChanged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, ok := sessionUser(r); ok && user.MustChangePassword {
			writeError(w, r, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "The password must be changed first.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

Dans `backend/internal/delivery/http/auth.go`, dans `loginHandler` **et** `sessionHandler`, remplacer :

```go
		writeJSON(w, http.StatusOK, map[string]any{
			"user": map[string]any{"id": user.ID, "username": user.Username},
		})
```

par :

```go
		writeJSON(w, http.StatusOK, map[string]any{"user": toSessionUserDTO(user)})
```

- [ ] **Étape 4 : handlers des comptes**

`backend/internal/delivery/http/users.go` :

```go
package httpdelivery

import (
	"errors"
	"net/http"
	"regexp"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

var userIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// userDTO is an account as shown to administrators; never the password hash.
type userDTO struct {
	ID                 uuid.UUID  `json:"id"`
	Username           string     `json:"username"`
	DisplayName        string     `json:"displayName"`
	Role               string     `json:"role"`
	Disabled           bool       `json:"disabled"`
	MustChangePassword bool       `json:"mustChangePassword"`
	LastLoginAt        *time.Time `json:"lastLoginAt"`
	LastLoginIP        *string    `json:"lastLoginIp"`
	CreatedAt          time.Time  `json:"createdAt"`
}

func toUserDTO(user domain.User) userDTO {
	dto := userDTO{
		ID: user.ID, Username: user.Username, DisplayName: user.DisplayName,
		Role: string(user.Role), Disabled: user.Disabled,
		MustChangePassword: user.MustChangePassword, LastLoginAt: user.LastLoginAt,
		CreatedAt: user.CreatedAt,
	}
	if user.LastLoginIP != "" {
		ip := user.LastLoginIP
		dto.LastLoginIP = &ip
	}
	return dto
}

type createUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Password    string `json:"password"`
}

type updateUserRequest struct {
	DisplayName *string `json:"displayName"`
	Role        *string `json:"role"`
	Disabled    *bool   `json:"disabled"`
}

func listUsersHandler(accounts *usecase.AccountUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := accounts.List(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "USERS_UNAVAILABLE", "Could not list the accounts.")
			return
		}
		dtos := make([]userDTO, 0, len(users))
		for _, user := range users {
			dtos = append(dtos, toUserDTO(user))
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": dtos})
	}
}

func createUserHandler(accounts *usecase.AccountUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request createUserRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The request body is invalid.")
			return
		}
		input, err := domain.NewUser{
			Username: request.Username, DisplayName: request.DisplayName, Role: domain.Role(request.Role),
		}.Normalize()
		if err == nil {
			err = domain.ValidateNewPassword(request.Password)
		}
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The account values are invalid.")
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		created, err := accounts.Create(r.Context(), actor, input, request.Password, middleware.GetReqID(r.Context()))
		if err != nil {
			writeAccountError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"user": toUserDTO(created)})
	}
}

func updateUserHandler(accounts *usecase.AccountUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := userIDParam(w, r)
		if !ok {
			return
		}
		var request updateUserRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The request body is invalid.")
			return
		}
		change := domain.UserChange{DisplayName: request.DisplayName, Disabled: request.Disabled}
		if request.Role != nil {
			role := domain.Role(*request.Role)
			change.Role = &role
		}
		change, err := change.Normalize()
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The account values are invalid.")
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		updated, err := accounts.Update(r.Context(), actor, id, change, middleware.GetReqID(r.Context()))
		if err != nil {
			writeAccountError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": toUserDTO(updated)})
	}
}

func deleteUserHandler(accounts *usecase.AccountUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := userIDParam(w, r)
		if !ok {
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		if err := accounts.Delete(r.Context(), actor, id, middleware.GetReqID(r.Context())); err != nil {
			writeAccountError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

// userIDParam validates the {userID} path parameter (canonical lowercase UUID).
func userIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	value := chi.URLParam(r, "userID")
	id, err := uuid.Parse(value)
	if !userIDPattern.MatchString(value) || err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "userID must be a lowercase UUID.")
		return uuid.UUID{}, false
	}
	return id, true
}

func writeAccountError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case domain.IsValidationError(err):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The account values are invalid.")
	case errors.Is(err, domain.ErrUserNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The account does not exist.")
	case errors.Is(err, domain.ErrUsernameTaken):
		writeError(w, r, http.StatusConflict, "USERNAME_TAKEN", "The username is already used.")
	case errors.Is(err, domain.ErrLastAdmin):
		writeError(w, r, http.StatusConflict, "LAST_ADMIN", "The last active administrator must remain.")
	default:
		writeError(w, r, http.StatusInternalServerError, "ACCOUNT_UPDATE_FAILED", "Could not update the account.")
	}
}
```

- [ ] **Étape 5 : routeur, journal des requêtes et composition**

Dans `backend/internal/delivery/http/router.go` :

1. Remplacer la signature de `NewRouter` par :

```go
func NewRouter(logger *zap.Logger, health *usecase.HealthUsecase, auth *usecase.AuthUsecase, monitoring *usecase.MonitoringUsecase, accounts *usecase.AccountUsecase, sessionSecure bool, webRoot string) http.Handler {
```

2. Remplacer tout le bloc `r.Group(func(protected chi.Router) { ... })` par :

```go
		r.Group(func(protected chi.Router) {
			protected.Use(requireSession(auth))
			protected.Use(sameOrigin)
			protected.Post("/auth/logout", logoutHandler(auth, sessionSecure))
			protected.Post("/auth/password", changePasswordHandler(auth, sessionSecure))
			protected.Group(func(active chi.Router) {
				active.Use(requirePasswordChanged)
				active.Get("/dashboard", dashboardHandler(monitoring))
				active.Get("/host", hostHandler(monitoring))
				active.Get("/host/metrics/current", currentMetricsHandler(monitoring, false))
				active.Get("/host/metrics/{metricName}", metricHistoryHandler(monitoring, false))
				active.Get("/host/temperatures", hostTemperaturesHandler(monitoring))
				active.Get("/containers", listContainersHandler(monitoring))
				active.Get("/containers/{containerID}", getContainerHandler(monitoring))
				active.Get("/containers/{containerID}/metrics/current", currentMetricsHandler(monitoring, true))
				active.Get("/containers/{containerID}/metrics/{metricName}", metricHistoryHandler(monitoring, true))
				active.Get("/containers/{containerID}/events", listEventsHandler(monitoring, true))
				active.Get("/events", listEventsHandler(monitoring, false))
				active.Get("/settings", settingsHandler(monitoring))
				active.With(requirePermission(domain.PermManageSettings)).Put("/settings/retention", updateRetentionHandler(monitoring))
				active.Route("/users", func(users chi.Router) {
					users.Use(requirePermission(domain.PermManageUsers))
					users.Get("/", listUsersHandler(accounts))
					users.Post("/", createUserHandler(accounts))
					users.Patch("/{userID}", updateUserHandler(accounts))
					users.Delete("/{userID}", deleteUserHandler(accounts))
				})
			})
		})
```

3. Remplacer `requestLogger` par une version qui journalise le statut (les refus `403` deviennent visibles avec leur `request_id`) :

```go
func requestLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(wrapped, r)
			logger.Info("http request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", wrapped.Status()),
				zap.String("request_id", middleware.GetReqID(r.Context())),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}
```

Dans `backend/internal/delivery/http/router_test.go`, ajouter l'argument `accounts` aux appels existants :

```bash
sed -i 's/nil, nil, false, /nil, nil, nil, false, /' backend/internal/delivery/http/router_test.go
```

Dans `backend/cmd/manager/main.go` :

1. Après `var authUsecase *usecase.AuthUsecase`, ajouter `var accountUsecase *usecase.AccountUsecase`.
2. Après `authUsecase = usecase.NewAuthUsecase(authRepository, authRepository, hasher)`, ajouter :

```go
		accountUsecase = usecase.NewAccountUsecase(sqlite.NewAccountRepository(client), authRepository, hasher)
```

3. Remplacer l'appel `httpdelivery.NewRouter(...)` par :

```go
	router := httpdelivery.NewRouter(logger, healthUsecase, authUsecase, monitoringUsecase, accountUsecase, config.SessionSecure, config.WebRoot)
```

- [ ] **Étape 6 : lancer les tests et constater le succès**

Commande : `go -C backend test ./... -count=1 && go -C backend test -race ./internal/delivery/http/ ./internal/usecase/ -count=1`
Attendu : tous les paquets `ok`, y compris `TestAccountAdministrationEndToEnd`.

- [ ] **Étape 7 : contrat d'API**

Dans `docs/API_CONTRACTS.md` :

1. Dans le tableau du § 2, après la ligne `PUT /api/v1/settings/retention`, remplacer la colonne « Session » de cette ligne par `admin` et ajouter :

```markdown
| `GET /api/v1/users` | admin | Comptes (§ 3.1) |
| `POST /api/v1/users` | admin | Crée un compte (§ 3.1) |
| `PATCH /api/v1/users/{userID}` | admin | Modifie un compte (§ 3.1) |
| `DELETE /api/v1/users/{userID}` | admin | Supprime un compte (§ 3.1) |
```

2. Remplacer « Toutes les routes de supervision sont en lecture seule ; les seules écritures sont l'authentification et `PUT /api/v1/settings/retention`. » par « Toutes les routes de supervision sont en lecture seule ; les seules écritures sont l'authentification, `PUT /api/v1/settings/retention` et la gestion des comptes. La colonne « Session » indique `admin` pour les routes réservées au rôle administrateur (§ 3.1). »

3. Dans « Paramètres de chemin », ajouter : « `userID` : UUID Manager en minuscules (`8-4-4-4-12`). Sinon `400 VALIDATION_ERROR` ; inconnu → `404 NOT_FOUND`. »

4. Au § 3, remplacer « Succès `200` : `{ "user": { "id": "uuid", "username": "admin" } }` et cookie de session. » par :

```markdown
- Succès `200` : `{ "user": { "id": "uuid", "username": "admin", "displayName": "admin", "role": "admin", "mustChangePassword": false } }` et cookie de session. Un compte désactivé reçoit `401 INVALID_CREDENTIALS`, comme un mot de passe faux.
```

et « **Session** — `GET /api/v1/auth/session` : `200 { "user": { "id", "username" } }`, sinon `401 SESSION_NOT_FOUND`. » par « **Session** — `GET /api/v1/auth/session` : `200 { "user": … }` (même objet que la connexion), sinon `401 SESSION_NOT_FOUND`, y compris pour un compte désactivé ou supprimé. »

5. Ajouter à la fin du § 3 :

```markdown
### 3.1 Comptes et rôles

Comptes de Homelab Manager uniquement : aucun compte Linux n'est lu ni modifié.

**Rôles** — `viewer` : consultation et son propre mot de passe. `operator` : idem en 1A (actions sur les services et sauvegardes dans les phases suivantes). `admin` : idem, plus `PUT /api/v1/settings/retention` et les routes `/api/v1/users`. Un rôle insuffisant reçoit `403 FORBIDDEN`.

**Mot de passe initial** — un compte créé par un administrateur a `mustChangePassword: true`. Tant qu'il ne l'a pas changé (`POST /api/v1/auth/password`), toutes les routes authentifiées sauf `/auth/session`, `/auth/logout` et `/auth/password` répondent `403 PASSWORD_CHANGE_REQUIRED`.

**Compte** :

```json
{
  "id": "uuid", "username": "alice", "displayName": "Alice", "role": "viewer",
  "disabled": false, "mustChangePassword": true,
  "lastLoginAt": null, "lastLoginIp": null, "createdAt": "2026-11-23T10:00:00Z"
}
```

- `GET /api/v1/users` : `200 { "users": [Compte] }`, triés par identifiant, 500 au plus.
- `POST /api/v1/users` avec `{ "username", "displayName", "role", "password" }` : `username` selon `^[a-z0-9._-]{1,64}$` ; `displayName` 128 octets au plus, sans caractère de contrôle, espaces de bord retirés, identifiant par défaut ; `role` ∈ `viewer`, `operator`, `admin` ; `password` 12 à 1 024 octets. Succès `201 { "user": Compte }`. Identifiant déjà pris : `409 USERNAME_TAKEN`.
- `PATCH /api/v1/users/{userID}` avec au moins un champ parmi `{ "displayName", "role", "disabled" }` : succès `200 { "user": Compte }`. Désactiver un compte révoque ses sessions.
- `DELETE /api/v1/users/{userID}` : `200 { "deleted": true }`. Les sessions du compte sont supprimées ; ses lignes d'audit sont conservées.
- Le dernier administrateur actif ne peut être ni rétrogradé, ni désactivé, ni supprimé : `409 LAST_ADMIN`.
- Chaque modification réussie écrit une ligne d'audit (`user`, UUID du compte, `create` / `update` / `delete`) dans la même transaction.
```

6. Au § 8, ajouter au tableau :

```markdown
| `FORBIDDEN` | 403 | Rôle insuffisant pour la route |
| `PASSWORD_CHANGE_REQUIRED` | 403 | Mot de passe initial à changer d'abord |
| `USERNAME_TAKEN` | 409 | Identifiant de compte déjà utilisé |
| `LAST_ADMIN` | 409 | Dernier administrateur actif à conserver |
| `USERS_UNAVAILABLE` | 500 | Lecture des comptes en échec |
| `ACCOUNT_UPDATE_FAILED` | 500 | Écriture d'un compte en échec |
```

et remplacer la ligne `NOT_FOUND` par `| \`NOT_FOUND\` | 404 | Route, conteneur ou compte inconnu |`.

- [ ] **Étape 8 : sécurité**

Dans `docs/SECURITY.md`, section « Authentification locale et HTTP », ajouter après la puce « Changement de mot de passe » :

```markdown
- Comptes multiples, propres à Homelab Manager (aucun compte Linux) : rôles `viewer`, `operator`, `admin` vérifiés côté serveur par un middleware unique à partir de la politique du domaine ; l'interface masque les actions non permises sans que ce soit une protection.
- Un compte créé par un administrateur reçoit un mot de passe initial (12 octets au moins) qu'il doit changer avant toute autre route. Un compte désactivé ne peut plus se connecter et ses sessions sont révoquées ; une session existante est de toute façon refusée dès la désactivation. Le dernier administrateur actif ne peut être ni rétrogradé, ni désactivé, ni supprimé.
- Chaque requête est journalisée avec son statut et son `requestId` : les refus `403` sont ainsi traçables. L'audit des refus dans `audit_logs` arrive avec la phase 1B.
```

et remplacer « Journal d'audit : toute modification faite depuis l'interface (aujourd'hui la rétention) est tracée » par « Journal d'audit : toute modification faite depuis l'interface (rétention, comptes) est tracée ».

- [ ] **Étape 9 : commit**

```bash
gofmt -l backend   # doit être vide
scripts/verify.sh
git add backend/internal/delivery/http backend/cmd/manager/main.go docs/API_CONTRACTS.md docs/SECURITY.md
git commit -m "feat(api): add role-based authorization and account routes"
```

---

### Tâche 6 : interface — clients d'API, rôles, schéma et messages

**Fichiers :**
- Créer : `frontend/src/lib/api/users.ts`, `frontend/src/lib/roles.ts`, `frontend/src/lib/accounts.ts`, `frontend/src/schemas/account.ts`
- Modifier : `frontend/src/lib/api/auth.ts`, `frontend/src/lib/api/client.ts`
- Tests : `frontend/src/lib/api/users.test.ts`, `frontend/src/lib/roles.test.ts`, `frontend/src/lib/accounts.test.ts`, `frontend/src/schemas/account.test.ts`

**Interfaces :**
- Consomme : contrat de la tâche 5.
- Produit :
  - `type Role = 'viewer' | 'operator' | 'admin'` ; `SessionUser = { id, username, displayName, role: Role, mustChangePassword: boolean }` (`lib/api/auth.ts`).
  - `lib/api/users.ts` : `Account`, `NewAccount`, `AccountChange`, `listAccounts(): Promise<Account[]>`, `createAccount(input): Promise<Account>`, `updateAccount(id, change): Promise<Account>`, `deleteAccount(id): Promise<{ deleted: boolean }>`.
  - `lib/roles.ts` : `roleOptions: Array<{ value: Role; label: string }>`, `roleLabel(role)`, `isRole(value: string): value is Role`, `canManageUsers(role?)`, `canManageSettings(role?)`.
  - `lib/accounts.ts` : `AccountRow`, `toAccountRow(account, currentUserId?)`.
  - `schemas/account.ts` : `accountSchema`, `AccountValues`.

- [ ] **Étape 1 : écrire les tests qui échouent**

`frontend/src/lib/api/users.test.ts` :

```ts
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createAccount, deleteAccount, listAccounts, updateAccount, type Account } from './users'

const account: Account = {
  id: '9369be23-5ad3-467d-b954-470b622a1442',
  username: 'alice',
  displayName: 'Alice',
  role: 'viewer',
  disabled: false,
  mustChangePassword: true,
  lastLoginAt: null,
  lastLoginIp: null,
  createdAt: '2026-11-23T10:00:00Z',
}

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('accounts API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('lists the accounts', async () => {
    const fetchMock = mockFetch({ users: [account] })
    await expect(listAccounts()).resolves.toEqual([account])
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/users')
  })

  it('creates an account', async () => {
    const fetchMock = mockFetch({ user: account }, 201)
    const input = {
      username: 'alice',
      displayName: 'Alice',
      role: 'viewer' as const,
      password: 'initial-password-1',
    }
    await expect(createAccount(input)).resolves.toEqual(account)
    const [path, init] = fetchMock.mock.calls[0]
    expect(path).toBe('/api/v1/users')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toEqual(input)
  })

  it('sends only the changed fields', async () => {
    const fetchMock = mockFetch({ user: { ...account, disabled: true } })
    await updateAccount(account.id, { disabled: true })
    const [path, init] = fetchMock.mock.calls[0]
    expect(path).toBe(`/api/v1/users/${account.id}`)
    expect(init.method).toBe('PATCH')
    expect(JSON.parse(init.body)).toEqual({ disabled: true })
  })

  it('deletes an account', async () => {
    const fetchMock = mockFetch({ deleted: true })
    await expect(deleteAccount(account.id)).resolves.toEqual({ deleted: true })
    expect(fetchMock.mock.calls[0][1].method).toBe('DELETE')
  })

  it('translates the last-administrator refusal', async () => {
    mockFetch({ error: { code: 'LAST_ADMIN', message: 'The last active administrator must remain.' } }, 409)
    await expect(updateAccount(account.id, { role: 'viewer' })).rejects.toThrow(
      'Le dernier administrateur actif ne peut être ni supprimé, ni rétrogradé, ni désactivé.',
    )
  })
})
```

`frontend/src/lib/roles.test.ts` :

```ts
import { describe, expect, it } from 'vitest'
import { canManageSettings, canManageUsers, isRole, roleLabel, roleOptions } from './roles'

describe('roles', () => {
  it('labels every role in French, in increasing order of rights', () => {
    expect(roleOptions).toEqual([
      { value: 'viewer', label: 'Lecteur' },
      { value: 'operator', label: 'Opérateur' },
      { value: 'admin', label: 'Administrateur' },
    ])
    expect(roleLabel('admin')).toBe('Administrateur')
  })

  it('recognises only the defined roles', () => {
    expect(isRole('operator')).toBe(true)
    expect(isRole('root')).toBe(false)
    expect(isRole('toString')).toBe(false)
  })

  it('mirrors the server policy for display only', () => {
    expect(canManageUsers('admin')).toBe(true)
    expect(canManageUsers('operator')).toBe(false)
    expect(canManageUsers(undefined)).toBe(false)
    expect(canManageSettings('admin')).toBe(true)
    expect(canManageSettings('viewer')).toBe(false)
  })
})
```

`frontend/src/lib/accounts.test.ts` :

```ts
import { describe, expect, it } from 'vitest'
import type { Account } from './api/users'
import { toAccountRow } from './accounts'

const base: Account = {
  id: 'a',
  username: 'alice',
  displayName: 'Alice',
  role: 'operator',
  disabled: false,
  mustChangePassword: false,
  lastLoginAt: '2026-11-23T10:00:00Z',
  lastLoginIp: '192.0.2.4',
  createdAt: '2026-11-20T10:00:00Z',
}

describe('toAccountRow', () => {
  it('describes an active account', () => {
    const row = toAccountRow(base, 'b')
    expect(row.roleLabel).toBe('Opérateur')
    expect(row.statusLabel).toBe('Actif')
    expect(row.statusTone).toBe('success')
    expect(row.toggleLabel).toBe('Désactiver')
    expect(row.lastLogin).toContain('192.0.2.4')
    expect(row.isSelf).toBe(false)
  })

  it('flags the pending password change and the current account', () => {
    const row = toAccountRow({ ...base, mustChangePassword: true, lastLoginAt: null, lastLoginIp: null }, 'a')
    expect(row.statusLabel).toBe('Mot de passe à changer')
    expect(row.statusTone).toBe('warning')
    expect(row.lastLogin).toBe('Jamais')
    expect(row.isSelf).toBe(true)
  })

  it('puts the disabled state first', () => {
    const row = toAccountRow({ ...base, disabled: true, mustChangePassword: true }, undefined)
    expect(row.statusLabel).toBe('Désactivé')
    expect(row.statusTone).toBe('neutral')
    expect(row.toggleLabel).toBe('Réactiver')
  })
})
```

`frontend/src/schemas/account.test.ts` :

```ts
import { describe, expect, it } from 'vitest'
import { accountSchema } from './account'

const valid = { username: 'alice', displayName: '', role: 'viewer', password: 'initial-password-1' }

describe('accountSchema', () => {
  it('accepts a valid account with an empty display name', () => {
    expect(accountSchema.safeParse(valid).success).toBe(true)
  })

  it.each([
    { ...valid, username: 'Alice' },
    { ...valid, username: '' },
    { ...valid, username: 'a'.repeat(65) },
    { ...valid, role: 'root' },
    { ...valid, password: 'short' },
    { ...valid, displayName: 'x'.repeat(129) },
  ])('rejects %o', (values) => {
    expect(accountSchema.safeParse(values).success).toBe(false)
  })
})
```

- [ ] **Étape 2 : lancer les tests et constater l'échec**

Commande : `npm --prefix frontend run test -- src/lib/api/users.test.ts src/lib/roles.test.ts src/lib/accounts.test.ts src/schemas/account.test.ts`
Attendu : échec, modules `./users`, `./roles`, `./accounts`, `./account` introuvables.

- [ ] **Étape 3 : implémentation**

Dans `frontend/src/lib/api/auth.ts`, remplacer le type `SessionUser` par :

```ts
export type Role = 'viewer' | 'operator' | 'admin'

export type SessionUser = {
  id: string
  username: string
  displayName: string
  role: Role
  /** The account must choose its own password before anything else. */
  mustChangePassword: boolean
}
```

`frontend/src/lib/api/users.ts` :

```ts
import type { Role } from './auth'
import { requestJSON } from './client'

/** A Manager account as returned to administrators (API_CONTRACTS.md § 3.1). */
export type Account = {
  id: string
  username: string
  displayName: string
  role: Role
  disabled: boolean
  mustChangePassword: boolean
  lastLoginAt: string | null
  lastLoginIp: string | null
  createdAt: string
}

export type NewAccount = {
  username: string
  displayName: string
  role: Role
  password: string
}

export type AccountChange = {
  displayName?: string
  role?: Role
  disabled?: boolean
}

export function listAccounts() {
  return requestJSON<{ users: Account[] }>('/api/v1/users').then((body) => body.users)
}

export function createAccount(input: NewAccount) {
  return requestJSON<{ user: Account }>('/api/v1/users', {
    method: 'POST',
    body: JSON.stringify(input),
  }).then((body) => body.user)
}

export function updateAccount(id: string, change: AccountChange) {
  return requestJSON<{ user: Account }>(`/api/v1/users/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(change),
  }).then((body) => body.user)
}

export function deleteAccount(id: string) {
  return requestJSON<{ deleted: boolean }>(`/api/v1/users/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}
```

Dans `frontend/src/lib/api/client.ts`, ajouter à `errorMessages`, avant `INTERNAL_ERROR` :

```ts
  FORBIDDEN: 'Votre rôle ne permet pas cette action.',
  PASSWORD_CHANGE_REQUIRED: 'Choisissez votre mot de passe pour continuer.',
  USERNAME_TAKEN: 'Cet identifiant est déjà utilisé.',
  LAST_ADMIN:
    'Le dernier administrateur actif ne peut être ni supprimé, ni rétrogradé, ni désactivé.',
  USERS_UNAVAILABLE: 'Comptes momentanément indisponibles.',
  ACCOUNT_UPDATE_FAILED: "Le compte n'a pas pu être modifié.",
```

`frontend/src/lib/roles.ts` :

```ts
import type { Role } from './api/auth'

const labels: Record<Role, string> = {
  viewer: 'Lecteur',
  operator: 'Opérateur',
  admin: 'Administrateur',
}

/** Roles in increasing order of rights, for select inputs. */
export const roleOptions: Array<{ value: Role; label: string }> = (
  ['viewer', 'operator', 'admin'] as const
).map((value) => ({ value, label: labels[value] }))

export function roleLabel(role: Role): string {
  return labels[role]
}

export function isRole(value: string): value is Role {
  return roleOptions.some((option) => option.value === value)
}

// Display-only mirror of the server policy (SPEC_ADMINISTRATION.md § 3.2):
// it hides actions, the server decides.
export function canManageUsers(role: Role | undefined): boolean {
  return role === 'admin'
}

export function canManageSettings(role: Role | undefined): boolean {
  return role === 'admin'
}
```

`frontend/src/lib/accounts.ts` :

```ts
import type { Role } from './api/auth'
import type { Account } from './api/users'
import { formatDateTime } from './format'
import { roleLabel } from './roles'

export type AccountRow = {
  id: string
  username: string
  displayName: string
  role: Role
  roleLabel: string
  disabled: boolean
  statusLabel: string
  statusTone: 'success' | 'warning' | 'neutral'
  /** Label of the button that toggles the disabled state. */
  toggleLabel: string
  lastLogin: string
  isSelf: boolean
}

function accountStatus(account: Account): Pick<AccountRow, 'statusLabel' | 'statusTone'> {
  if (account.disabled) return { statusLabel: 'Désactivé', statusTone: 'neutral' }
  if (account.mustChangePassword)
    return { statusLabel: 'Mot de passe à changer', statusTone: 'warning' }
  return { statusLabel: 'Actif', statusTone: 'success' }
}

function lastLogin(account: Account): string {
  if (!account.lastLoginAt) return 'Jamais'
  const when = formatDateTime(account.lastLoginAt)
  return account.lastLoginIp ? `${when} · ${account.lastLoginIp}` : when
}

/** Table row of the accounts page; the current account is flagged. */
export function toAccountRow(account: Account, currentUserId: string | undefined): AccountRow {
  return {
    id: account.id,
    username: account.username,
    displayName: account.displayName,
    role: account.role,
    roleLabel: roleLabel(account.role),
    disabled: account.disabled,
    ...accountStatus(account),
    toggleLabel: account.disabled ? 'Réactiver' : 'Désactiver',
    lastLogin: lastLogin(account),
    isSelf: account.id === currentUserId,
  }
}
```

`frontend/src/schemas/account.ts` :

```ts
import { z } from 'zod'

// Bounds of API_CONTRACTS.md § 3.1. The server counts bytes; these counts
// are characters, so the server remains the reference.
export const accountSchema = z.object({
  username: z
    .string()
    .regex(/^[a-z0-9._-]{1,64}$/, 'Minuscules, chiffres, « . », « _ » ou « - », 64 au plus'),
  displayName: z.string().max(128, 'Maximum 128 caractères'),
  role: z.enum(['viewer', 'operator', 'admin']),
  password: z.string().min(12, 'Au moins 12 caractères').max(1024, 'Maximum 1 024 caractères'),
})

export type AccountValues = z.infer<typeof accountSchema>
```

- [ ] **Étape 4 : lancer les tests et constater le succès**

Commande : `npm --prefix frontend run test -- src/lib src/schemas && npm --prefix frontend run typecheck`
Attendu : tous les tests passent ; `typecheck` sans erreur.

- [ ] **Étape 5 : commit**

```bash
npm --prefix frontend run format
scripts/verify.sh
git add frontend/src/lib frontend/src/schemas
git commit -m "feat(frontend): add accounts API client, role labels and account schema"
```

---

### Tâche 7 : interface — rôle dans la session et changement de mot de passe imposé

**Fichiers :**
- Créer : `frontend/src/features/auth/useForcedPasswordChange.ts`, `frontend/src/features/auth/ForcedPasswordChangePage.tsx`
- Modifier : `frontend/src/hooks/useSession.ts`, `frontend/src/app/RequireAuth.tsx`, `frontend/src/app/router.tsx`
- Tests : `frontend/src/app/RequireAuth.test.tsx`, `frontend/src/features/auth/ForcedPasswordChangePage.test.tsx`

**Interfaces :**
- Consomme : `SessionUser` (tâche 6), `roleLabel`, `canManageUsers`, `canManageSettings`, `useChangePassword` (existant).
- Produit : `useSession()` renvoie en plus `mustChangePassword: boolean`, `roleLabel: string | null`, `canManageUsers: boolean`, `canManageSettings: boolean`. Route `/password-change`.

- [ ] **Étape 1 : écrire les tests qui échouent**

`frontend/src/app/RequireAuth.test.tsx` :

```tsx
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { RequireAuth } from './RequireAuth'

const session = vi.hoisted(() => ({
  user: {
    id: 'a',
    username: 'alice',
    displayName: 'Alice',
    role: 'viewer' as const,
    mustChangePassword: true,
  },
  isPending: false,
  unauthenticated: false,
  error: null,
  mustChangePassword: true,
  roleLabel: 'Lecteur',
  canManageUsers: false,
  canManageSettings: false,
}))

vi.mock('../hooks/useSession', () => ({ useSession: () => session }))

describe('RequireAuth', () => {
  it('sends an account with an initial password to the password change', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route
            path="/"
            element={
              <RequireAuth>
                <p>Contenu protégé</p>
              </RequireAuth>
            }
          />
          <Route path="/password-change" element={<p>Changement imposé</p>} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('Changement imposé')).toBeInTheDocument()
    expect(screen.queryByText('Contenu protégé')).not.toBeInTheDocument()
  })
})
```

`frontend/src/features/auth/ForcedPasswordChangePage.test.tsx` :

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { changePassword } from '../../lib/api/auth'
import { ForcedPasswordChangePage } from './ForcedPasswordChangePage'

const session = vi.hoisted(() => ({
  user: {
    id: 'a',
    username: 'alice',
    displayName: 'Alice',
    role: 'viewer' as const,
    mustChangePassword: true,
  },
  isPending: false,
  unauthenticated: false,
  error: null,
  mustChangePassword: true,
  roleLabel: 'Lecteur',
  canManageUsers: false,
  canManageSettings: false,
}))

vi.mock('../../hooks/useSession', () => ({
  sessionQueryKey: ['session'],
  useSession: () => session,
}))

vi.mock('../../lib/api/auth', () => ({
  changePassword: vi.fn().mockResolvedValue({ changed: true }),
  ApiError: class ApiError extends Error {
    status = 401
  },
}))

describe('ForcedPasswordChangePage', () => {
  it('asks the account to choose its own password', async () => {
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <ForcedPasswordChangePage />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(screen.getByRole('heading', { name: 'Choisissez votre mot de passe' })).toBeInTheDocument()
    await user.type(screen.getByLabelText('Mot de passe initial'), 'initial-password-1')
    await user.type(screen.getByLabelText('Nouveau mot de passe'), 'alice-own-password')
    await user.type(screen.getByLabelText('Confirmation'), 'alice-own-password')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))

    await waitFor(() =>
      expect(changePassword).toHaveBeenCalledWith('initial-password-1', 'alice-own-password'),
    )
  })
})
```

- [ ] **Étape 2 : lancer les tests et constater l'échec**

Commande : `npm --prefix frontend run test -- src/app/RequireAuth.test.tsx src/features/auth/ForcedPasswordChangePage.test.tsx`
Attendu : `RequireAuth` affiche « Contenu protégé » (pas de redirection) ; `ForcedPasswordChangePage` introuvable.

- [ ] **Étape 3 : implémentation**

Dans `frontend/src/hooks/useSession.ts`, ajouter l'import `import { canManageSettings, canManageUsers, roleLabel } from '../lib/roles'` et remplacer le corps de `useSession` après la déclaration de `unauthenticated` par :

```ts
  const user = session.data?.user ?? null
  return {
    user,
    isPending: session.isPending,
    unauthenticated,
    /** Error other than "not signed in" (Manager unreachable, database down…). */
    error: session.isError && !unauthenticated ? session.error : null,
    /** The account must choose its own password before anything else. */
    mustChangePassword: user?.mustChangePassword ?? false,
    roleLabel: user ? roleLabel(user.role) : null,
    canManageUsers: canManageUsers(user?.role),
    canManageSettings: canManageSettings(user?.role),
  }
```

Dans `frontend/src/app/RequireAuth.tsx`, juste avant le `return children` final, ajouter :

```tsx
  if (session.mustChangePassword) {
    return <Navigate to="/password-change" replace />
  }
```

`frontend/src/features/auth/useForcedPasswordChange.ts` :

```ts
import { useSession } from '../../hooks/useSession'
import { useChangePassword } from '../settings/useChangePassword'

/**
 * Password change imposed on an account created by an administrator. Once
 * done, the Manager revokes every session and the user signs in again.
 */
export function useForcedPasswordChange() {
  const session = useSession()
  const password = useChangePassword()
  let redirectTo: '/login' | '/' | null = null
  if (!session.isPending) {
    if (session.unauthenticated) redirectTo = '/login'
    else if (!session.mustChangePassword) redirectTo = '/'
  }
  return {
    checking: session.isPending,
    redirectTo,
    username: session.user?.username ?? null,
    register: password.register,
    errors: password.errors,
    onSubmit: password.onSubmit,
    isSubmitting: password.isPending,
    errorMessage: password.errorMessage,
  }
}
```

`frontend/src/features/auth/ForcedPasswordChangePage.tsx` :

```tsx
import { KeyRound } from 'lucide-react'
import type { ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Logo } from '../../components/ui/Logo'
import { useForcedPasswordChange } from './useForcedPasswordChange'

const inputClass =
  'h-9 w-full rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none focus:border-accent'

export function ForcedPasswordChangePage() {
  const page = useForcedPasswordChange()

  if (page.redirectTo) return <Navigate to={page.redirectTo} replace />

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 bg-background px-4">
      <Logo />
      <Card className="w-full max-w-md">
        <CardHeader
          title="Choisissez votre mot de passe"
          subtitle={
            page.username
              ? `Compte ${page.username} : le mot de passe initial doit être remplacé`
              : 'Le mot de passe initial doit être remplacé'
          }
          icon={<KeyRound className="h-3.5 w-3.5" />}
        />
        <CardBody>
          <form className="space-y-3" onSubmit={page.onSubmit} noValidate>
            <Field id="currentPassword" label="Mot de passe initial" error={page.errors.currentPassword?.message}>
              <input
                id="currentPassword"
                type="password"
                autoComplete="current-password"
                className={inputClass}
                {...page.register('currentPassword')}
              />
            </Field>
            <Field id="newPassword" label="Nouveau mot de passe" error={page.errors.newPassword?.message}>
              <input
                id="newPassword"
                type="password"
                autoComplete="new-password"
                className={inputClass}
                {...page.register('newPassword')}
              />
            </Field>
            <Field id="confirmPassword" label="Confirmation" error={page.errors.confirmPassword?.message}>
              <input
                id="confirmPassword"
                type="password"
                autoComplete="new-password"
                className={inputClass}
                {...page.register('confirmPassword')}
              />
            </Field>
            {page.errorMessage && (
              <p className="text-xs text-danger" role="alert">
                {page.errorMessage}
              </p>
            )}
            <Button type="submit" variant="primary" loading={page.isSubmitting} disabled={page.checking}>
              Enregistrer
            </Button>
            <p className="text-xs text-text-muted">
              Minimum 12 caractères. Vous serez ensuite invité à vous reconnecter.
            </p>
          </form>
        </CardBody>
      </Card>
    </main>
  )
}

function Field({
  id,
  label,
  error,
  children,
}: {
  id: string
  label: string
  error?: string
  children: ReactNode
}) {
  return (
    <div>
      <label
        htmlFor={id}
        className="mb-1.5 block font-mono text-[10px] uppercase tracking-wider text-text-secondary"
      >
        {label}
      </label>
      {children}
      {error && (
        <p className="mt-1 text-[11px] text-danger" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}
```

Dans `frontend/src/app/router.tsx`, ajouter l'import `import { ForcedPasswordChangePage } from '../features/auth/ForcedPasswordChangePage'` et, juste après `{ path: '/login', element: <LoginPage /> },`, la route :

```tsx
  { path: '/password-change', element: <ForcedPasswordChangePage /> },
```

- [ ] **Étape 4 : lancer les tests et constater le succès**

Commande : `npm --prefix frontend run test && npm --prefix frontend run typecheck && npm --prefix frontend run lint`
Attendu : tous les tests passent, y compris `LoginPage` et `DashboardPage` existants ; aucun avertissement ESLint.

- [ ] **Étape 5 : commit**

```bash
npm --prefix frontend run format
scripts/verify.sh
git add frontend/src
git commit -m "feat(frontend): require accounts to replace their initial password"
```

---

### Tâche 8 : interface — page « Utilisateurs & accès », navigation et paramètres

**Fichiers :**
- Créer : `frontend/src/features/users/useUsersPage.ts`, `frontend/src/features/users/UsersPage.tsx`
- Modifier : `frontend/src/app/router.tsx`, `frontend/src/components/layout/AppSidebar.tsx`, `frontend/src/features/settings/useRetentionForm.ts`, `frontend/src/features/settings/RetentionForm.tsx`, `frontend/src/features/settings/SettingsPage.tsx`, `README.md`, `docs/PRODUCT_SPEC.md`, `docs/SPEC_ADMINISTRATION.md`, `docs/TESTING.md`
- Tests : `frontend/src/features/users/UsersPage.test.tsx`

**Interfaces :**
- Consomme : tâches 6 et 7.
- Produit : route `/users` ; entrée de navigation visible des administrateurs ; formulaire de rétention en lecture seule pour les autres rôles.

- [ ] **Étape 1 : écrire le test qui échoue**

`frontend/src/features/users/UsersPage.test.tsx` :

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { deleteAccount, listAccounts, type Account } from '../../lib/api/users'
import { UsersPage } from './UsersPage'

vi.mock('../../lib/api/users', () => ({
  listAccounts: vi.fn(),
  createAccount: vi.fn(),
  updateAccount: vi.fn(),
  deleteAccount: vi.fn(),
}))

vi.mock('../../hooks/useSession', () => ({
  useSession: () => ({
    user: { id: 'admin-id', username: 'admin', displayName: 'admin', role: 'admin', mustChangePassword: false },
    isPending: false,
    unauthenticated: false,
    error: null,
    mustChangePassword: false,
    roleLabel: 'Administrateur',
    canManageUsers: true,
    canManageSettings: true,
  }),
}))

const admin: Account = {
  id: 'admin-id',
  username: 'admin',
  displayName: 'admin',
  role: 'admin',
  disabled: false,
  mustChangePassword: false,
  lastLoginAt: '2026-11-23T09:00:00Z',
  lastLoginIp: '192.0.2.1',
  createdAt: '2026-11-01T09:00:00Z',
}
const alice: Account = {
  ...admin,
  id: 'alice-id',
  username: 'alice',
  displayName: 'Alice',
  role: 'viewer',
  mustChangePassword: true,
  lastLoginAt: null,
  lastLoginIp: null,
}

function renderPage() {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <UsersPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('UsersPage', () => {
  it('lists the accounts and confirms before deleting one', async () => {
    vi.mocked(listAccounts).mockResolvedValue([admin, alice])
    vi.mocked(deleteAccount).mockResolvedValue({ deleted: true })
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText('alice')).toBeInTheDocument()
    expect(screen.getByText('Mot de passe à changer')).toBeInTheDocument()
    expect(screen.getByText('(vous)')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Supprimer alice' }))
    expect(deleteAccount).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Confirmer la suppression' }))
    await waitFor(() => expect(deleteAccount).toHaveBeenCalledWith('alice-id'))
  })

  it('validates the new account before sending it', async () => {
    vi.mocked(listAccounts).mockResolvedValue([admin])
    const user = userEvent.setup()
    renderPage()

    await user.type(screen.getByLabelText('Identifiant'), 'Alice')
    await user.click(screen.getByRole('button', { name: 'Créer le compte' }))
    expect(
      await screen.findByText('Minuscules, chiffres, « . », « _ » ou « - », 64 au plus'),
    ).toBeInTheDocument()
    expect(screen.getByText('Au moins 12 caractères')).toBeInTheDocument()
  })
})
```

- [ ] **Étape 2 : lancer le test et constater l'échec**

Commande : `npm --prefix frontend run test -- src/features/users`
Attendu : échec, `./UsersPage` introuvable.

- [ ] **Étape 3 : hook de page**

`frontend/src/features/users/useUsersPage.ts` :

```ts
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import { toAccountRow, type AccountRow } from '../../lib/accounts'
import {
  createAccount,
  deleteAccount,
  listAccounts,
  updateAccount,
  type AccountChange,
} from '../../lib/api/users'
import { isRole, roleOptions } from '../../lib/roles'
import { accountSchema, type AccountValues } from '../../schemas/account'

export const accountsQueryKey = ['accounts'] as const

/** Logic and view model of UsersPage (administrators only). */
export function useUsersPage() {
  const queryClient = useQueryClient()
  const { user } = useSession()
  const accounts = useQuery({ queryKey: accountsQueryKey, queryFn: listAccounts })
  const refresh = () => queryClient.invalidateQueries({ queryKey: accountsQueryKey })

  const form = useForm<AccountValues>({
    resolver: zodResolver(accountSchema),
    defaultValues: { username: '', displayName: '', role: 'viewer', password: '' },
  })
  const create = useMutation({
    mutationFn: (values: AccountValues) => createAccount(values),
    onSuccess: async () => {
      form.reset()
      await refresh()
    },
  })
  const update = useMutation({
    mutationFn: ({ id, change }: { id: string; change: AccountChange }) => updateAccount(id, change),
    onSettled: refresh,
  })
  const [pendingDeletion, setPendingDeletion] = useState<AccountRow | null>(null)
  const remove = useMutation({
    mutationFn: (id: string) => deleteAccount(id),
    onSuccess: () => setPendingDeletion(null),
    onSettled: refresh,
  })

  const rows = useMemo(
    () => (accounts.data ?? []).map((account) => toAccountRow(account, user?.id)),
    [accounts.data, user?.id],
  )

  return {
    state: loadState(accounts),
    rows,
    roleOptions,
    register: form.register,
    errors: form.formState.errors,
    onCreate: form.handleSubmit((values) => create.mutate(values)),
    isCreating: create.isPending,
    created: create.isSuccess,
    createError: create.error?.message ?? null,
    changeRole: (id: string, value: string) => {
      if (isRole(value)) update.mutate({ id, change: { role: value } })
    },
    toggleDisabled: (row: AccountRow) =>
      update.mutate({ id: row.id, change: { disabled: !row.disabled } }),
    isUpdating: update.isPending,
    actionError: update.error?.message ?? remove.error?.message ?? null,
    pendingDeletion,
    askDelete: (row: AccountRow) => setPendingDeletion(row),
    cancelDelete: () => setPendingDeletion(null),
    confirmDelete: () => {
      if (pendingDeletion) remove.mutate(pendingDeletion.id)
    },
    isDeleting: remove.isPending,
  }
}
```

- [ ] **Étape 4 : page**

`frontend/src/features/users/UsersPage.tsx` :

```tsx
import { Trash2, UserPlus } from 'lucide-react'
import type { ReactNode } from 'react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { Select } from '../../components/ui/Select'
import { useUsersPage } from './useUsersPage'

const inputClass =
  'h-9 w-full rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none focus:border-accent'

export function UsersPage() {
  const page = useUsersPage()

  return (
    <div className="space-y-4">
      <PageHeader
        title="Utilisateurs & accès"
        breadcrumb={[{ label: 'Paramètres' }, { label: 'Utilisateurs & accès' }]}
        description="Comptes de Homelab Manager uniquement : aucun compte Linux de la machine n'est lu ni modifié."
      />

      {page.state.isError && (
        <ErrorState message={page.state.error.message} onRetry={page.state.refetch} />
      )}
      {page.state.isLoading && <LoadingState label="Chargement des comptes…" rows={3} />}
      {page.actionError && (
        <p className="text-xs text-danger" role="alert">
          {page.actionError}
        </p>
      )}

      {page.pendingDeletion && (
        <Card className="border-danger/40">
          <CardBody className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-text-primary">
              Supprimer le compte « {page.pendingDeletion.username} » ? Ses sessions sont
              révoquées ; ses lignes d'audit sont conservées.
            </p>
            <div className="flex gap-2">
              <Button size="sm" onClick={page.cancelDelete}>
                Annuler
              </Button>
              <Button
                size="sm"
                variant="danger"
                loading={page.isDeleting}
                onClick={page.confirmDelete}
              >
                Confirmer la suppression
              </Button>
            </div>
          </CardBody>
        </Card>
      )}

      {page.rows.length > 0 && (
        <Table>
          <THead>
            <TR>
              <TH>Identifiant</TH>
              <TH>Nom affiché</TH>
              <TH>Rôle</TH>
              <TH>État</TH>
              <TH>Dernière connexion</TH>
              <TH>
                <span className="sr-only">Actions</span>
              </TH>
            </TR>
          </THead>
          <TBody>
            {page.rows.map((row) => (
              <TR key={row.id}>
                <TD className="font-mono text-text-primary">
                  {row.username}
                  {row.isSelf && <span className="ml-2 text-[10px] text-text-muted">(vous)</span>}
                </TD>
                <TD>{row.displayName}</TD>
                <TD>
                  <Select
                    aria-label={`Rôle de ${row.username}`}
                    value={row.role}
                    disabled={page.isUpdating}
                    onChange={(event) => page.changeRole(row.id, event.target.value)}
                  >
                    {page.roleOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </Select>
                </TD>
                <TD>
                  <Badge tone={row.statusTone} dot>
                    {row.statusLabel}
                  </Badge>
                </TD>
                <TD className="font-mono text-[11px]">{row.lastLogin}</TD>
                <TD>
                  <div className="flex justify-end gap-2">
                    <Button
                      size="sm"
                      variant="ghost"
                      aria-label={`${row.toggleLabel} ${row.username}`}
                      disabled={page.isUpdating}
                      onClick={() => page.toggleDisabled(row)}
                    >
                      {row.toggleLabel}
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      aria-label={`Supprimer ${row.username}`}
                      icon={<Trash2 className="h-3.5 w-3.5" />}
                      onClick={() => page.askDelete(row)}
                    />
                  </div>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}

      <Card>
        <CardHeader
          title="Nouveau compte"
          subtitle="Le mot de passe initial devra être changé à la première connexion"
          icon={<UserPlus className="h-3.5 w-3.5" />}
        />
        <CardBody>
          <form className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4" onSubmit={page.onCreate} noValidate>
            <Field id="username" label="Identifiant" error={page.errors.username?.message}>
              <input id="username" autoComplete="off" className={inputClass} {...page.register('username')} />
            </Field>
            <Field id="displayName" label="Nom affiché" error={page.errors.displayName?.message}>
              <input id="displayName" autoComplete="off" className={inputClass} {...page.register('displayName')} />
            </Field>
            <Field id="role" label="Rôle" error={page.errors.role?.message}>
              <Select id="role" containerClassName="w-full" {...page.register('role')}>
                {page.roleOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            </Field>
            <Field id="password" label="Mot de passe initial" error={page.errors.password?.message}>
              <input
                id="password"
                type="password"
                autoComplete="new-password"
                className={inputClass}
                {...page.register('password')}
              />
            </Field>
            <div className="flex flex-wrap items-center gap-3 sm:col-span-2 lg:col-span-4">
              <Button
                type="submit"
                variant="primary"
                loading={page.isCreating}
                icon={<UserPlus className="h-3.5 w-3.5" />}
              >
                Créer le compte
              </Button>
              {page.createError && (
                <p className="text-xs text-danger" role="alert">
                  {page.createError}
                </p>
              )}
              {page.created && (
                <p className="text-xs text-success" role="status">
                  Compte créé. Transmettez le mot de passe initial à son titulaire.
                </p>
              )}
            </div>
          </form>
        </CardBody>
      </Card>
    </div>
  )
}

function Field({
  id,
  label,
  error,
  children,
}: {
  id: string
  label: string
  error?: string
  children: ReactNode
}) {
  return (
    <div>
      <label
        htmlFor={id}
        className="mb-1.5 block font-mono text-[10px] uppercase tracking-wider text-text-secondary"
      >
        {label}
      </label>
      {children}
      {error && (
        <p className="mt-1 text-[11px] text-danger" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}
```

- [ ] **Étape 5 : routes, navigation et paramètres**

`frontend/src/app/router.tsx` : importer `import { UsersPage } from '../features/users/UsersPage'` et ajouter, après `{ path: '/settings', element: <SettingsPage /> },`, la route `{ path: '/users', element: <UsersPage /> },`.

`frontend/src/components/layout/AppSidebar.tsx` :

1. Importer `import { useSession } from '../../hooks/useSession'`.
2. Supprimer la ligne `{ label: 'Utilisateurs & accès', icon: Users },` de `soonItems`.
3. Dans `AppSidebar`, après `const { host, online, osLabel } = useHost()`, ajouter `const { canManageUsers } = useSession()`.
4. Dans la section « Paramètres », après le `NavLink` vers `/settings`, ajouter :

```tsx
            {canManageUsers && (
              <NavLink to="/users" className={navLinkClass} onClick={onClose}>
                <Users className="h-4 w-4 shrink-0 text-text-muted" aria-hidden="true" />
                <span>Utilisateurs & accès</span>
              </NavLink>
            )}
```

`frontend/src/features/settings/useRetentionForm.ts` : importer `import { useSession } from '../../hooks/useSession'`, ajouter en tête du hook `const { canManageSettings } = useSession()`, et ajouter au modèle renvoyé :

```ts
    /** Only administrators may change the retention; the server enforces it. */
    readOnly: !canManageSettings,
```

`frontend/src/features/settings/RetentionForm.tsx` : ajouter `readOnly` à la déstructuration de `useRetentionForm(settings)`, placer `<fieldset disabled={readOnly} className="space-y-3">` juste après la balise ouvrante `<form className="space-y-3" onSubmit={onSubmit} noValidate>` et `</fieldset>` juste avant `</form>`, puis ajouter juste avant `</fieldset>` :

```tsx
            {readOnly && (
              <p className="text-xs text-text-muted">Réservé aux administrateurs.</p>
            )}
```

`frontend/src/features/settings/SettingsPage.tsx` : remplacer le titre de carte `title="Compte administrateur"` par `title="Compte"` et `<p className="font-mono text-[10px] text-text-muted">Session authentifiée</p>` par :

```tsx
                      <p className="font-mono text-[10px] text-text-muted">
                        {session.roleLabel ?? 'Session authentifiée'}
                      </p>
```

et `{session.user?.username ?? 'administrateur'}` par `{session.user?.displayName ?? 'compte'}`. Remplacer dans la description de `PageHeader` « sécurité du compte administrateur local » par « sécurité du compte ».

- [ ] **Étape 6 : lancer les vérifications et constater le succès**

Commande : `npm --prefix frontend run format && npm --prefix frontend run test && npm --prefix frontend run lint && npm --prefix frontend run typecheck && npm --prefix frontend run build`
Attendu : tous les tests passent (dont `UsersPage`), lint sans avertissement, build réussi.

- [ ] **Étape 7 : état d'avancement dans la documentation**

1. `README.md`, section « État fonctionnel et limites connues » : remplacer la puce « Les modules d'administration (utilisateurs, journaux, services, alertes, sauvegardes, Cron) sont spécifiés … mais pas implémentés. » par :

```markdown
- Comptes multiples avec rôles (`viewer`, `operator`, `admin`), gérés par les administrateurs dans « Utilisateurs & accès » ; un compte créé doit changer son mot de passe initial. Sessions consultables, verrouillage, second facteur et les autres modules d'administration (journaux, services, alertes, sauvegardes, Cron) sont spécifiés dans [docs/SPEC_ADMINISTRATION.md](docs/SPEC_ADMINISTRATION.md) mais pas encore implémentés. Les applications restent gérées par Coolify.
```

2. `docs/PRODUCT_SPEC.md` § 6 : remplacer la puce « Modules d'administration (SPEC_ADMINISTRATION.md) non implémentés : leurs entrées de navigation restent marquées « à venir ». » par « Modules d'administration (SPEC_ADMINISTRATION.md) : seuls les comptes multiples et les rôles (phase 1A) sont livrés ; les autres entrées de navigation restent marquées « à venir ». » ; dans la puce « Historique », ajouter en tête « 2.2 — 23 novembre 2026 : comptes multiples, rôles et mot de passe initial à changer (phase 1A des modules d'administration). »

3. `docs/SPEC_ADMINISTRATION.md` : remplacer la ligne « Version 3.0 (brouillon) — 22 novembre 2026 — **non implémenté** » par « Version 3.0 (brouillon) — 22 novembre 2026 — **partiellement implémenté** : comptes multiples et rôles (§§ 3.1–3.2, phase 1A, 23 novembre 2026). » ; dans le tableau du § 9, scinder la ligne de la phase 1 en indiquant « 1A livrée (rôles, comptes) ; reste : TOTP, codes de secours, sessions, audit consultable, verrouillage ».

4. `docs/TESTING.md`, paragraphe « Couverture actuelle » : ajouter après « anti-énumération à la connexion), » le texte « comptes et rôles (politique des rôles, validation des comptes, protection du dernier administrateur, révocation des sessions à la désactivation, refus d'un compte désactivé, parcours HTTP complet création → mot de passe imposé → refus par rôle → désactivation → suppression sur SQLite réel, migration `003` sur une base existante), » et, côté interface, « client des comptes, schéma de compte, libellés de rôles, redirection vers le changement de mot de passe imposé et page Utilisateurs ».

- [ ] **Étape 8 : commit**

```bash
scripts/verify.sh
git add frontend/src README.md docs/PRODUCT_SPEC.md docs/SPEC_ADMINISTRATION.md docs/TESTING.md
git commit -m "feat(frontend): add the users and access page for administrators"
```

---

### Tâche 9 : vérification finale

**Fichiers :** aucun nouveau.

- [ ] **Étape 1 : vérification complète**

Commande : `scripts/verify.sh`
Attendu : code de sortie 0 (gofmt, `go test` avec et sans `-race`, `go vet`, Prettier, ESLint, TypeScript, Vitest, build).

- [ ] **Étape 2 : vérification manuelle**

Avec `scripts/dev.sh` et une base neuve (`rm backend/data/homelab-manager.db*` en développement seulement) :

1. Se connecter en `admin` ; « Utilisateurs & accès » apparaît dans la navigation et n'est plus dans « Feuille de route ».
2. Créer `alice` (Lecteur) ; elle apparaît « Mot de passe à changer ».
3. Dans une fenêtre privée, se connecter en `alice` : redirection vers « Choisissez votre mot de passe » ; après changement, retour à la connexion, puis accès en lecture ; pas d'entrée « Utilisateurs & accès » ; rétention en lecture seule ; `/users` affiche « Votre rôle ne permet pas cette action. ».
4. En `admin` : tenter de passer `admin` en Lecteur → message « Le dernier administrateur actif… » ; désactiver `alice` → sa fenêtre est renvoyée à la connexion au rafraîchissement suivant ; supprimer `alice` avec confirmation.
5. Thèmes clair et sombre, largeur mobile (tableau défilant horizontalement), navigation au clavier dans le formulaire.

- [ ] **Étape 3 : revue**

Lancer le skill `superpowers:requesting-code-review` sur la branche, corriger les retours fondés, relancer `scripts/verify.sh`.

## Auto-revue du plan

- **Couverture de la spécification (§§ 3.1–3.2)** : plusieurs comptes (tâches 2, 4, 5, 8) ; identifiant, nom affiché, rôle, état, dernière connexion et IP (tâches 1–3) ; compte initial administrateur (tâche 2, `EnsureAdmin` et migration) ; dernier administrateur protégé (tâche 4) ; création avec mot de passe initial à changer (tâches 2, 5, 7) ; suppression qui révoque les sessions et conserve l'audit (tâche 2) ; rôles et matrice (tâches 1, 5) ; « son profil » = mot de passe (route existante laissée hors du garde de rôle). Jetons d'API : phase 8, hors 1A.
- **Hors périmètre assumé** : sessions consultables, verrouillage, politique, audit consultable et des refus (1B) ; second facteur (1C) ; réinitialisation par un administrateur (Q-1) ; changement forcé de `admin`/`admin` (Q-2).
- **Cohérence des noms** : `AccountRepository` (port et adaptateur), `AccountUsecase`, `toDomainUser`, `toUserDTO`, `toSessionUserDTO`, `requirePermission`, `requirePasswordChanged`, `accountsQueryKey`, `toAccountRow`, `roleOptions` sont définis une fois et réutilisés à l'identique.
