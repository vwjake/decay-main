package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"decay-main/db"
	"decay-main/images"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

func registerUserRoutes(g *echo.Group, conn *sql.DB, uploadsDir string) {
	// Managing accounts is its own permission, so a keyholder can run the
	// calendar without being able to hand out access.
	users := g.Group("/users", requirePermission(db.PermUsers))
	users.GET("", listUsers(conn))
	users.POST("", createUser(conn))
	users.GET("/:id", editUser(conn))
	users.POST("/:id", saveUser(conn))
	users.POST("/:id/password", resetPassword(conn))
	users.POST("/:id/photo/delete", removeUserPhoto(conn, uploadsDir))
	users.POST("/:id/delete", deleteUser(conn, uploadsDir))
}

func listUsers(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		return renderUsers(c, conn, "")
	}
}

func createUser(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		username := c.FormValue("username")
		password := c.FormValue("password")
		role := c.FormValue("role")
		if role == "" {
			role = db.RoleKeyholder
		}
		// Nobody can create an account in a role they can't see, so a
		// manager can't quietly mint a master.
		if !currentUser(c).CanSee(db.User{Role: role}) {
			return renderUsers(c, conn, "Unknown role.")
		}

		_, err := db.CreateUser(conn, username, c.FormValue("display_name"), password, role)
		switch {
		case errors.Is(err, db.ErrUsernameTaken):
			return renderUsers(c, conn, "That username is already taken.")
		case errors.Is(err, db.ErrPasswordTooShort):
			return renderUsers(c, conn, db.ErrPasswordTooShort.Error()+".")
		case err != nil:
			return renderUsers(c, conn, "Couldn't create that account: "+err.Error())
		}
		return c.Redirect(http.StatusSeeOther, "/admin/users")
	}
}

func editUser(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, err := loadUser(c, conn)
		if err != nil {
			return err
		}
		return views.AdminUserEdit(user, currentUser(c), "").Render(c.Request().Context(), c.Response())
	}
}

func saveUser(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, err := loadUser(c, conn)
		if err != nil {
			return err
		}
		me := currentUser(c)

		rerender := func(msg string) error {
			return views.AdminUserEdit(user, me, msg).Render(c.Request().Context(), c.Response())
		}

		role := c.FormValue("role")
		if _, ok := db.Roles[role]; !ok || !me.CanSee(db.User{Role: role}) {
			return rerender("Unknown role.")
		}

		// Taking the last account-manager's access away would lock
		// everyone out of ever granting it again.
		if user.Can(db.PermUsers) && !db.Can(role, db.PermUsers) {
			remaining, err := db.CountUsersWith(conn, db.PermUsers)
			if err != nil {
				return err
			}
			if remaining <= 1 {
				return rerender("This is the only account that can manage accounts — give another account that access first.")
			}
		}

		// The blurb belongs to the account, but an account manager can
		// clear or fix one rather than having to ask its owner to. Checked
		// before anything is written, so a too-long one doesn't half-save.
		if err := db.SetBlurb(conn, user.ID, c.FormValue("blurb")); err != nil {
			if errors.Is(err, db.ErrBlurbTooLong) {
				return rerender(db.ErrBlurbTooLong.Error() + ".")
			}
			return err
		}
		if err := db.UpdateUser(conn, user.ID, c.FormValue("display_name"), role); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/users")
	}
}

func resetPassword(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, err := loadUser(c, conn)
		if err != nil {
			return err
		}
		if err := db.SetPassword(conn, user.ID, c.FormValue("password")); err != nil {
			if errors.Is(err, db.ErrPasswordTooShort) {
				return views.AdminUserEdit(user, currentUser(c), db.ErrPasswordTooShort.Error()+".").
					Render(c.Request().Context(), c.Response())
			}
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/users")
	}
}

// removeUserPhoto takes an avatar down without touching the rest of the
// account — the moderation half of a profile anyone can upload to.
func removeUserPhoto(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, err := loadUser(c, conn)
		if err != nil {
			return err
		}
		if err := clearAvatar(conn, uploadsDir, user.ID); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/users/"+strconv.FormatInt(user.ID, 10))
	}
}

func deleteUser(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, err := loadUser(c, conn)
		if err != nil {
			return err
		}

		// Deleting yourself logs you out mid-request, and deleting the
		// last account-manager locks the panel for good.
		if user.ID == currentUser(c).ID {
			return renderUsers(c, conn, "You can't delete the account you're signed in as.")
		}
		if user.Can(db.PermUsers) {
			remaining, err := db.CountUsersWith(conn, db.PermUsers)
			if err != nil {
				return err
			}
			if remaining <= 1 {
				return renderUsers(c, conn, "That's the only account that can manage accounts.")
			}
		}

		photo, err := db.DeleteUser(conn, user.ID)
		if err != nil {
			return err
		}
		removeAvatarFiles(uploadsDir, photo)
		return c.Redirect(http.StatusSeeOther, "/admin/users")
	}
}

func renderUsers(c echo.Context, conn *sql.DB, msg string) error {
	me := currentUser(c)
	all, err := db.ListUsers(conn)
	if err != nil {
		return err
	}
	// Master accounts are invisible to anyone who isn't one, so the list a
	// manager sees simply doesn't contain them.
	visible := make([]db.User, 0, len(all))
	for _, u := range all {
		if me.CanSee(u) {
			visible = append(visible, u)
		}
	}
	return views.AdminUsers(visible, me, msg).Render(c.Request().Context(), c.Response())
}

// loadUser reads the account a /admin/users/:id route is about. An account
// the signed-in user isn't allowed to see is a 404 rather than a 403 —
// telling a manager "forbidden" would confirm a master exists at that id,
// which is the whole thing being hidden.
func loadUser(c echo.Context, conn *sql.DB) (db.User, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return db.User{}, echo.NewHTTPError(http.StatusBadRequest)
	}
	user, err := db.UserByID(conn, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.User{}, echo.NewHTTPError(http.StatusNotFound)
	}
	if err != nil {
		return db.User{}, err
	}
	if !currentUser(c).CanSee(user) {
		return db.User{}, echo.NewHTTPError(http.StatusNotFound)
	}
	return user, nil
}

// clearAvatar drops an account's photo and the files behind it.
func clearAvatar(conn *sql.DB, uploadsDir string, id int64) error {
	previous, err := db.SetUserPhoto(conn, id, "")
	if err != nil {
		return err
	}
	removeAvatarFiles(uploadsDir, previous)
	return nil
}

// removeAvatarFiles deletes an avatar and its web copy, ignoring files
// that are already gone.
func removeAvatarFiles(uploadsDir, filename string) {
	if filename == "" {
		return
	}
	dir := filepath.Join(uploadsDir, db.AvatarsSubdir)
	_ = os.Remove(filepath.Join(dir, filename))
	_ = os.Remove(filepath.Join(dir, "web", images.WebName(filename)))
}
