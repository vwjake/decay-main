package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"

	"decay-main/db"
	"decay-main/images"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

// registerAccountRoutes wires up /admin/account — your own profile. It's
// deliberately behind no permission at all: every signed-in account has
// one, including a keyholder who can reach nothing else in the panel.
func registerAccountRoutes(g *echo.Group, conn *sql.DB, uploadsDir string) {
	g.GET("/account", showAccount)
	g.POST("/account", saveAccount(conn))
	g.POST("/account/photo", uploadAccountPhoto(conn, uploadsDir))
	g.POST("/account/photo/delete", deleteAccountPhoto(conn, uploadsDir))
	g.POST("/account/password", changeOwnPassword(conn))
}

func showAccount(c echo.Context) error {
	return renderAccount(c, currentUser(c), "", "")
}

func saveAccount(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		me := currentUser(c)
		blurb := c.FormValue("blurb")
		if err := db.UpdateProfile(conn, me.ID, c.FormValue("display_name"), blurb); err != nil {
			if errors.Is(err, db.ErrBlurbTooLong) {
				// Hand back what they typed rather than the stored blurb,
				// so a rejected edit isn't lost.
				me.DisplayName = c.FormValue("display_name")
				me.Blurb = blurb
				return renderAccount(c, me, db.ErrBlurbTooLong.Error()+".", "")
			}
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/account?saved=1")
	}
}

func uploadAccountPhoto(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		me := currentUser(c)

		fileHeader, err := c.FormFile("photo")
		if err != nil {
			return renderAccount(c, me, "Choose an image to upload.", "")
		}
		dir := filepath.Join(uploadsDir, db.AvatarsSubdir)
		filename, err := saveImage(fileHeader, dir)
		if err != nil {
			if errors.Is(err, errNotAnImage) {
				return renderAccount(c, me, "That file doesn't look like an image. Use jpg, png, gif, or webp.", "")
			}
			return err
		}
		if err := images.MakeWeb(
			filepath.Join(dir, filename),
			filepath.Join(dir, "web", images.WebName(filename)),
		); err != nil {
			return err
		}

		previous, err := db.SetUserPhoto(conn, me.ID, filename)
		if err != nil {
			return err
		}
		if previous != filename {
			removeAvatarFiles(uploadsDir, previous)
		}
		return c.Redirect(http.StatusSeeOther, "/admin/account?saved=1")
	}
}

func deleteAccountPhoto(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := clearAvatar(conn, uploadsDir, currentUser(c).ID); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/account?saved=1")
	}
}

// changeOwnPassword is how an account without the users permission changes
// its own password. The current password is required: a session left open
// on a shared machine shouldn't be enough to lock its owner out.
func changeOwnPassword(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		me := currentUser(c)
		fail := func(msg string) error { return renderAccount(c, me, "", msg) }

		if _, err := db.Authenticate(conn, me.Username, c.FormValue("current_password")); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fail("That's not your current password.")
			}
			return err
		}
		if err := db.SetPassword(conn, me.ID, c.FormValue("password")); err != nil {
			if errors.Is(err, db.ErrPasswordTooShort) {
				return fail(db.ErrPasswordTooShort.Error() + ".")
			}
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/account?saved=1")
	}
}

func renderAccount(c echo.Context, me db.User, profileErr, passwordErr string) error {
	page := views.AccountPage{
		Saved:         c.QueryParam("saved") != "",
		ProfileError:  profileErr,
		PasswordError: passwordErr,
	}
	return views.AdminAccount(me, page).Render(c.Request().Context(), c.Response())
}
