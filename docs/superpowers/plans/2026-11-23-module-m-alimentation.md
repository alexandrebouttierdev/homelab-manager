# Module M — Redémarrer et éteindre la machine : plan d'implémentation

> **Pour les agents :** sous-skill requis : `superpowers:subagent-driven-development` (recommandé) ou `superpowers:executing-plans` pour exécuter ce plan tâche par tâche. Les étapes utilisent des cases à cocher (`- [ ]`) pour le suivi.

**Objectif :** permettre à un administrateur de redémarrer ou d'éteindre la machine depuis la page Machine, sans que le Manager tourne en root ni exécute de commande.

**Architecture :** un `PowerUsecase` vérifie l'activation, la confirmation par le nom d'hôte et l'unicité de la demande, écrit l'audit **avant** d'agir, puis demande l'action à un port `PowerController`. L'adaptateur `logind` appelle `org.freedesktop.login1.Manager.Reboot` / `PowerOff` sur le bus système ; une règle polkit fournie n'accorde ces deux actions qu'au compte `homelab-manager`. Un réglage de service `power_actions_enabled` (faux par défaut) doit aussi être activé.

**Stack :** Go, Chi v5, `github.com/godbus/dbus/v5` v5.2.4, SQLite ; React, TanStack Query, React Hook Form, Zod, Vitest.

**Prérequis :** la phase 1A est livrée ([plan](./2026-11-23-phase-1a-roles-comptes.md)) : ce plan réutilise `domain.Role.Can`, `rolePermissions`, `requirePermission`, `requirePasswordChanged`, `sessionUser`, `withTx`, `writeAudit`, les helpers de test `call`, `signIn`, `decodeBody`, `errorCode`, `newAccountsTestRouter`, et `useSession` avec ses permissions. Spécification : `docs/SPEC_ADMINISTRATION.md` § 8 bis.

## Contraintes globales

- Aucune commande exécutée (`shutdown`, `systemctl`, `reboot`) : uniquement l'API D-Bus de `systemd-logind`, avec `interactive = false`.
- Le Manager ne tourne jamais en root. polkit n'accorde que `org.freedesktop.login1.reboot`, `org.freedesktop.login1.reboot-multiple-sessions`, `org.freedesktop.login1.power-off`, `org.freedesktop.login1.power-off-multiple-sessions`, au seul utilisateur `homelab-manager`. Jamais les actions `-ignore-inhibit`.
- `power_actions_enabled` (`HOMELAB_POWER_ACTIONS_ENABLED`) vaut `false` par défaut.
- Rôle `admin` seul (D-7). Confirmation : nom d'hôte exact (espaces de bord ignorés), 253 octets au plus, vérifié par le serveur.
- Audit `host` / nom d'hôte / `reboot` ou `poweroff` / `requested` écrit avant la demande ; sans audit, aucune demande. Échec : ligne `failure` supplémentaire.
- Après une demande acceptée, toute nouvelle demande reçoit `409 ACTION_IN_PROGRESS`.
- Dépendance nouvelle : `github.com/godbus/dbus/v5` **v5.2.4** (publiée le 13 octobre 2026, plus de deux semaines). Justification dans le commit : client D-Bus Go pur, déjà dépendance de `go-systemd` prévu en phase 3 ; le paquet `login1` de `go-systemd` ignore l'erreur de `Reboot`/`PowerOff`, inutilisable ici.
- **Ne jamais tester une action réelle sur la machine de développement ni en CI** : les tests unitaires utilisent un faux contrôleur ; l'essai réel se fait sur une machine virtuelle Debian 13 jetable.
- Règles communes : code et commits en anglais, documentation et interface en français, Conventional Commits, `scripts/verify.sh` avant chaque commit, documents mis à jour dans le commit du code.

## Carte des fichiers

| Fichier | Rôle |
|---|---|
| `backend/internal/domain/power.go` (nouveau), `domain/role.go` | `PowerAction`, erreurs, confirmation ; permission `PermManagePower` |
| `backend/internal/ports/power.go` (nouveau) | `PowerController`, `AuditRecorder` |
| `backend/internal/adapters/sqlite/audit_repository.go` (nouveau) | `RecordAudit` |
| `backend/internal/adapters/logind/controller.go` (nouveau) | Appels D-Bus à logind |
| `backend/internal/usecase/power.go` (nouveau) | Règles de la demande |
| `backend/internal/delivery/http/machine.go` (nouveau), `router.go` | Routes `/api/v1/machine/power` |
| `backend/cmd/manager/config.go`, `main.go` | Réglage et composition |
| `deployments/polkit/50-homelab-manager-power.rules` (nouveau), `deployments/config/manager.yaml` | Déploiement |
| `frontend/src/lib/api/machine.ts`, `lib/power.ts`, `schemas/power.ts` (nouveaux) | Client, textes, confirmation |
| `frontend/src/features/host/usePowerActions.ts`, `PowerCard.tsx` (nouveaux), `HostPage.tsx` | Carte « Alimentation » |
| `frontend/src/lib/roles.ts`, `hooks/useSession.ts`, `lib/api/client.ts` | Permission d'affichage, messages |
| `docs/API_CONTRACTS.md`, `SECURITY.md`, `TESTING.md`, `README.md` | Documentation |

---

### Tâche 1 : domaine — actions d'alimentation et permission

**Fichiers :**
- Créer : `backend/internal/domain/power.go`
- Modifier : `backend/internal/domain/role.go`, `backend/internal/domain/role_test.go`
- Tests : `backend/internal/domain/power_test.go`

**Interfaces :**
- Produit : `type PowerAction string` ; `PowerReboot = "reboot"`, `PowerPowerOff = "poweroff"` ; `(PowerAction).Valid() bool` ; `ConfirmsHostname(confirmation, hostname string) bool` ; `MaxHostnameBytes = 253` ; erreurs `ErrInvalidPowerAction`, `ErrHostnameMismatch`, `ErrPowerActionsDisabled`, `ErrActionInProgress`, `ErrPowerActionRefused` ; `PermManagePower Permission = "machine:power"` accordée à `admin` seul.

- [ ] **Étape 1 : écrire les tests qui échouent**

`backend/internal/domain/power_test.go` :

```go
package domain

import "testing"

func TestPowerActionValid(t *testing.T) {
	for _, action := range []PowerAction{PowerReboot, PowerPowerOff} {
		if !action.Valid() {
			t.Errorf("%q should be valid", action)
		}
	}
	for _, action := range []PowerAction{"", "suspend", "halt", "Reboot"} {
		if action.Valid() {
			t.Errorf("%q should be invalid", action)
		}
	}
}

func TestConfirmsHostname(t *testing.T) {
	cases := []struct {
		confirmation, hostname string
		want                   bool
	}{
		{"homelab", "homelab", true},
		{"  homelab ", "homelab", true},
		{"Homelab", "homelab", false},
		{"homelab.local", "homelab", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := ConfirmsHostname(c.confirmation, c.hostname); got != c.want {
			t.Errorf("ConfirmsHostname(%q, %q) = %v, want %v", c.confirmation, c.hostname, got, c.want)
		}
	}
}
```

Dans `backend/internal/domain/role_test.go`, ajouter au tableau de `TestRolePolicy` :

```go
		{RoleViewer, PermManagePower, false},
		{RoleOperator, PermManagePower, false},
		{RoleAdmin, PermManagePower, true},
```

- [ ] **Étape 2 : lancer les tests et constater l'échec**

Commande : `go -C backend test ./internal/domain/ -count=1`
Attendu : échec de compilation, `undefined: PowerAction`, `undefined: PermManagePower`.

- [ ] **Étape 3 : implémentation**

`backend/internal/domain/power.go` :

