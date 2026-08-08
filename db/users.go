package db

import (
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLength is the shortest password an account may have. Short
// enough not to be a nuisance for a volunteer-run space, long enough that
// a stolen database isn't trivially reversed.
const MinPasswordLength = 10

// MaxBlurbLength caps the profile blurb. It's a line about yourself, not
// a bio page — the People table is where a real bio goes.
const MaxBlurbLength = 250

// AvatarsSubdir keeps account photos apart from the other uploads.
const AvatarsSubdir = "avatars"

// ErrUsernameTaken is returned when a username is already in use.
var ErrUsernameTaken = errors.New("username already taken")

// ErrPasswordTooShort is returned when a password fails MinPasswordLength.
var ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)

// ErrBlurbTooLong is returned when a blurb exceeds MaxBlurbLength.
var ErrBlurbTooLong = fmt.Errorf("blurb must be %d characters or fewer", MaxBlurbLength)

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
	ID          int64
	Username    string
	DisplayName string
	Role        string
	// Photo is a filename under uploads/avatars/, empty when there's none.
	Photo string
	// Blurb is the free-text line an account writes about itself. Stored
	// already stripped of markup — see SanitizeBlurb.
	Blurb        string
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

// RoleDescription explains the user's role in a line.
func (u User) RoleDescription() string { return RoleDescription(u.Role) }

// CanSee reports whether this account is allowed to know that another one
// exists. Every handler under /admin/users checks it, so a hidden account
// is a 404 rather than a row that's merely missing from the list.
func (u User) CanSee(other User) bool { return VisibleRole(u.Role, other.Role) }

// AssignableRoles lists the roles this account may give out.
func (u User) AssignableRoles() []string { return VisibleRoles(u.Role) }

func (u User) HasPhoto() bool { return u.Photo != "" }

// PhotoPath is the web-sized copy pages display; avatars come off a phone
// at full resolution the same as any other upload.
func (u User) PhotoPath() string {
	return "/uploads/" + AvatarsSubdir + "/web/" + strings.TrimSuffix(u.Photo, path.Ext(u.Photo)) + ".jpg"
}

func (u User) HasBlurb() bool { return u.Blurb != "" }

// rawTextTags are the elements whose *contents* are code rather than
// words, so stripping the tag alone would leave the payload behind as
// text. SanitizeBlurb drops them wholesale.
var rawTextTags = []string{"script", "style"}

// SanitizeBlurb reduces a submitted blurb to plain text. The templates
// escape what they render, so this isn't the thing standing between the
// site and an injection — it's so nothing that could ever be *executed* if
// the value is one day put somewhere less careful (an email, an export, a
// template that reaches for templ.Raw) is in the database to begin with.
//
// Everything between angle brackets goes, along with the body of a script
// or style tag; then any stray bracket, then control characters other than
// newline, and runs of blank lines collapse. The result is text, not
// markup.
func SanitizeBlurb(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	for _, tag := range rawTextTags {
		raw = dropElement(raw, tag)
	}

	// Drop tag-shaped spans whole. Unbalanced brackets are handled by the
	// depth never going below zero, so "a < b" loses the bracket and the
	// rest of the line rather than half a tag being reassembled.
	var b strings.Builder
	depth := 0
	for _, r := range raw {
		switch {
		case r == '<':
			depth++
		case r == '>':
			if depth > 0 {
				depth--
			}
		case depth > 0:
			// inside a tag: discard
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case unicode.IsControl(r):
			// discard
		default:
			b.WriteRune(r)
		}
	}

	out := b.String()
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(out)
}

// dropElement removes "<tag …> … </tag>" spans, and an unclosed opening
// tag along with everything after it — an unterminated <script> that
// swallows the rest of the blurb is the safe reading, not the one that
// keeps the payload.
func dropElement(s, tag string) string {
	for {
		lower := strings.ToLower(s)
		start := indexTag(lower, "<"+tag)
		if start < 0 {
			return s
		}
		end := strings.Index(lower[start:], "</"+tag)
		if end < 0 {
			return s[:start]
		}
		rest := s[start+end:]
		if close := strings.Index(rest, ">"); close >= 0 {
			s = s[:start] + rest[close+1:]
		} else {
			return s[:start]
		}
	}
}

