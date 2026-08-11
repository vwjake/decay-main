package admin

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"decay-main/db"
	"decay-main/mail"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

// registerInviteRoutes wires up /admin/invites, where a master or manager
// sends out a signup link instead of picking a password for someone
// themselves. It shares the accounts permission — anyone who can create an
// account directly can also hand out a link that does the same thing.
func registerInviteRoutes(g *echo.Group, conn *sql.DB, mailer *mail.Mailer, siteURL string) {
	invites := g.Group("/invites", requirePermission(db.PermUsers))
	invites.GET("", listInvites(conn))
	invites.POST("", createInvite(conn, mailer, siteURL))
	invites.POST("/:id/revoke", revokeInvite(conn))
}

func listInvites(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		return renderInvites(c, conn, "", "")
	}
}

func createInvite(conn *sql.DB, mailer *mail.Mailer, siteURL string) echo.HandlerFunc {
	return func(c echo.Context) error {
		me := currentUser(c)
		role := c.FormValue("role")
		if role == "" {
			role = db.RoleKeyholder
		}
		// Same rule as creating an account directly: nobody can send out a
		// link for a role they can't see, so a manager can't mint a master
		// by mail either.
		if !me.CanSee(db.User{Role: role}) {
			return renderInvites(c, conn, "Unknown role.", "")
		}

		email := strings.TrimSpace(c.FormValue("email"))
		invite, err := db.CreateInvite(conn, role, email, c.FormValue("display_name"), me.ID)
		if err != nil {
			return renderInvites(c, conn, "Couldn't create that invite: "+err.Error(), "")
		}

		link := strings.TrimRight(siteURL, "/") + invite.SignupPath()
		status := "Link created: " + link
		if email != "" {
			body := fmt.Sprintf(
				"You've been invited to set up a DECAY admin account as a %s.\n\n"+
					"Choose your username and password here:\n%s\n\n"+
					"This link works once and expires %s.",
				invite.RoleLabel(), link, invite.ExpiresLabel(),
			)
			if err := mailer.Send(email, "Set up your DECAY admin account", body); err != nil {
				status = "Link created, but the email didn't send: " + err.Error() + " Share it directly: " + link
			} else if mailer.Enabled() {
				status = "Invite emailed to " + email + ". Link: " + link
			}
		}
		return renderInvites(c, conn, "", status)
	}
}

func revokeInvite(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.DeleteInvite(conn, id); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/invites")
	}
}

func renderInvites(c echo.Context, conn *sql.DB, errorMsg, status string) error {
	me := currentUser(c)
	all, err := db.ListPendingInvites(conn)
	if err != nil {
		return err
	}
	// Same visibility rule as the accounts list: a manager shouldn't see
	// that a master-role invite exists.
	visible := make([]db.Invite, 0, len(all))
	for _, inv := range all {
		if me.CanSee(db.User{Role: inv.Role}) {
			visible = append(visible, inv)
		}
	}
	return views.AdminInvites(visible, me, errorMsg, status).Render(c.Request().Context(), c.Response())
}