```go
package domain

import (
	"errors"
	"strings"
)

// PowerAction is a request to reboot or power off the machine
// (SPEC_ADMINISTRATION.md § 8 bis).
type PowerAction string

const (
	PowerReboot   PowerAction = "reboot"
	PowerPowerOff PowerAction = "poweroff"
)

// MaxHostnameBytes bounds the typed confirmation (longest DNS host name).
const MaxHostnameBytes = 253

var (
	ErrInvalidPowerAction   = errors.New("power action must be reboot or poweroff")
	ErrHostnameMismatch     = errors.New("the confirmation does not match the host name")
	ErrPowerActionsDisabled = errors.New("power actions are disabled in the service configuration")
	ErrActionInProgress     = errors.New("an action is already in progress")
	ErrPowerActionRefused   = errors.New("the system refused the power action")
)

func (a PowerAction) Valid() bool {
	return a == PowerReboot || a == PowerPowerOff
}

// ConfirmsHostname reports whether the confirmation typed by the user names
// the machine exactly; only surrounding spaces are ignored.
func ConfirmsHostname(confirmation, hostname string) bool {
	return hostname != "" && strings.TrimSpace(confirmation) == hostname
}
```

Dans `backend/internal/domain/role.go`, ajouter au bloc `const` des permissions :

```go
	// PermManagePower covers rebooting and powering off the machine.
	PermManagePower Permission = "machine:power"
```

et remplacer la ligne `RoleAdmin` de `rolePermissions` par :

```go
	RoleAdmin:    {PermManageSettings: true, PermManageUsers: true, PermManagePower: true},
```

- [ ] **Étape 4 : lancer les tests et constater le succès**

Commande : `go -C backend test ./internal/domain/ -count=1`
Attendu : `ok`.

- [ ] **Étape 5 : commit**

```bash
scripts/verify.sh
git add backend/internal/domain
git commit -m "feat(domain): add power actions and the machine power permission"
```

---

### Tâche 2 : ports et enregistrement d'audit autonome

**Fichiers :**
- Créer : `backend/internal/ports/power.go`, `backend/internal/adapters/sqlite/audit_repository.go`
- Tests : `backend/internal/adapters/sqlite/audit_repository_test.go`

**Interfaces :**
- Consomme : `withTx`, `writeAudit` (phase 1A, `transaction.go`).
- Produit : `ports.PowerController { Request(ctx, domain.PowerAction) error }` (renvoie une erreur enveloppant `domain.ErrPowerActionRefused` quand polkit ou logind refuse) ; `ports.AuditRecorder { RecordAudit(ctx, domain.AuditEntry) error }` ; `sqlite.NewAuditRepository(client *ent.Client) *AuditRepository`.

- [ ] **Étape 1 : écrire le test qui échoue**

`backend/internal/adapters/sqlite/audit_repository_test.go` :

```go
package sqlite

import (
	"context"
	"testing"
	"time"

	"homelab-manager/backend/ent/auditlog"
	"homelab-manager/backend/internal/domain"
)

func TestAuditRepositoryRecordsAnEntry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)

	entry := domain.AuditEntry{Actor: "admin", TargetType: "host", TargetID: "homelab", Action: "reboot", RequestID: "req-9", Outcome: "requested"}
	if err := NewAuditRepository(database.Client()).RecordAudit(ctx, entry); err != nil {
		t.Fatal(err)
	}
	record, err := database.Client().AuditLog.Query().Where(auditlog.Action("reboot")).Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if record.Actor != "admin" || record.TargetType != "host" || record.TargetID != "homelab" || record.Outcome != "requested" || record.RequestID != "req-9" {
		t.Fatalf("audit row = %+v", record)
	}
}
```

- [ ] **Étape 2 : lancer le test et constater l'échec**

Commande : `go -C backend test ./internal/adapters/sqlite/ -run TestAuditRepository -count=1`
Attendu : `undefined: NewAuditRepository`.

- [ ] **Étape 3 : implémentation**

`backend/internal/ports/power.go` :

```go
package ports

import (
	"context"

	"homelab-manager/backend/internal/domain"
)

// PowerController asks the operating system to reboot or power off.
type PowerController interface {
	// Request returns once the system accepted or refused the action; a
	// refusal (polkit, inhibitor lock, shutdown already running) wraps
	// domain.ErrPowerActionRefused.
	Request(ctx context.Context, action domain.PowerAction) error
}

// AuditRecorder writes an audit entry that is not part of another change,
// such as a request whose effect happens outside the database.
type AuditRecorder interface {
	RecordAudit(ctx context.Context, entry domain.AuditEntry) error
}
```

`backend/internal/adapters/sqlite/audit_repository.go` :

```go
package sqlite

import (
	"context"

	"homelab-manager/backend/ent"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

var _ ports.AuditRecorder = (*AuditRepository)(nil)

// AuditRepository records audit entries on their own, committed before the
// caller goes on.
type AuditRepository struct {
	client *ent.Client
}

func NewAuditRepository(client *ent.Client) *AuditRepository {
	return &AuditRepository{client: client}
}

func (r *AuditRepository) RecordAudit(ctx context.Context, entry domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		return writeAudit(ctx, tx, entry)
	})
}
```

- [ ] **Étape 4 : lancer le test et constater le succès**

Commande : `go -C backend test ./internal/adapters/sqlite/ -count=1`
Attendu : `ok`.

- [ ] **Étape 5 : commit**

```bash
scripts/verify.sh
git add backend/internal/ports/power.go backend/internal/adapters/sqlite/audit_repository.go backend/internal/adapters/sqlite/audit_repository_test.go
git commit -m "feat(storage): record standalone audit entries"
```

---

### Tâche 3 : adaptateur logind

**Fichiers :**
- Créer : `backend/internal/adapters/logind/controller.go`
- Modifier : `backend/go.mod`, `backend/go.sum`
- Tests : `backend/internal/adapters/logind/controller_test.go`

**Interfaces :**
- Consomme : `ports.PowerController`, `domain.PowerAction`, erreurs du domaine.
- Produit : `logind.New() *Controller`, qui implémente `ports.PowerController`.

- [ ] **Étape 1 : ajouter la dépendance**

Commande : `go -C backend get github.com/godbus/dbus/v5@v5.2.4`
Attendu : `go.mod` contient `github.com/godbus/dbus/v5 v5.2.4` en dépendance directe après `go mod tidy` (étape 5).

- [ ] **Étape 2 : écrire les tests qui échouent**

`backend/internal/adapters/logind/controller_test.go` :

```go
package logind

import (
	"context"
	"errors"
	"testing"

	"homelab-manager/backend/internal/domain"

	"github.com/godbus/dbus/v5"
)

func TestTranslateClassifiesRefusals(t *testing.T) {
	for _, name := range []string{
		"org.freedesktop.DBus.Error.AccessDenied",
		"org.freedesktop.DBus.Error.InteractiveAuthorizationRequired",
		"org.freedesktop.login1.OperationInProgress",
	} {
		if err := translate(dbus.Error{Name: name}); !errors.Is(err, domain.ErrPowerActionRefused) {
			t.Errorf("%s = %v, want ErrPowerActionRefused", name, err)
		}
	}
	other := translate(dbus.Error{Name: "org.freedesktop.DBus.Error.ServiceUnknown"})
	if other == nil || errors.Is(other, domain.ErrPowerActionRefused) {
		t.Fatalf("an unavailable logind = %v, want a plain failure", other)
	}
	if err := translate(nil); err != nil {
		t.Fatalf("translate(nil) = %v", err)
	}
}

// An unknown action must be refused before any connection to the system
// bus: this test never reaches logind.
func TestRequestRejectsUnknownActionsWithoutCallingLogind(t *testing.T) {
	if err := New().Request(context.Background(), "suspend"); !errors.Is(err, domain.ErrInvalidPowerAction) {
		t.Fatalf("Request(suspend) = %v, want ErrInvalidPowerAction", err)
	}
}
```

- [ ] **Étape 3 : lancer les tests et constater l'échec**

Commande : `go -C backend test ./internal/adapters/logind/ -count=1`
Attendu : `undefined: translate`, `undefined: New`.

- [ ] **Étape 4 : implémentation**

`backend/internal/adapters/logind/controller.go` :

