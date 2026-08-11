package db

import "testing"

// TestRolePermissions pins what each role can reach. Widening one of these
// sets is a deliberate decision, not something to discover from a test
// going green on its own.
func TestRolePermissions(t *testing.T) {
	for _, p := range AllPermissions {
		if !Can(RoleMaster, p) {
			t.Errorf("master is missing %q", p)
		}
		if !Can(RoleManager, p) {
			t.Errorf("manager is missing %q", p)
		}
	}

	// A keyholder runs the space, not the site or its accounts.
	granted := map[Permission]bool{
		PermEvents: true, PermMedia: true, PermBookings: true,
		PermMessages: true, PermReports: true, PermStaff: true,
	}
	for _, p := range AllPermissions {
		if got := Can(RoleKeyholder, p); got != granted[p] {
			t.Errorf("keyholder %q = %v, want %v", p, got, granted[p])
		}
	}
	if Can(RoleKeyholder, PermUsers) {
		t.Error("a keyholder must not be able to manage accounts")
	}
}

// TestMasterIsHidden covers the rule the whole accounts page rests on:
// only a master knows the role exists.
func TestMasterIsHidden(t *testing.T) {
	for _, actor := range []string{RoleManager, RoleKeyholder, "retired-role"} {
		if VisibleRole(actor, RoleMaster) {
			t.Errorf("%s can see the master role", actor)
		}
		for _, r := range VisibleRoles(actor) {
			if r == RoleMaster {
				t.Errorf("%s was offered the master role", actor)
			}
		}
	}
	if !VisibleRole(RoleMaster, RoleMaster) {
		t.Error("a master should see other masters")
	}

	// Everything that isn't master is visible to everyone.
	manager := User{Role: RoleManager}
	if !manager.CanSee(User{Role: RoleKeyholder}) || !manager.CanSee(User{Role: RoleManager}) {
		t.Error("a manager should see managers and keyholders")
	}
	if manager.CanSee(User{Role: RoleMaster}) {
		t.Error("a manager should not see a master account")
	}
	if got := len(User{Role: RoleMaster}.AssignableRoles()); got != len(RoleNames) {
		t.Errorf("a master can assign %d roles, want all %d", got, len(RoleNames))
	}
}

// TestEveryRoleHasLabelAndDescription keeps the account forms from showing
// a bare slug when a role is added.
func TestEveryRoleHasLabelAndDescription(t *testing.T) {
	for _, r := range RoleNames {
		if _, ok := Roles[r]; !ok {
			t.Errorf("role %q is listed but grants nothing", r)
		}
		if RoleLabelFor(r) == r {
			t.Errorf("role %q has no display label", r)
		}
		if RoleDescription(r) == "" {
			t.Errorf("role %q has no description", r)
		}
	}
	if len(Roles) != len(RoleNames) {
		t.Errorf("%d roles defined but %d listed", len(Roles), len(RoleNames))
	}
}
