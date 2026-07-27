package db

// Permission is one thing an account is allowed to do. Handlers check a
// permission rather than a role name, so adding a role later is a matter
// of listing what it grants — no handler has to change.
type Permission string

const (
	PermEvents   Permission = "events"
	PermPosts    Permission = "posts"
	PermShop     Permission = "shop"
	PermPhotos   Permission = "photos"
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
var AllPermissions = []Permission{PermEvents, PermPosts, PermShop, PermPhotos, PermPeople, PermGroups, PermMedia, PermBookings, PermMessages, PermForms, PermReports, PermStaff, PermUsers}

// Label renders a permission for display.
func (p Permission) Label() string {
	switch p {
	case PermEvents:
		return "Events"
	case PermPosts:
		return "Blog"
	case PermShop:
		return "Shop"
	case PermPhotos:
		return "Photos"
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

// RoleMaster has everything, including managing other accounts. It's the
// only role for now; narrower ones get added here as they're needed, and
// the account form picks up any role listed in Roles automatically.
const RoleMaster = "master"

// Roles maps a role name to what it grants.
var Roles = map[string][]Permission{
	RoleMaster: AllPermissions,
}

// RoleNames lists the defined roles in display order.
var RoleNames = []string{RoleMaster}

// RoleLabelFor renders a role name for display.
func RoleLabelFor(role string) string {
	switch role {
	case RoleMaster:
		return "Master"
	}
	return role
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