```go
// Package logind asks systemd-logind, over the system D-Bus, to reboot or
// power off the machine. polkit decides whether the Manager account may do
// so; the Manager never runs a command and never runs as root.
package logind

import (
	"context"
	"errors"
	"fmt"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/godbus/dbus/v5"
)

const (
	busName    = "org.freedesktop.login1"
	objectPath = dbus.ObjectPath("/org/freedesktop/login1")
	// callTimeout bounds the call; logind answers as soon as the shutdown
	// job is queued, long before the machine stops.
	callTimeout = 10 * time.Second
)

var methods = map[domain.PowerAction]string{
	domain.PowerReboot:   "org.freedesktop.login1.Manager.Reboot",
	domain.PowerPowerOff: "org.freedesktop.login1.Manager.PowerOff",
}

// refusals are the D-Bus errors by which polkit or logind decline: no
// matching polkit rule, an inhibitor lock (the -ignore-inhibit actions are
// never granted), or a shutdown already running.
var refusals = map[string]bool{
	"org.freedesktop.DBus.Error.AccessDenied":                     true,
	"org.freedesktop.DBus.Error.InteractiveAuthorizationRequired": true,
	"org.freedesktop.login1.OperationInProgress":                  true,
}

var _ ports.PowerController = (*Controller)(nil)

type Controller struct{}

func New() *Controller { return &Controller{} }

func (c *Controller) Request(ctx context.Context, action domain.PowerAction) error {
	method, ok := methods[action]
	if !ok {
		return domain.ErrInvalidPowerAction
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("connect to the system bus: %w", err)
	}
	defer func() { _ = conn.Close() }()
	// interactive = false: polkit answers from its rules, never by prompting.
	call := conn.Object(busName, objectPath).CallWithContext(ctx, method, 0, false)
	return translate(call.Err)
}

func translate(err error) error {
	if err == nil {
		return nil
	}
	var dbusErr dbus.Error
	if errors.As(err, &dbusErr) && refusals[dbusErr.Name] {
		return fmt.Errorf("%w: %s", domain.ErrPowerActionRefused, dbusErr.Name)
	}
	return fmt.Errorf("call logind: %w", err)
}
```

- [ ] **Étape 5 : lancer les tests et constater le succès**

Commande : `go -C backend mod tidy && go -C backend test ./internal/adapters/logind/ -count=1 && go -C backend vet ./...`
Attendu : `ok`, `go vet` silencieux.

- [ ] **Étape 6 : commit**

Le corps du message justifie la dépendance (règle `CODE_RULES.md` § 6).

```bash
scripts/verify.sh
git add backend/go.mod backend/go.sum backend/internal/adapters/logind
git commit -m "feat(power): ask systemd-logind to reboot or power off over D-Bus" -m "Adds github.com/godbus/dbus/v5 v5.2.4 (released 2026-10-13): pure-Go D-Bus client, already a dependency of go-systemd planned for the services module. go-systemd's login1 package drops the error of Reboot/PowerOff, so it cannot report a polkit refusal. No command is executed."
```

---

### Tâche 4 : usecase des actions d'alimentation

**Fichiers :**
- Créer : `backend/internal/usecase/power.go`
- Tests : `backend/internal/usecase/power_test.go`

**Interfaces :**
- Consomme : `ports.PowerController`, `ports.AuditRecorder`, domaine (tâche 1).
- Produit : `usecase.NewPowerUsecase(controller ports.PowerController, audit ports.AuditRecorder, hostname func() (string, error), enabled bool) *PowerUsecase` ; `Enabled() bool` ; `Request(ctx, actor domain.User, action domain.PowerAction, confirmation, requestID string) error`.

- [ ] **Étape 1 : écrire les tests qui échouent**

`backend/internal/usecase/power_test.go` :

```go
package usecase

import (
	"context"
	"errors"
	"testing"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

type fakeAudit struct {
	entries []domain.AuditEntry
	err     error
}

func (f *fakeAudit) RecordAudit(_ context.Context, entry domain.AuditEntry) error {
	if f.err != nil {
		return f.err
	}
	f.entries = append(f.entries, entry)
	return nil
}

// fakePower records requests and how many audit entries existed then.
type fakePower struct {
	audit        *fakeAudit
	requested    []domain.PowerAction
	auditedFirst []int
	err          error
}

func (f *fakePower) Request(_ context.Context, action domain.PowerAction) error {
	f.requested = append(f.requested, action)
	f.auditedFirst = append(f.auditedFirst, len(f.audit.entries))
	return f.err
}

func newPowerFixture(enabled bool) (*PowerUsecase, *fakePower, *fakeAudit) {
	audit := &fakeAudit{}
	power := &fakePower{audit: audit}
	hostname := func() (string, error) { return "homelab", nil }
	return NewPowerUsecase(power, audit, hostname, enabled), power, audit
}

var powerAdmin = domain.User{ID: uuid.New(), Username: "admin", Role: domain.RoleAdmin}

func TestPowerRequestIsAuditedBeforeTheAction(t *testing.T) {
	usecase, power, audit := newPowerFixture(true)
	if err := usecase.Request(context.Background(), powerAdmin, domain.PowerReboot, " homelab ", "req-1"); err != nil {
		t.Fatal(err)
	}
	if len(power.requested) != 1 || power.requested[0] != domain.PowerReboot {
		t.Fatalf("requested = %v, want [reboot]", power.requested)
	}
	if power.auditedFirst[0] != 1 {
		t.Fatal("the audit entry must be written before asking the system")
	}
	want := domain.AuditEntry{Actor: "admin", TargetType: "host", TargetID: "homelab", Action: "reboot", RequestID: "req-1", Outcome: "requested"}
	if audit.entries[0] != want {
		t.Fatalf("audit = %+v, want %+v", audit.entries[0], want)
	}
}

func TestPowerRequestIsRefusedBeforeAnyEffect(t *testing.T) {
	cases := []struct {
		name         string
		enabled      bool
		action       domain.PowerAction
		confirmation string
		want         error
	}{
		{"disabled", false, domain.PowerReboot, "homelab", domain.ErrPowerActionsDisabled},
		{"wrong host", true, domain.PowerPowerOff, "other", domain.ErrHostnameMismatch},
		{"empty confirmation", true, domain.PowerPowerOff, "", domain.ErrHostnameMismatch},
		{"unknown action", true, "suspend", "homelab", domain.ErrInvalidPowerAction},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			usecase, power, audit := newPowerFixture(c.enabled)
			err := usecase.Request(context.Background(), powerAdmin, c.action, c.confirmation, "")
			if !errors.Is(err, c.want) {
				t.Fatalf("Request error = %v, want %v", err, c.want)
			}
			if len(power.requested) != 0 || len(audit.entries) != 0 {
				t.Fatal("a refused request must neither be audited nor reach the system")
			}
		})
	}
}

func TestPowerRequestWithoutAuditDoesNothing(t *testing.T) {
	usecase, power, audit := newPowerFixture(true)
	audit.err = errors.New("disk full")
	if err := usecase.Request(context.Background(), powerAdmin, domain.PowerReboot, "homelab", ""); err == nil {
		t.Fatal("expected an error when the audit cannot be written")
	}
	if len(power.requested) != 0 {
		t.Fatal("no trace, no action")
	}
}

func TestARefusedPowerRequestIsAuditedAndCanBeRetried(t *testing.T) {
	usecase, power, audit := newPowerFixture(true)
	power.err = domain.ErrPowerActionRefused
	if err := usecase.Request(context.Background(), powerAdmin, domain.PowerPowerOff, "homelab", ""); !errors.Is(err, domain.ErrPowerActionRefused) {
		t.Fatalf("Request error = %v, want ErrPowerActionRefused", err)
	}
	if len(audit.entries) != 2 || audit.entries[1].Outcome != "failure" {
		t.Fatalf("audit = %+v, want requested then failure", audit.entries)
	}
	power.err = nil
	if err := usecase.Request(context.Background(), powerAdmin, domain.PowerPowerOff, "homelab", ""); err != nil {
		t.Fatalf("a refused request must not block the next one: %v", err)
	}
}

func TestOnlyOnePowerRequestIsAccepted(t *testing.T) {
	usecase, power, _ := newPowerFixture(true)
	if err := usecase.Request(context.Background(), powerAdmin, domain.PowerReboot, "homelab", ""); err != nil {
		t.Fatal(err)
	}
	if err := usecase.Request(context.Background(), powerAdmin, domain.PowerPowerOff, "homelab", ""); !errors.Is(err, domain.ErrActionInProgress) {
		t.Fatalf("second request = %v, want ErrActionInProgress", err)
	}
	if len(power.requested) != 1 {
		t.Fatalf("requested = %v, want a single request", power.requested)
	}
}
```

