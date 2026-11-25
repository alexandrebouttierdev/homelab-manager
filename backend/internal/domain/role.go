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
	// PermManagePower covers rebooting and powering off the machine.
	PermManagePower Permission = "machine:power"
	// PermViewAudit covers reading and exporting the audit log.
	PermViewAudit Permission = "audit:view"
	// PermOperateAlerts covers acknowledging alerts and silences.
	PermOperateAlerts Permission = "alerts:operate"
	// PermManageAlerts covers alert rules and notification channels.
	PermManageAlerts Permission = "alerts:manage"
	// PermScanVulnerabilities covers starting a vulnerability scan.
	PermScanVulnerabilities Permission = "vulnerabilities:scan"
	// PermManageServices covers starting, stopping, restarting and
	// reloading the units of the allow list.
	PermManageServices Permission = "services:manage"
	// PermManageContainers covers starting, stopping and restarting
	// containers (D-5).
	PermManageContainers Permission = "containers:manage"
	// PermManageSystem covers the host name, time zone and language of
	// the machine (D-6).
	PermManageSystem Permission = "system:manage"
	// PermRunBackups covers starting a backup.
	PermRunBackups Permission = "backups:run"
	// PermManageBackups covers the repository, the sources, the
	// exclusions and the schedule.
	PermManageBackups Permission = "backups:manage"
)

// rolePermissions is the single source of the role policy.
var rolePermissions = map[Role]map[Permission]bool{
	RoleViewer: {},
	RoleOperator: {
		PermOperateAlerts: true, PermScanVulnerabilities: true, PermManageServices: true, PermRunBackups: true,
		PermManageContainers: true,
	},
	RoleAdmin: {
		PermManageSettings: true, PermManageUsers: true, PermManagePower: true, PermViewAudit: true,
		PermOperateAlerts: true, PermManageAlerts: true, PermScanVulnerabilities: true, PermManageServices: true,
		PermRunBackups: true, PermManageBackups: true, PermManageContainers: true, PermManageSystem: true,
	},
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