// indexTag finds "<tag" only where it's a whole tag name, so "<strong>"
// isn't mistaken for the start of a "<s" element.
func indexTag(lower, prefix string) int {
	for i := 0; ; {
		j := strings.Index(lower[i:], prefix)
		if j < 0 {
			return -1
		}
		at := i + j
		after := at + len(prefix)
		if after >= len(lower) || !isTagNameChar(lower[after]) {
			return at
		}
		i = after
	}
}

func isTagNameChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-'
}

// BlurbLines splits a blurb for display, so a template can keep the line
// breaks someone typed without being handed HTML.
func (u User) BlurbLines() []string {
	var out []string
	for _, line := range strings.Split(u.Blurb, "\n") {
		out = append(out, strings.TrimSpace(line))
	}
	return out
}

// LastLogin renders the last sign-in for display.
func (u User) LastLogin() string {
	if u.LastLoginAt == nil {
		return "never"
	}
	return u.LastLoginAt.Format("Jan 2, 2006 3:04 PM")
}

const userColumns = `id, username, display_name, password_hash, role, photo, blurb, created_at, last_login_at`

func scanUsers(rows *sql.Rows) ([]User, error) {
	var users []User
	for rows.Next() {
		var u User
		var displayName sql.NullString
		var createdAt string
		var lastLogin sql.NullString
		if err := rows.Scan(&u.ID, &u.Username, &displayName, &u.passwordHash, &u.Role, &u.Photo, &u.Blurb, &createdAt, &lastLogin); err != nil {
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

// UpdateProfile saves the parts of an account its own holder controls:
// the name shown around the panel and the blurb under it.
func UpdateProfile(conn *sql.DB, id int64, displayName, blurb string) error {
	blurb, err := checkBlurb(blurb)
	if err != nil {
		return err
	}
	_, err = conn.Exec(
		`UPDATE users SET display_name = ?, blurb = ? WHERE id = ?`,
		strings.TrimSpace(displayName), blurb, id,
	)
	return err
}

// SetBlurb replaces just the blurb, which is how an account manager clears
// one someone else wrote.
func SetBlurb(conn *sql.DB, id int64, blurb string) error {
	blurb, err := checkBlurb(blurb)
	if err != nil {
		return err
	}
	_, err = conn.Exec(`UPDATE users SET blurb = ? WHERE id = ?`, blurb, id)
	return err
}

// checkBlurb sanitizes and length-checks a submitted blurb. It lives here
// rather than in the handlers so nothing reaches the column unfiltered,
// whichever page wrote it.
func checkBlurb(raw string) (string, error) {
	blurb := SanitizeBlurb(raw)
	if len([]rune(blurb)) > MaxBlurbLength {
		return "", ErrBlurbTooLong
	}
	return blurb, nil
}

// SetUserPhoto points an account at a new avatar and returns the filename
// it replaced, so the caller can delete the file it no longer needs.
func SetUserPhoto(conn *sql.DB, id int64, filename string) (string, error) {
	var previous string
	if err := conn.QueryRow(`SELECT photo FROM users WHERE id = ?`, id).Scan(&previous); err != nil {
		return "", err
	}
	_, err := conn.Exec(`UPDATE users SET photo = ? WHERE id = ?`, filename, id)
	return previous, err
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

// DeleteUser removes an account and returns its avatar filename, so the
// caller can clean the file up the same way deleting a person does.
func DeleteUser(conn *sql.DB, id int64) (string, error) {
	var photo string
	if err := conn.QueryRow(`SELECT photo FROM users WHERE id = ?`, id).Scan(&photo); err != nil {
		return "", err
	}
	_, err := conn.Exec(`DELETE FROM users WHERE id = ?`, id)
	return photo, err
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