- [ ] **Étape 2 : lancer les tests et constater l'échec**

Commande : `go -C backend test ./internal/usecase/ -run Power -count=1`
Attendu : `undefined: NewPowerUsecase`.

- [ ] **Étape 3 : implémentation**

`backend/internal/usecase/power.go` :

```go
package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

const auditTargetHost = "host"

// PowerUsecase reboots or powers off the machine on an administrator's
// request (SPEC_ADMINISTRATION.md § 8 bis). Callers must hold
// domain.PermManagePower; the HTTP layer enforces it.
type PowerUsecase struct {
	controller ports.PowerController
	audit      ports.AuditRecorder
	hostname   func() (string, error)
	enabled    bool

	mu sync.Mutex
	// accepted is set once the system accepted a request: the machine is
	// going down and any further request is refused.
	accepted bool
}

func NewPowerUsecase(controller ports.PowerController, audit ports.AuditRecorder, hostname func() (string, error), enabled bool) *PowerUsecase {
	return &PowerUsecase{controller: controller, audit: audit, hostname: hostname, enabled: enabled}
}

// Enabled reports whether the service configuration allows power actions.
func (u *PowerUsecase) Enabled() bool { return u.enabled }

func (u *PowerUsecase) Request(ctx context.Context, actor domain.User, action domain.PowerAction, confirmation, requestID string) error {
	if !action.Valid() {
		return domain.ErrInvalidPowerAction
	}
	if !u.enabled {
		return domain.ErrPowerActionsDisabled
	}
	hostname, err := u.hostname()
	if err != nil {
		return fmt.Errorf("read host name: %w", err)
	}
	if !domain.ConfirmsHostname(confirmation, hostname) {
		return domain.ErrHostnameMismatch
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	if u.accepted {
		return domain.ErrActionInProgress
	}
	entry := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetHost, TargetID: hostname,
		Action: string(action), RequestID: requestID, Outcome: "requested",
	}
	// No trace, no action: the machine may stop before anything else is
	// written, so the entry is committed first.
	if err := u.audit.RecordAudit(ctx, entry); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}
	if err := u.controller.Request(ctx, action); err != nil {
		entry.Outcome = "failure"
		if auditErr := u.audit.RecordAudit(ctx, entry); auditErr != nil {
			return errors.Join(err, fmt.Errorf("record audit: %w", auditErr))
		}
		return err
	}
	u.accepted = true
	return nil
}
```

- [ ] **Étape 4 : lancer les tests et constater le succès**

Commande : `go -C backend test ./internal/usecase/ -count=1 && go -C backend test -race ./internal/usecase/ -count=1`
Attendu : `ok` deux fois.

- [ ] **Étape 5 : commit**

```bash
scripts/verify.sh
git add backend/internal/usecase/power.go backend/internal/usecase/power_test.go
git commit -m "feat(power): confirm, audit and serialise power requests"
```

---

### Tâche 5 : configuration, routes HTTP, déploiement et contrat

**Fichiers :**
- Créer : `backend/internal/delivery/http/machine.go`, `backend/internal/delivery/http/machine_test.go`, `deployments/polkit/50-homelab-manager-power.rules`
- Modifier : `backend/cmd/manager/config.go`, `backend/cmd/manager/config_test.go`, `backend/cmd/manager/main.go`, `backend/internal/delivery/http/router.go`, `backend/internal/delivery/http/router_test.go`, `backend/internal/delivery/http/users_test.go`, `deployments/config/manager.yaml`, `docs/API_CONTRACTS.md`, `docs/SECURITY.md`, `README.md`

**Interfaces :**
- Consomme : `usecase.PowerUsecase` (tâche 4), `logind.New` (tâche 3), `sqlite.NewAuditRepository` (tâche 2), `requirePermission` (1A).
- Produit :
  - `Config.PowerActionsEnabled bool` (clé `power_actions_enabled`, défaut `false`).
  - `NewRouter(logger, health, auth, monitoring, accounts, power *usecase.PowerUsecase, sessionSecure, webRoot)`.
  - `GET /api/v1/machine/power` (admin) → `200 { "enabled": bool, "actions": ["reboot", "poweroff"] }`.
  - `POST /api/v1/machine/power/{action}` (admin) avec `{ "confirmHostname": "..." }` → `202 { "requested": true, "action": "reboot" }`. Codes : `400 VALIDATION_ERROR`, `400 HOSTNAME_MISMATCH`, `409 POWER_ACTIONS_DISABLED`, `409 ACTION_IN_PROGRESS`, `503 POWER_ACTION_REFUSED`, `500 POWER_ACTION_FAILED`.

- [ ] **Étape 1 : écrire les tests qui échouent**

Dans `backend/cmd/manager/config_test.go`, ajouter :

```go
func TestPowerActionsAreDisabledByDefault(t *testing.T) {
	t.Chdir(t.TempDir()) // no manager.yaml from the repository
	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.PowerActionsEnabled {
		t.Fatal("power actions must be disabled unless explicitly enabled")
	}
	t.Setenv("HOMELAB_POWER_ACTIONS_ENABLED", "true")
	if config, err = LoadConfig(); err != nil || !config.PowerActionsEnabled {
		t.Fatalf("LoadConfig = (%v, %v), want power actions enabled", config.PowerActionsEnabled, err)
	}
}
```

Dans `backend/internal/delivery/http/users_test.go` (phase 1A), remplacer la fonction `newAccountsTestRouter` par un constructeur commun qui accepte un contrôleur d'alimentation (`nil` = actions désactivées) :

```go
// newTestRouter builds the HTTP stack on a fresh SQLite file with an
// "admin" account. A nil controller leaves the power actions disabled.
func newTestRouter(t *testing.T, controller ports.PowerController) (http.Handler, *sqlite.Database) {
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
	hostname := func() (string, error) { return "homelab", nil }
	power := usecase.NewPowerUsecase(controller, sqlite.NewAuditRepository(database.Client()), hostname, controller != nil)
	return NewRouter(zap.NewNop(), usecase.NewHealthUsecase(time.Now()), auth, nil, accounts, power, false, ""), database
}
```

ajouter l'import `"homelab-manager/backend/internal/ports"`, et dans `TestAccountAdministrationEndToEnd` remplacer `router := newAccountsTestRouter(t)` par `router, _ := newTestRouter(t, nil)`.

`backend/internal/delivery/http/machine_test.go` :

