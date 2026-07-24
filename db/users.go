package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLength is the shortest password an account may have. Short
// enough not to be a nuisance for a volunteer-run space, long enough that
// a stolen database isn't trivially reversed.
const MinPasswordLength = 10

// ErrUsernameTaken is returned when a username is already in use.
var ErrUsernameTaken = errors.New("username already taken")

// ErrPasswordTooShort is returned when a password fails MinPasswordLength.
var ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)

// decoyHash is a real bcrypt hash, compared against when the username is
// unknown. A malformed hash would be rejected before any work happened,
// which is exactly the timing difference that reveals whether an account
// exists — so this has to be genuine.
var decoyHash = func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("no such account"), bcrypt.DefaultCost)
	if err != nil {
		panic("generating decoy hash: " + err.Error())
	}
	return h
}()

type User struct {
	ID           int64
	Username     string
	DisplayName  string
	Role         string
	CreatedAt    time.Time
	LastLoginAt  *time.Time
	passwordHash string
}

// Name is what to show for a user: their display name if they set one,
// otherwise the username they log in with.
func (u User) Name() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Username
}

// Can reports whether the user's role grants a permission.
func (u User) Can(p Permission) bool { return Can(u.Role, p) }

// RoleLabel renders the user's role for display.
func (u User) RoleLabel() string { return RoleLabelFor(u.Role) }

// LastLogin renders the last sign-in for display.
func (u User) LastLogin() string {
	if u.LastLoginAt == nil {
		return "never"
	}
	return u.LastLoginAt.Format("Jan 2, 2006 3:04 PM")
}

const userColumns = `id, username, display_name, password_hash, role, created_at, last_login_at`

func scanUsers(rows *sql.Rows) ([]User, error) {
	var users []User
	for rows.Next() {
		var u User
		var displayName sql.NullString
		var createdAt string
		var lastLogin sql.NullString
		if err := rows.Scan(&u.ID, &u.Username, &displayName, &u.passwordHash, &u.Role, &createdAt, &lastLogin); err != nil {
			return nil, err
		}
		u.DisplayName = displayName.String
		created, err := parseStamp(createdAt)
		if err != nil {
			return nil, err
		}
		u.CreatedAt = created
		if lastLogin.Valid {
			t, err := parseStamp(lastLogin.String)
			if err != nil {
				return nil, err
			}
			u.LastLoginAt = &t
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// parseStamp accepts both timestamp shapes in the database: SQLite's own
// `datetime('now')` output for defaulted columns, and RFC3339 for the ones
// Go writes.
func parseStamp(raw string) (time.Time, error) {
	if t, err := time.Parse(timeLayout, raw); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05", raw)
}

// ListUsers returns every account, oldest first.
func ListUsers(conn *sql.DB) ([]User, error) {
	rows, err := conn.Query(`SELECT ` + userColumns + ` FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

// UserByID fetches one account.
func UserByID(conn *sql.DB, id int64) (User, error) {
	rows, err := conn.Query(`SELECT `+userColumns+` FROM users WHERE id = ?`, id)
	if err != nil {
		return User{}, err
	}
	defer rows.Close()

	users, err := scanUsers(rows)
	if err != nil {
		return User{}, err
	}
	if len(users) == 0 {
		return User{}, sql.ErrNoRows
	}
	return users[0], nil
}

// Authenticate checks a username and password. It returns sql.ErrNoRows
// for both an unknown user and a wrong password, so a caller can't tell
// the two apart and neither can whoever is trying passwords. The bcrypt
// comparison runs either way, so the response takes the same time whether
// or not the account exists.
func Authenticate(conn *sql.DB, username, password string) (User, error) {
	rows, err := conn.Query(`SELECT `+userColumns+` FROM users WHERE lower(username) = lower(?)`, strings.TrimSpace(username))
	if err != nil {
		return User{}, err
	}
	defer rows.Close()

	users, err := scanUsers(rows)
	if err != nil {
		return User{}, err
	}

	hash := decoyHash
	var user User
	found := len(users) == 1
	if found {
		user = users[0]
		hash = []byte(user.passwordHash)
	}

	if err := bcrypt.CompareHashAndPassword(hash, []byte(password)); err != nil || !found {
		return User{}, sql.ErrNoRows
	}
	return user, nil
}

// CreateUser adds an account, returning its id.
func CreateUser(conn *sql.DB, username, displayName, password, role string) (int64, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return 0, errors.New("username is required")
	}
	if len([]rune(password)) < MinPasswordLength {
		return 0, ErrPasswordTooShort
	}
	if _, ok := Roles[role]; !ok {
		return 0, fmt.Errorf("unknown role %q", role)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}

	res, err := conn.Exec(
		`INSERT INTO users (username, display_name, password_hash, role) VALUES (?, ?, ?, ?)`,
		username, strings.TrimSpace(displayName), string(hash), role,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrUsernameTaken
		}
		return 0, err
	}
	return res.LastInsertId()
}

// EnsureFirstUser creates the initial master account from the credentials
// in the environment, but only when no accounts exist at all. It's the
// way out of the chicken-and-egg problem of needing an account to create
// accounts, and it means an existing deployment keeps working on the
// ADMIN_USERNAME/ADMIN_PASSWORD it already has. It reports whether it
// created anything.
//
// Once a single account exists this does nothing, so changing the
// environment variables later has no effect — passwords are managed in
// the admin panel from that point on.
//
// MinPasswordLength deliberately isn't enforced here. An existing
// deployment may already have a short ADMIN_PASSWORD, and refusing to
// start would leave nobody able to reach the admin panel — which is the
// only place the password can be changed. The caller warns instead.
func EnsureFirstUser(conn *sql.DB, username, password string) (bool, error) {
	var count int
	if err := conn.QueryRow(`SELECT count(*) FROM users`).Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return false, errors.New("username is required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, err
	}
	if _, err := conn.Exec(
		`INSERT INTO users (username, display_name, password_hash, role) VALUES (?, '', ?, ?)`,
		username, string(hash), RoleMaster,
	); err != nil {
		return false, err
	}
	return true, nil
}

// UpdateUser saves an account's display name and role.
func UpdateUser(conn *sql.DB, id int64, displayName, role string) error {
	if _, ok := Roles[role]; !ok {
		return fmt.Errorf("unknown role %q", role)
	}
	_, err := conn.Exec(
		`UPDATE users SET display_name = ?, role = ? WHERE id = ?`,
		strings.TrimSpace(displayName), role, id,
	)
	return err
}

// SetPassword replaces an account's password.
func SetPassword(conn *sql.DB, id int64, password string) error {
	if len([]rune(password)) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = conn.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), id)
	return err
}

// DeleteUser removes an account.
func DeleteUser(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

// TouchLogin records a successful sign-in.
func TouchLogin(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`UPDATE users SET last_login_at = ? WHERE id = ?`, time.Now().Format(timeLayout), id)
	return err
}

// CountUsersWith returns how many accounts hold a given permission. The
// admin panel uses it to refuse the change that would leave nobody able
// to manage accounts.
func CountUsersWith(conn *sql.DB, p Permission) (int, error) {
	users, err := ListUsers(conn)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, u := range users {
		if u.Can(p) {
			n++
		}
	}
	return n, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}
