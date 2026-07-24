package admin

import (
	"database/sql"
	"errors"
	"net/http"

	"decay-main/db"
	"decay-main/views"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

const sessionName = "decay_admin"

// userContextKey holds the signed-in account for the rest of the request.
const userContextKey = "admin_user"

// getSession always returns a usable session. gorilla/sessions returns a
// fresh (unauthenticated) session alongside a non-nil error whenever the
// cookie fails to decode — e.g. after a server restart regenerates an
// ephemeral SESSION_SECRET, or the cookie is simply stale — so that case
// should fall through to "not logged in", not a 500.
func getSession(c echo.Context) *sessions.Session {
	sess, _ := session.Get(sessionName, c)
	return sess
}

// currentUser returns the signed-in account. It is only valid inside
// handlers behind requireAuth, which is what puts it there.
func currentUser(c echo.Context) db.User {
	u, _ := c.Get(userContextKey).(db.User)
	return u
}

func loginForm(c echo.Context) error {
	return views.AdminLogin("").Render(c.Request().Context(), c.Response())
}

func login(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, err := db.Authenticate(conn, c.FormValue("username"), c.FormValue("password"))
		if errors.Is(err, sql.ErrNoRows) {
			// Deliberately the same message for an unknown account and a
			// wrong password.
			return views.AdminLogin("Invalid username or password.").Render(c.Request().Context(), c.Response())
		}
		if err != nil {
			return err
		}

		sess := getSession(c)
		// Drop anything the pre-login session carried, so a fixed cookie
		// can't be reused to ride in on this one.
		for k := range sess.Values {
			delete(sess.Values, k)
		}
		sess.Values["user_id"] = user.ID
		if err := sess.Save(c.Request(), c.Response()); err != nil {
			return err
		}

		if err := db.TouchLogin(conn, user.ID); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin")
	}
}

func logout(c echo.Context) error {
	sess := getSession(c)
	for k := range sess.Values {
		delete(sess.Values, k)
	}
	sess.Options.MaxAge = -1
	if err := sess.Save(c.Request(), c.Response()); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/admin/login")
}

// requireAuth loads the signed-in account on every request rather than
// trusting what the cookie says about it, so a deleted account or a role
// change takes effect immediately instead of at the end of the session.
func requireAuth(conn *sql.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sess := getSession(c)
			id, ok := sess.Values["user_id"].(int64)
			if !ok {
				return c.Redirect(http.StatusSeeOther, "/admin/login")
			}

			user, err := db.UserByID(conn, id)
			if errors.Is(err, sql.ErrNoRows) {
				return logout(c)
			}
			if err != nil {
				return err
			}

			c.Set(userContextKey, user)
			return next(c)
		}
	}
}

// requirePermission gates a route group on one permission.
func requirePermission(p db.Permission) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !currentUser(c).Can(p) {
				return echo.NewHTTPError(http.StatusForbidden, "You don't have access to that.")
			}
			return next(c)
		}
	}
}