```go
package httpdelivery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"homelab-manager/backend/ent/auditlog"
	"homelab-manager/backend/internal/domain"

	"github.com/go-chi/chi/v5"
)

// recordingPower stands for logind: tests never reboot the machine.
type recordingPower struct {
	requested []domain.PowerAction
	err       error
}

func (r *recordingPower) Request(_ context.Context, action domain.PowerAction) error {
	r.requested = append(r.requested, action)
	return r.err
}

func TestPowerActionValidatesBeforeAnyChange(t *testing.T) {
	// A nil usecase proves validation happens before any change.
	router := chi.NewRouter()
	router.Post("/api/v1/machine/power/{action}", powerActionHandler(nil))
	cases := []struct{ action, body string }{
		{"suspend", `{"confirmHostname":"homelab"}`},
		{"Reboot", `{"confirmHostname":"homelab"}`},
		{"reboot", `{}`},
		{"reboot", `{"confirmHostname":""}`},
		{"reboot", `{"confirmHostname":"` + strings.Repeat("a", 254) + `"}`},
		{"reboot", `{"confirmHostname":"homelab","force":true}`},
	}
	for _, c := range cases {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/machine/power/"+c.action, strings.NewReader(c.body)))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("POST %s %s returned %d, want 400", c.action, c.body, recorder.Code)
		}
	}
}

func TestPowerActionEndToEnd(t *testing.T) {
	controller := &recordingPower{}
	router, database := newTestRouter(t, controller)
	admin := signIn(t, router, "admin", "admin-password-1")

	var settings struct {
		Enabled bool     `json:"enabled"`
		Actions []string `json:"actions"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/machine/power", "", admin), &settings)
	if !settings.Enabled || len(settings.Actions) != 2 {
		t.Fatalf("power settings = %+v", settings)
	}

	if response := call(router, http.MethodPost, "/api/v1/machine/power/reboot", `{"confirmHostname":"other"}`, admin); response.Code != http.StatusBadRequest || errorCode(t, response) != "HOSTNAME_MISMATCH" {
		t.Fatalf("wrong host = %d %s, want 400 HOSTNAME_MISMATCH", response.Code, response.Body)
	}
	controller.err = domain.ErrPowerActionRefused
	if response := call(router, http.MethodPost, "/api/v1/machine/power/reboot", `{"confirmHostname":"homelab"}`, admin); response.Code != http.StatusServiceUnavailable || errorCode(t, response) != "POWER_ACTION_REFUSED" {
		t.Fatalf("polkit refusal = %d %s, want 503 POWER_ACTION_REFUSED", response.Code, response.Body)
	}
	controller.err = nil
	if response := call(router, http.MethodPost, "/api/v1/machine/power/reboot", `{"confirmHostname":"homelab"}`, admin); response.Code != http.StatusAccepted {
		t.Fatalf("reboot = %d %s, want 202", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/machine/power/poweroff", `{"confirmHostname":"homelab"}`, admin); response.Code != http.StatusConflict || errorCode(t, response) != "ACTION_IN_PROGRESS" {
		t.Fatalf("second request = %d %s, want 409 ACTION_IN_PROGRESS", response.Code, response.Body)
	}
	if len(controller.requested) != 2 {
		t.Fatalf("requested = %v, want the refused then the accepted reboot", controller.requested)
	}
	// refused: requested + failure; accepted: requested.
	count, err := database.Client().AuditLog.Query().Where(auditlog.TargetType("host"), auditlog.Action("reboot")).Count(context.Background())
	if err != nil || count != 3 {
		t.Fatalf("audit rows = (%d, %v), want 3", count, err)
	}
}

func TestPowerActionsDisabledByConfiguration(t *testing.T) {
	router, _ := newTestRouter(t, nil)
	admin := signIn(t, router, "admin", "admin-password-1")
	if response := call(router, http.MethodPost, "/api/v1/machine/power/poweroff", `{"confirmHostname":"homelab"}`, admin); response.Code != http.StatusConflict || errorCode(t, response) != "POWER_ACTIONS_DISABLED" {
		t.Fatalf("disabled = %d %s, want 409 POWER_ACTIONS_DISABLED", response.Code, response.Body)
	}
}
```

Le refus par rôle est déjà couvert par `TestRolePolicy` (permission) et `TestRequirePermissionAppliesTheRolePolicy` (middleware).

- [ ] **Étape 2 : lancer les tests et constater l'échec**

Commande : `go -C backend test ./cmd/manager/ ./internal/delivery/http/ -count=1`
Attendu : `config.PowerActionsEnabled undefined`, `undefined: powerActionHandler`, `too many arguments in call to NewRouter`.

- [ ] **Étape 3 : configuration**

Dans `backend/cmd/manager/config.go` :
1. Ajouter le champ `PowerActionsEnabled bool` à la fin de `Config`.
2. Après `v.SetDefault("metrics_retention_days", 7)`, ajouter :

```go
	// Rebooting and powering off also need the polkit rule of
	// deployments/polkit; both are off by default.
	v.SetDefault("power_actions_enabled", false)
```

3. Dans le littéral `config := Config{...}`, ajouter `PowerActionsEnabled: v.GetBool("power_actions_enabled"),`.

Dans `deployments/config/manager.yaml`, ajouter à la fin :

```yaml
# Redémarrer et éteindre la machine depuis l'interface : exige aussi la règle
# polkit deployments/polkit/50-homelab-manager-power.rules (voir README).
power_actions_enabled: false
```

- [ ] **Étape 4 : handlers et routes**

`backend/internal/delivery/http/machine.go` :

```go
package httpdelivery

import (
	"errors"
	"net/http"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var powerActions = []domain.PowerAction{domain.PowerReboot, domain.PowerPowerOff}

type powerRequest struct {
	ConfirmHostname string `json:"confirmHostname"`
}

func powerSettingsHandler(power *usecase.PowerUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": power.Enabled(), "actions": powerActions})
	}
}

func powerActionHandler(power *usecase.PowerUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		action := domain.PowerAction(chi.URLParam(r, "action"))
		if !action.Valid() {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "action must be reboot or poweroff.")
			return
		}
		var request powerRequest
		if err := decodeJSON(r, &request); err != nil || request.ConfirmHostname == "" || len(request.ConfirmHostname) > domain.MaxHostnameBytes {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "confirmHostname is required.")
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		err := power.Request(r.Context(), actor, action, request.ConfirmHostname, middleware.GetReqID(r.Context()))
		switch {
		case err == nil:
			writeJSON(w, http.StatusAccepted, map[string]any{"requested": true, "action": action})
		case errors.Is(err, domain.ErrHostnameMismatch):
			writeError(w, r, http.StatusBadRequest, "HOSTNAME_MISMATCH", "The confirmation does not match the host name.")
		case errors.Is(err, domain.ErrPowerActionsDisabled):
			writeError(w, r, http.StatusConflict, "POWER_ACTIONS_DISABLED", "Power actions are disabled in the service configuration.")
		case errors.Is(err, domain.ErrActionInProgress):
			writeError(w, r, http.StatusConflict, "ACTION_IN_PROGRESS", "A power action is already in progress.")
		case errors.Is(err, domain.ErrPowerActionRefused):
			writeError(w, r, http.StatusServiceUnavailable, "POWER_ACTION_REFUSED", "The system refused the power action.")
		default:
			writeError(w, r, http.StatusInternalServerError, "POWER_ACTION_FAILED", "The power action could not be requested.")
		}
	}
}
```

Dans `backend/internal/delivery/http/router.go` :
1. Signature : ajouter `power *usecase.PowerUsecase` après `accounts *usecase.AccountUsecase`.
2. Dans le groupe `active`, après le bloc `active.Route("/users", ...)`, ajouter :

```go
				active.Route("/machine/power", func(machine chi.Router) {
					machine.Use(requirePermission(domain.PermManagePower))
					machine.Get("/", powerSettingsHandler(power))
					machine.Post("/{action}", powerActionHandler(power))
				})
```

Mettre à jour les appels de test :

```bash
sed -i 's/nil, nil, nil, false, /nil, nil, nil, nil, false, /' backend/internal/delivery/http/router_test.go
```

Dans `backend/cmd/manager/main.go` :
1. Importer `logindadapter "homelab-manager/backend/internal/adapters/logind"`.
2. Après `var accountUsecase *usecase.AccountUsecase`, ajouter `var powerUsecase *usecase.PowerUsecase`.
3. Après la création de `accountUsecase`, ajouter :

```go
		powerUsecase = usecase.NewPowerUsecase(logindadapter.New(), sqlite.NewAuditRepository(client), os.Hostname, config.PowerActionsEnabled)
		if config.PowerActionsEnabled {
			logger.Info("Reboot and power off are enabled; they also require the polkit rule")
		}
