package admin

import (
	"database/sql"
	"errors"
	"net/http"

	"decay-main/db"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

// registerSignupRoutes wires up the public side of an invite: no auth, no
// permission check — the token in the URL is what stands in for both.
func registerSignupRoutes(e *echo.Echo, conn *sql.DB) {
	e.GET("/signup/:token", signupForm(conn))
	e.POST("/signup/:token", claimInvite(conn))
}

func signupForm(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		invite, err := db.InviteByToken(conn, c.Param("token"))
		if errors.Is(err, db.ErrInviteNotFound) {
			return views.SignupExpired().Render(c.Request().Context(), c.Response())
		}
		if err != nil {
			return err
		}
		return views.Signup(invite, "").Render(c.Request().Context(), c.Response())
	}
}

func claimInvite(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		invite, err := db.InviteByToken(conn, c.Param("token"))
		if errors.Is(err, db.ErrInviteNotFound) {
			return views.SignupExpired().Render(c.Request().Context(), c.Response())
		}
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			return views.Signup(invite, msg).Render(c.Request().Context(), c.Response())
		}

		password := c.FormValue("password")
		if password != c.FormValue("password_confirm") {
			return rerender("Those passwords don't match.")
		}

		displayName := c.FormValue("display_name")
		if displayName == "" {
			displayName = invite.DisplayName
		}

		id, err := db.CreateUser(conn, c.FormValue("username"), displayName, password, invite.Role)
		switch {
		case errors.Is(err, db.ErrUsernameTaken):
			return rerender("That username is already taken.")
		case errors.Is(err, db.ErrPasswordTooShort):
			return rerender(db.ErrPasswordTooShort.Error() + ".")
		case err != nil:
			return err
		}

		if err := db.MarkInviteUsed(conn, invite.ID); err != nil {
			return err
		}

		// Sign the new account straight in, the same way logging in does,
		// so accepting an invite lands in the admin panel rather than back
		// at a login form for credentials just chosen.
		sess := getSession(c)
		for k := range sess.Values {
			delete(sess.Values, k)
		}
		sess.Values["user_id"] = id
		if err := sess.Save(c.Request(), c.Response()); err != nil {
			return err
		}
		if err := db.TouchLogin(conn, id); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin")
	}
}
