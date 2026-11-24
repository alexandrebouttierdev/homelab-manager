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
		{RoleViewer, PermManagePower, false},
		{RoleOperator, PermManagePower, false},
		{RoleAdmin, PermManagePower, true},
		{RoleOperator, PermViewAudit, false},
		{RoleAdmin, PermViewAudit, true},
		{RoleViewer, PermManageServices, false},
		{RoleOperator, PermManageServices, true},
		{RoleAdmin, PermManageServices, true},
		{RoleViewer, PermRunBackups, false},
		{RoleOperator, PermRunBackups, true},
		{RoleOperator, PermManageBackups, false},
		{RoleAdmin, PermManageBackups, true},
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