```

4. Passer `powerUsecase` à `NewRouter` après `accountUsecase`.

- [ ] **Étape 5 : règle polkit**

`deployments/polkit/50-homelab-manager-power.rules` :

```js
// Homelab Manager: lets the homelab-manager account reboot or power off the
// machine through systemd-logind, and nothing else (SPEC_ADMINISTRATION.md
// § 8 bis). The *-ignore-inhibit actions are deliberately not granted:
// inhibitor locks, such as a running backup, keep blocking the request.
//
// Install as root:root 0644 in /etc/polkit-1/rules.d/ and set
// power_actions_enabled: true in /etc/homelab-manager/manager.yaml.
polkit.addRule(function (action, subject) {
    var allowed = [
        "org.freedesktop.login1.reboot",
        "org.freedesktop.login1.reboot-multiple-sessions",
        "org.freedesktop.login1.power-off",
        "org.freedesktop.login1.power-off-multiple-sessions"
    ];
    if (subject.user === "homelab-manager" && allowed.indexOf(action.id) >= 0) {
        return polkit.Result.YES;
    }
});
```

- [ ] **Étape 6 : lancer les tests et constater le succès**

Commande : `go -C backend test ./... -count=1 && go -C backend test -race ./internal/delivery/http/ ./internal/usecase/ -count=1`
Attendu : tous les paquets `ok`, y compris `TestPowerActionEndToEnd` et `TestAccountAdministrationEndToEnd`.

- [ ] **Étape 7 : documentation**

`docs/API_CONTRACTS.md` :
1. Tableau du § 2, ajouter :

```markdown
| `GET /api/v1/machine/power` | admin | Actions d'alimentation disponibles (§ 4.5) |
| `POST /api/v1/machine/power/{action}` | admin | Redémarre ou éteint la machine (§ 4.5) |
```

et compléter la phrase sur les écritures : « … la gestion des comptes et les demandes d'alimentation de la machine. ». Dans « Paramètres de chemin » : « `action` : `reboot` ou `poweroff`. Sinon `400 VALIDATION_ERROR`. »

2. Supprimer le § 9.2 « Alimentation de la machine (module M) » (contrat annoncé avant implémentation) et ajouter après le § 4.4 :

```markdown
### 4.5 Alimentation de la machine

Réservé au rôle `admin` ; voir SPEC_ADMINISTRATION.md § 8 bis.

`GET /api/v1/machine/power` : `200 { "enabled": false, "actions": ["reboot", "poweroff"] }`. `enabled` reflète le réglage de service `power_actions_enabled` ; la règle polkit, elle, n'est connue qu'au moment de la demande.

`POST /api/v1/machine/power/{action}` avec `{ "confirmHostname": "homelab" }` (1 à 253 octets, espaces de bord ignorés, comparé exactement au nom d'hôte de la machine) :

- Succès `202 { "requested": true, "action": "reboot" }` : `systemd-logind` a accepté la demande ; le Manager s'arrête avec les autres services peu après. La réponse peut ne pas parvenir si l'arrêt est très rapide.
- `400 HOSTNAME_MISMATCH` : confirmation différente du nom d'hôte. `409 POWER_ACTIONS_DISABLED` : réglage désactivé. `409 ACTION_IN_PROGRESS` : une demande a déjà été acceptée. `503 POWER_ACTION_REFUSED` : polkit ou logind refuse (règle absente, verrou d'inhibition, arrêt déjà en cours). `500 POWER_ACTION_FAILED` : bus système ou logind injoignable.
- Une ligne d'audit (`host`, nom d'hôte, action, `requested`) est écrite avant la demande ; un refus ajoute `failure`.
```

3. Au § 8, ajouter :

```markdown
| `HOSTNAME_MISMATCH` | 400 | Confirmation différente du nom de la machine |
| `POWER_ACTIONS_DISABLED` | 409 | Redémarrage et extinction désactivés par la configuration |
| `ACTION_IN_PROGRESS` | 409 | Une action est déjà en cours |
| `POWER_ACTION_REFUSED` | 503 | polkit ou logind refuse l'action |
| `POWER_ACTION_FAILED` | 500 | Demande d'alimentation impossible à transmettre |
```

`docs/SECURITY.md`, section « Système et déploiement », ajouter :

```markdown
- Redémarrage et extinction (désactivés par défaut) : le Manager demande l'action à `systemd-logind` par D-Bus, sans exécuter de commande ni tourner en root. La règle polkit fournie n'accorde que `reboot` et `power-off` (et leurs variantes `-multiple-sessions`) au compte `homelab-manager`, jamais les variantes `-ignore-inhibit`. Risque résiduel : un compte administrateur compromis peut interrompre le service de la machine ; il est limité par le rôle, la confirmation du nom d'hôte, l'audit écrit avant l'action et la double activation (réglage + règle polkit). Après une extinction, rien ne rallume la machine à distance.
```

`README.md` :
1. Dans « Configuration », ajouter : « `HOMELAB_POWER_ACTIONS_ENABLED` : autorise Redémarrer / Éteindre depuis la page Machine (défaut `false`) ; exige aussi la règle polkit ci-dessous. »
2. Dans « Déploiement natif Debian », ajouter une sous-section :

```markdown
### Redémarrer et éteindre depuis l'interface (optionnel)

Désactivé par défaut. Pour l'activer :

```bash
sudo apt install polkitd
sudo install -o root -g root -m 0644 deployments/polkit/50-homelab-manager-power.rules /etc/polkit-1/rules.d/
```

puis mettre `power_actions_enabled: true` dans `/etc/homelab-manager/manager.yaml` et redémarrer `homelab-manager.service`. La règle n'autorise que le redémarrage et l'extinction, par le seul compte `homelab-manager`, et respecte les verrous d'inhibition. Seuls les administrateurs voient la carte « Alimentation » de la page Machine ; la confirmation demande de saisir le nom de la machine. Après une extinction, il faut un accès physique (ou Wake-on-LAN configuré hors de Homelab Manager) pour rallumer la machine.
```

- [ ] **Étape 8 : commit**

```bash
gofmt -l backend   # doit être vide
scripts/verify.sh
git add backend deployments docs/API_CONTRACTS.md docs/SECURITY.md README.md
git commit -m "feat(api): add admin-only reboot and power off routes"
```

---

### Tâche 6 : interface — carte « Alimentation » de la page Machine

**Fichiers :**
- Créer : `frontend/src/lib/api/machine.ts`, `frontend/src/lib/power.ts`, `frontend/src/schemas/power.ts`, `frontend/src/features/host/usePowerActions.ts`, `frontend/src/features/host/PowerCard.tsx`
- Modifier : `frontend/src/lib/api/client.ts`, `frontend/src/lib/roles.ts`, `frontend/src/lib/roles.test.ts`, `frontend/src/hooks/useSession.ts`, `frontend/src/features/host/HostPage.tsx`, et les objets de session simulés des tests de la phase 1A (`RequireAuth.test.tsx`, `ForcedPasswordChangePage.test.tsx`, `UsersPage.test.tsx`) qui reçoivent `canManagePower`
- Tests : `frontend/src/lib/api/machine.test.ts`, `frontend/src/schemas/power.test.ts`, `frontend/src/features/host/PowerCard.test.tsx`

**Interfaces :**
- Consomme : contrat de la tâche 5 ; `useSession` (1A).
- Produit : `PowerAction`, `getPowerSettings()`, `requestPowerAction(action, confirmHostname)` ; `powerActionList`, `powerActionTexts` ; `powerConfirmationSchema(hostname)` ; `canManagePower(role?)` ; `useSession().canManagePower`.

- [ ] **Étape 1 : écrire les tests qui échouent**

`frontend/src/lib/api/machine.test.ts` :

```ts
import { afterEach, describe, expect, it, vi } from 'vitest'
import { getPowerSettings, requestPowerAction } from './machine'

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('machine power API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('reads whether power actions are enabled', async () => {
    mockFetch({ enabled: false, actions: ['reboot', 'poweroff'] })
    await expect(getPowerSettings()).resolves.toEqual({
      enabled: false,
      actions: ['reboot', 'poweroff'],
    })
  })

  it('sends the typed host name with the action', async () => {
    const fetchMock = mockFetch({ requested: true, action: 'reboot' }, 202)
    await requestPowerAction('reboot', 'homelab')
    const [path, init] = fetchMock.mock.calls[0]
    expect(path).toBe('/api/v1/machine/power/reboot')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toEqual({ confirmHostname: 'homelab' })
  })

  it('explains a polkit refusal', async () => {
    mockFetch({ error: { code: 'POWER_ACTION_REFUSED', message: 'refused' } }, 503)
    await expect(requestPowerAction('poweroff', 'homelab')).rejects.toThrow(/polkit/)
  })
})
```

`frontend/src/schemas/power.test.ts` :

```ts
import { describe, expect, it } from 'vitest'
import { powerConfirmationSchema } from './power'

