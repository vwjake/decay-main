package admin

import (
	"crypto/subtle"
	"net/http"

	"decay-main/views"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

const sessionName = "decay_admin"

// getSession always returns a usable session. gorilla/sessions returns a
// fresh (unauthenticated) session alongside a non-nil error whenever the
// cookie fails to decode — e.g. after a server restart regenerates an
// ephemeral SESSION_SECRET, or the cookie is simply stale — so that case
// should fall through to "not logged in", not a 500.
func getSession(c echo.Context) *sessions.Session {
	sess, _ := session.Get(sessionName, c)
	return sess
}

func loginForm(c echo.Context) error {
	return views.AdminLogin("").Render(c.Request().Context(), c.Response())
}

func login(cfg Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		username := c.FormValue("username")
		password := c.FormValue("password")

		userOK := subtle.ConstantTimeCompare([]byte(username), []byte(cfg.Username)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(password), []byte(cfg.Password)) == 1
		if !userOK || !passOK {
			return views.AdminLogin("Invalid username or password.").Render(c.Request().Context(), c.Response())
		}

		sess := getSession(c)
		sess.Values["authenticated"] = true
		if err := sess.Save(c.Request(), c.Response()); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin")
	}
}

func logout(c echo.Context) error {
	sess := getSession(c)
	sess.Values["authenticated"] = false
	sess.Options.MaxAge = -1
	if err := sess.Save(c.Request(), c.Response()); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/admin/login")
}

func requireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sess := getSession(c)
		authed, _ := sess.Values["authenticated"].(bool)
		if !authed {
			return c.Redirect(http.StatusSeeOther, "/admin/login")
		}
		return next(c)
	}
}
