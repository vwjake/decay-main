package db

// Permission is one thing an account is allowed to do. Handlers check a
// permission rather than a role name, so adding a role later is a matter
// of listing what it grants — no handler has to change.
type Permission string

const (
	PermEvents   Permission = "events"
	PermPosts    Permission = "posts"
	PermShop     Permission = "shop"
	PermPeople   Permission = "people"
	PermGroups   Permission = "groups"
	PermMedia    Permission = "media"
	PermBookings Permission = "bookings"
	PermMessages Permission = "messages"
	PermForms    Permission = "forms"
	PermReports  Permission = "reports"
	PermStaff    Permission = "staff"
	PermUsers    Permission = "users"
)

// AllPermissions is every permission, in the order they're displayed.
var AllPermissions = []Permission{PermEvents, PermPosts, PermShop, PermPeople, PermGroups, PermMedia, PermBookings, PermMessages, PermForms, PermReports, PermStaff, PermUsers}

// Label renders a permission for display.
func (p Permission) Label() string {
	switch p {
	case PermEvents:
		return "Events"
	case PermPosts:
		return "Blog"
	case PermShop:
		return "Shop"
	case PermPeople:
		return "People"
	case PermGroups:
		return "Groups"
	case PermMedia:
		return "Media"
	case PermBookings:
		return "Bookings"
	case PermMessages:
		return "Messages"
	case PermForms:
		return "Forms"
	case PermReports:
		return "Reports"
	case PermStaff:
		return "Staff"
	case PermUsers:
		return "Accounts"
	}
	return string(p)
}

const (
	// RoleMaster has everything and is the hidden owner role: it's the
	// only role that can see other master accounts or hand the role out.
	// See VisibleRole.
	RoleMaster = "master"
	// RoleManager has everything a master does, including managing
	// accounts. It's the role actually handed out — a master is a manager
	// that doesn't show up in the list.
	RoleManager = "manager"
	// RoleKeyholder runs the space day to day: the calendar, who's asking
	// to book it, who's written in, the gallery, the internal staff
	// calendar, and the numbers after a show. Not the shop, the site's
	// copy, or accounts.
	RoleKeyholder = "keyholder"
)

// Roles maps a role name to what it grants. The account form picks up any
// role listed here automatically, so a narrower role is an entry here and
// no handler change.
var Roles = map[string][]Permission{
	RoleMaster:  AllPermissions,
	RoleManager: AllPermissions,
	RoleKeyholder: {
		PermEvents, PermMedia, PermBookings, PermMessages, PermReports, PermStaff,
	},
}

// RoleNames lists the defined roles in display order. Master is last
// because most accounts never see it — VisibleRoles filters it out.
var RoleNames = []string{RoleManager, RoleKeyholder, RoleMaster}

// RoleLabelFor renders a role name for display.
func RoleLabelFor(role string) string {
	switch role {
	case RoleMaster:
		return "Master"
	case RoleManager:
		return "Manager"
	case RoleKeyholder:
		return "Keyholder"
	}
	return role
}

// RoleDescription is the one-line explanation shown next to a role on the
// account forms.
func RoleDescription(role string) string {
	switch role {
	case RoleMaster:
		return "Everything, and the only role that can see or create other masters."
	case RoleManager:
		return "Everything, including creating accounts and handing out access."
	case RoleKeyholder:
		return "Events, bookings, messages, media, reports, and the staff calendar."
	}
	return ""
}

// VisibleRole reports whether an account with actorRole is allowed to know
// that role exists. Master is concealed from everyone who isn't one, so a
// manager can neither see a master account in the list nor promote anyone
// (including themselves) into the role.
func VisibleRole(actorRole, role string) bool {
	if role == RoleMaster {
		return actorRole == RoleMaster
	}
	return true
}

// VisibleRoles is the set of roles an account may pick from, in display
// order.
func VisibleRoles(actorRole string) []string {
	var out []string
	for _, r := range RoleNames {
		if VisibleRole(actorRole, r) {
			out = append(out, r)
		}
	}
	return out
}

// Can reports whether a role grants a permission. An unknown role grants
// nothing, so a row with a role that's been retired from the code fails
// closed rather than being treated as an admin.
func Can(role string, want Permission) bool {
	for _, p := range Roles[role] {
		if p == want {
			return true
		}
	}
	return false
}