describe('powerConfirmationSchema', () => {
  const schema = powerConfirmationSchema('homelab')

  it('accepts the exact host name, ignoring surrounding spaces', () => {
    expect(schema.safeParse({ confirmHostname: 'homelab' }).success).toBe(true)
    expect(schema.safeParse({ confirmHostname: ' homelab ' }).success).toBe(true)
  })

  it.each(['', 'Homelab', 'homelab.local', 'other'])('rejects %j', (value) => {
    expect(schema.safeParse({ confirmHostname: value }).success).toBe(false)
  })

  it('never accepts anything while the host name is unknown', () => {
    expect(powerConfirmationSchema('').safeParse({ confirmHostname: '' }).success).toBe(false)
  })
})
```

Dans `frontend/src/lib/roles.test.ts`, ajouter `canManagePower` à l'import et au test « mirrors the server policy » :

```ts
    expect(canManagePower('admin')).toBe(true)
    expect(canManagePower('operator')).toBe(false)
```

`frontend/src/features/host/PowerCard.test.tsx` :

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { getPowerSettings, requestPowerAction } from '../../lib/api/machine'
import { PowerCard } from './PowerCard'

vi.mock('../../lib/api/machine', () => ({
  getPowerSettings: vi.fn(),
  requestPowerAction: vi.fn(),
}))

vi.mock('../../hooks/useSession', () => ({
  useSession: () => ({ canManagePower: true }),
}))

function renderCard() {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <PowerCard hostname="homelab" />
    </QueryClientProvider>,
  )
}

describe('PowerCard', () => {
  it('reboots only after the host name is typed', async () => {
    vi.mocked(getPowerSettings).mockResolvedValue({ enabled: true, actions: ['reboot', 'poweroff'] })
    vi.mocked(requestPowerAction).mockResolvedValue({ requested: true, action: 'reboot' })
    const user = userEvent.setup()
    renderCard()

    await user.click(await screen.findByRole('button', { name: 'Redémarrer' }))
    const input = screen.getByLabelText('Saisissez « homelab » pour confirmer')
    await user.type(input, 'other')
    await user.click(screen.getByRole('button', { name: 'Confirmer : Redémarrer' }))
    expect(await screen.findByText('Saisissez exactement le nom de la machine')).toBeInTheDocument()
    expect(requestPowerAction).not.toHaveBeenCalled()

    await user.clear(input)
    await user.type(input, 'homelab')
    await user.click(screen.getByRole('button', { name: 'Confirmer : Redémarrer' }))
    await waitFor(() => expect(requestPowerAction).toHaveBeenCalledWith('reboot', 'homelab'))
    expect(await screen.findByRole('status')).toHaveTextContent('Redémarrage demandé')
  })

  it('explains how to enable the actions when they are disabled', async () => {
    vi.mocked(getPowerSettings).mockResolvedValue({ enabled: false, actions: ['reboot', 'poweroff'] })
    renderCard()
    expect(await screen.findByText(/power_actions_enabled/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Éteindre' })).not.toBeInTheDocument()
  })
})
```

- [ ] **Étape 2 : lancer les tests et constater l'échec**

Commande : `npm --prefix frontend run test -- src/lib src/schemas src/features/host`
Attendu : modules `./machine`, `./power`, `./PowerCard` introuvables ; `canManagePower` non exporté.

- [ ] **Étape 3 : clients, textes, schéma, permissions**

`frontend/src/lib/api/machine.ts` :

```ts
import { requestJSON } from './client'

export type PowerAction = 'reboot' | 'poweroff'

export type PowerSettings = { enabled: boolean; actions: PowerAction[] }

export function getPowerSettings() {
  return requestJSON<PowerSettings>('/api/v1/machine/power')
}

export function requestPowerAction(action: PowerAction, confirmHostname: string) {
  return requestJSON<{ requested: boolean; action: PowerAction }>(
    `/api/v1/machine/power/${action}`,
    { method: 'POST', body: JSON.stringify({ confirmHostname }) },
  )
}
```

`frontend/src/lib/api/client.ts`, ajouter à `errorMessages` avant `INTERNAL_ERROR` :

```ts
  HOSTNAME_MISMATCH: 'Le nom saisi ne correspond pas au nom de la machine.',
  POWER_ACTIONS_DISABLED:
    "Le redémarrage et l'extinction sont désactivés dans la configuration du Manager.",
  ACTION_IN_PROGRESS: 'Une action est déjà en cours.',
  POWER_ACTION_REFUSED:
    "La machine a refusé : règle polkit absente, ou une opération en cours (sauvegarde, mise à jour) bloque l'arrêt.",
  POWER_ACTION_FAILED: "La demande n'a pas pu être transmise à la machine.",
```

`frontend/src/lib/power.ts` :

```ts
import type { PowerAction } from './api/machine'

export const powerActionList: PowerAction[] = ['reboot', 'poweroff']

export const powerActionTexts: Record<
  PowerAction,
  { label: string; warning: string; done: string }
> = {
  reboot: {
    label: 'Redémarrer',
    warning:
      "Tous les services et conteneurs seront arrêtés puis relancés. L'interface sera indisponible pendant le redémarrage.",
    done: "Redémarrage demandé. L'interface reviendra quand la machine aura redémarré.",
  },
  poweroff: {
    label: 'Éteindre',
    warning:
      "La machine s'arrêtera complètement. Homelab Manager ne pourra pas la rallumer : il faudra un accès physique ou Wake-on-LAN.",
    done: "Extinction demandée. La machine va s'arrêter.",
  },
}
```

`frontend/src/schemas/power.ts` :

```ts
import { z } from 'zod'

/** Confirmation of a power action: the exact machine name (API_CONTRACTS.md § 4.5). */
export function powerConfirmationSchema(hostname: string) {
  return z.object({
    confirmHostname: z
      .string()
      .trim()
      .refine(
        (value) => hostname !== '' && value === hostname,
        'Saisissez exactement le nom de la machine',
      ),
  })
}

export type PowerConfirmationValues = { confirmHostname: string }
```

`frontend/src/lib/roles.ts`, ajouter :

```ts
export function canManagePower(role: Role | undefined): boolean {
  return role === 'admin'
}
```

