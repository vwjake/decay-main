package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// InviteTTL is how long a signup link stays good before it has to be
// reissued.
const InviteTTL = 7 * 24 * time.Hour

// ErrInviteNotFound covers an unknown, expired, or already-used token, so a
// stale signup link fails the same way whether it never existed or simply
// isn't good anymore — nothing about the failure hints at which.
var ErrInviteNotFound = errors.New("invite not found")

type Invite struct {
	ID          int64
	Token       string
	Role        string
	Email       string
	DisplayName string
	InvitedBy   int64
	CreatedAt   time.Time
	ExpiresAt   time.Time
	UsedAt      *time.Time
}

// RoleLabel renders the invite's role for display.
func (i Invite) RoleLabel() string { return RoleLabelFor(i.Role) }

// Expired reports whether the link is past its expiry.
func (i Invite) Expired() bool { return time.Now().After(i.ExpiresAt) }

// Used reports whether the link has already made an account.
func (i Invite) Used() bool { return i.UsedAt != nil }

// SignupPath is the public URL a recipient uses to claim the invite.
func (i Invite) SignupPath() string { return "/signup/" + i.Token }

// ExpiresLabel renders when the link stops working.
func (i Invite) ExpiresLabel() string { return i.ExpiresAt.Format("Jan 2, 2006 3:04 PM") }

// CreateInvite mints a one-time signup link for a new account. email and
// displayName are both optional: email only decides whether a link gets
// mailed out, displayName is just a suggestion the signup form prefills.
func CreateInvite(conn *sql.DB, role, email, displayName string, invitedBy int64) (Invite, error) {
	if _, ok := Roles[role]; !ok {
		return Invite{}, fmt.Errorf("unknown role %q", role)
	}
	token, err := generateInviteToken()
	if err != nil {
		return Invite{}, err
	}
	email = strings.TrimSpace(email)
	displayName = strings.TrimSpace(displayName)
	now := time.Now()
	expires := now.Add(InviteTTL)

	res, err := conn.Exec(
		`INSERT INTO invites (token, role, email, display_name, invited_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		token, role, email, displayName, invitedBy, now.Format(timeLayout), expires.Format(timeLayout),
	)
	if err != nil {
		return Invite{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Invite{}, err
	}
	return Invite{
		ID: id, Token: token, Role: role, Email: email, DisplayName: displayName,
		InvitedBy: invitedBy, CreatedAt: now, ExpiresAt: expires,
	}, nil
}

const inviteColumns = `id, token, role, email, display_name, invited_by, created_at, expires_at, used_at`

func scanInvites(rows *sql.Rows) ([]Invite, error) {
	var invites []Invite
	for rows.Next() {
		var i Invite
		var invitedBy sql.NullInt64
		var createdAt, expiresAt string
		var usedAt sql.NullString
		if err := rows.Scan(&i.ID, &i.Token, &i.Role, &i.Email, &i.DisplayName, &invitedBy, &createdAt, &expiresAt, &usedAt); err != nil {
			return nil, err
		}
		i.InvitedBy = invitedBy.Int64
		created, err := parseStamp(createdAt)
		if err != nil {
			return nil, err
		}
		i.CreatedAt = created
		expires, err := parseStamp(expiresAt)
		if err != nil {
			return nil, err
		}
		i.ExpiresAt = expires
		if usedAt.Valid {
			t, err := parseStamp(usedAt.String)
			if err != nil {
				return nil, err
			}
			i.UsedAt = &t
		}
		invites = append(invites, i)
	}
	return invites, rows.Err()
}

// ListPendingInvites returns invites that haven't been used yet, newest
// first — expired ones stay listed too, so an admin can see a link went
// stale rather than it just quietly vanishing.
func ListPendingInvites(conn *sql.DB) ([]Invite, error) {
	rows, err := conn.Query(`SELECT ` + inviteColumns + ` FROM invites WHERE used_at IS NULL ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInvites(rows)
}

// InviteByToken looks up a live invite: unused and not expired. Anything
// else — unknown token, spent, or past its expiry — is ErrInviteNotFound.
func InviteByToken(conn *sql.DB, token string) (Invite, error) {
	rows, err := conn.Query(`SELECT `+inviteColumns+` FROM invites WHERE token = ?`, token)
	if err != nil {
		return Invite{}, err
	}
	defer rows.Close()

	invites, err := scanInvites(rows)
	if err != nil {
		return Invite{}, err
	}
	if len(invites) == 0 {
		return Invite{}, ErrInviteNotFound
	}
	invite := invites[0]
	if invite.Used() || invite.Expired() {
		return Invite{}, ErrInviteNotFound
	}
	return invite, nil
}

// MarkInviteUsed spends the invite, so the same link can't create a second
// account.
func MarkInviteUsed(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`UPDATE invites SET used_at = ? WHERE id = ?`, time.Now().Format(timeLayout), id)
	return err
}

// DeleteInvite revokes a link before it's used.
func DeleteInvite(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM invites WHERE id = ?`, id)
	return err
}

// generateInviteToken creates a random hex token for a signup link, the
// same shape as an order's secure token.
func generateInviteToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