`frontend/src/hooks/useSession.ts` : importer `canManagePower` et ajouter au modèle renvoyé `canManagePower: canManagePower(user?.role),`. Ajouter `canManagePower: false` (ou `true` pour l'administrateur de `UsersPage.test.tsx`) aux objets de session simulés des trois tests de la phase 1A.

- [ ] **Étape 4 : hook et carte**

`frontend/src/features/host/usePowerActions.ts` :

```ts
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import { getPowerSettings, requestPowerAction, type PowerAction } from '../../lib/api/machine'
import { powerActionList, powerActionTexts } from '../../lib/power'
import { powerConfirmationSchema, type PowerConfirmationValues } from '../../schemas/power'

/** Power card of the Machine page (administrators only). */
export function usePowerActions(hostname: string) {
  const { canManagePower } = useSession()
  const settings = useQuery({
    queryKey: ['machine', 'power'],
    queryFn: getPowerSettings,
    enabled: canManagePower,
    staleTime: 60_000,
  })
  const [selected, setSelected] = useState<PowerAction | null>(null)
  const schema = useMemo(() => powerConfirmationSchema(hostname), [hostname])
  const form = useForm<PowerConfirmationValues>({
    resolver: zodResolver(schema),
    defaultValues: { confirmHostname: '' },
  })
  const mutation = useMutation({
    mutationFn: (request: { action: PowerAction; confirmHostname: string }) =>
      requestPowerAction(request.action, request.confirmHostname),
  })

  return {
    visible: canManagePower,
    state: loadState(settings),
    enabled: settings.data?.enabled ?? false,
    hostname,
    actions: powerActionList.map((action) => ({ action, label: powerActionTexts[action].label })),
    selected: selected ? { action: selected, ...powerActionTexts[selected] } : null,
    choose: (action: PowerAction) => {
      mutation.reset()
      form.reset()
      setSelected(action)
    },
    cancel: () => {
      form.reset()
      setSelected(null)
    },
    register: form.register,
    errors: form.formState.errors,
    onConfirm: form.handleSubmit((values) => {
      if (selected) mutation.mutate({ action: selected, confirmHostname: values.confirmHostname })
    }),
    isPending: mutation.isPending,
    /** Once a request is accepted the Manager goes down: no second request. */
    accepted: mutation.isSuccess,
    errorMessage: mutation.error?.message ?? null,
    doneMessage: mutation.data ? powerActionTexts[mutation.data.action].done : null,
  }
}
```

`frontend/src/features/host/PowerCard.tsx` :

```tsx
import { Power } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { usePowerActions } from './usePowerActions'

export function PowerCard({ hostname }: { hostname: string }) {
  const power = usePowerActions(hostname)
  if (!power.visible) return null

  return (
    <Card>
      <CardHeader
        title="Alimentation"
        subtitle="Redémarrer ou éteindre cette machine"
        icon={<Power className="h-3.5 w-3.5" />}
      />
      <CardBody className="space-y-3">
        {power.state.isError && (
          <p className="text-xs text-danger" role="alert">
            {power.state.error.message}
          </p>
        )}
        {!power.state.isLoading && !power.state.isError && !power.enabled && (
          <p className="text-xs text-text-secondary">
            Désactivé dans la configuration du Manager (<code>power_actions_enabled</code>) ;
            l'activation exige aussi la règle polkit fournie. Voir le README.
          </p>
        )}
        {power.enabled && !power.selected && (
          <div className="flex flex-wrap gap-2">
            {power.actions.map((item) => (
              <Button key={item.action} variant="danger" size="sm" onClick={() => power.choose(item.action)}>
                {item.label}
              </Button>
            ))}
          </div>
        )}
        {power.selected && !power.accepted && (
          <form className="space-y-3" onSubmit={power.onConfirm} noValidate>
            <p className="text-xs text-text-secondary">{power.selected.warning}</p>
            <div>
              <label
                htmlFor="confirmHostname"
                className="mb-1.5 block font-mono text-[10px] uppercase tracking-wider text-text-secondary"
              >
                Saisissez « {power.hostname} » pour confirmer
              </label>
              <input
                id="confirmHostname"
                autoComplete="off"
                className="h-9 w-full rounded-md border border-line bg-surface px-3 font-mono text-sm text-text-primary outline-none focus:border-accent"
                {...power.register('confirmHostname')}
              />
              {power.errors.confirmHostname && (
                <p className="mt-1 text-[11px] text-danger" role="alert">
                  {power.errors.confirmHostname.message}
                </p>
              )}
            </div>
            {power.errorMessage && (
              <p className="text-xs text-danger" role="alert">
                {power.errorMessage}
              </p>
            )}
            <div className="flex gap-2">
              <Button type="submit" variant="danger" size="sm" loading={power.isPending}>
                Confirmer : {power.selected.label}
              </Button>
              <Button size="sm" onClick={power.cancel} disabled={power.isPending}>
                Annuler
              </Button>
            </div>
          </form>
        )}
        {power.doneMessage && (
          <p className="text-xs text-warning" role="status">
            {power.doneMessage}
          </p>
        )}
      </CardBody>
    </Card>
  )
}
```

Le nom accessible du bouton de confirmation est « Confirmer : Redémarrer » (texte du bouton). Le jeton `text-warning` est défini dans `frontend/src/styles.css`.

Dans `frontend/src/features/host/HostPage.tsx`, importer `import { PowerCard } from './PowerCard'` et ajouter, comme dernier enfant de l'élément racine `<div className="space-y-4">` (juste avant sa balise fermante, à la fin du `return` de `HostPage`) :

```tsx
      {hostInfo?.hostname && <PowerCard hostname={hostInfo.hostname} />}
```

- [ ] **Étape 5 : lancer les vérifications et constater le succès**

Commande : `npm --prefix frontend run format && npm --prefix frontend run test && npm --prefix frontend run lint && npm --prefix frontend run typecheck && npm --prefix frontend run build`
Attendu : tous les tests passent, lint sans avertissement, build réussi.

- [ ] **Étape 6 : commit**

```bash
scripts/verify.sh
git add frontend/src
git commit -m "feat(frontend): add the power card to the machine page"
```

---

### Tâche 7 : documentation d'état, vérification et essai réel

- [ ] **Étape 1 : documents d'état**

- `docs/TESTING.md`, « Couverture actuelle » : ajouter « actions d'alimentation (permission, confirmation par le nom d'hôte, audit écrit avant l'action, refus polkit classé, demande unique, désactivation par défaut, parcours HTTP avec un faux contrôleur logind), » ; dans « Non couverts automatiquement », ajouter « redémarrage et extinction réels par logind et polkit (machine virtuelle uniquement) ».
- `docs/PRODUCT_SPEC.md` § 6 : mentionner le module M livré, désactivé par défaut ; § 8 : ajouter l'entrée d'historique correspondante.
- `docs/SPEC_ADMINISTRATION.md` : dans l'en-tête d'état, ajouter « redémarrage et extinction (§ 8 bis) ».

- [ ] **Étape 2 : vérification complète**

Commande : `scripts/verify.sh`
Attendu : code de sortie 0.

- [ ] **Étape 3 : essai réel sur une machine virtuelle Debian 13 jetable**

Jamais sur la machine de développement ni sur le serveur du homelab avant cet essai.

1. Installer le Manager selon le README, sans la règle polkit, avec `power_actions_enabled: true` : « Redémarrer » renvoie « La machine a refusé : règle polkit absente… » ; `audit_logs` contient `requested` puis `failure`.
2. Installer la règle polkit : « Redémarrer » avec le bon nom → la VM redémarre ; le Manager revient ; l'audit contient la demande.
3. Lancer `systemd-inhibit --what=shutdown sleep 300` puis « Éteindre » → refus explicite, la VM reste allumée.
4. Sans inhibition, « Éteindre » → la VM s'arrête proprement (`journalctl -b -1 -u homelab-manager` montre l'arrêt ordonné des collecteurs).
5. Avec un compte `viewer` ou `operator` : aucune carte « Alimentation » ; `POST /api/v1/machine/power/reboot` → `403 FORBIDDEN`.
6. Vérifier que `homelab-manager` n'a toujours aucun droit `sudo` et n'appartient qu'aux groupes attendus (`id homelab-manager`).

- [ ] **Étape 4 : commit et revue**

```bash
git add docs
git commit -m "docs: record the machine power module as delivered"
```

Puis lancer `superpowers:requesting-code-review` et relancer `scripts/verify.sh` après corrections.

## Auto-revue du plan

- **Couverture de § 8 bis** : actions par logind sans commande (tâche 3) ; double activation (tâches 5 : réglage et règle polkit) ; rôle `admin` (tâche 1, route en tâche 5) ; confirmation serveur et interface (tâches 1, 4, 6) ; inhibitions respectées (règle polkit sans `-ignore-inhibit`, refus classé en tâche 3) ; audit avant l'action et `failure` (tâche 4) ; demande unique (tâche 4) ; exclusions (aucune route de veille, d'arrêt programmé ni de réveil).
- **Cohérence des noms** : `PowerAction`, `PowerReboot`, `PowerPowerOff`, `PermManagePower`, `PowerController`, `AuditRecorder`, `NewAuditRepository`, `NewPowerUsecase`, `powerActionHandler`, `newTestRouter`, `usePowerActions`, `powerConfirmationSchema`, `powerActionTexts`, `canManagePower` sont définis une fois et réutilisés à l'identique.
- **Dépendance** : godbus v5.2.4 vérifiée (publiée le 13 octobre 2026) ; `dbus.Error` est une valeur, `errors.As` vers `dbus.Error` fonctionne pour les réponses d'erreur (vérifié dans le code source de godbus).
