package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"decay-main/db"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

func registerUserRoutes(g *echo.Group, conn *sql.DB) {
	// Managing accounts is its own permission, so a future narrower role
	// can edit content without being able to hand out access.
	users := g.Group("/users", requirePermission(db.PermUsers))
	users.GET("", listUsers(conn))
	users.POST("", createUser(conn))
	users.GET("/:id", editUser(conn))
	users.POST("/:id", saveUser(conn))
	users.POST("/:id/password", resetPassword(conn))
	users.POST("/:id/delete", deleteUser(conn))
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
			role = db.RoleMaster
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
		user, err := loadUser(conn, c.Param("id"))
		if err != nil {
			return err
		}
		return views.AdminUserEdit(user, currentUser(c), "").Render(c.Request().Context(), c.Response())
	}
}

func saveUser(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, err := loadUser(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			return views.AdminUserEdit(user, currentUser(c), msg).Render(c.Request().Context(), c.Response())
		}

		role := c.FormValue("role")
		if _, ok := db.Roles[role]; !ok {
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

		if err := db.UpdateUser(conn, user.ID, c.FormValue("display_name"), role); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/users")
	}
}

func resetPassword(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, err := loadUser(conn, c.Param("id"))
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

func deleteUser(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, err := loadUser(conn, c.Param("id"))
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

		if err := db.DeleteUser(conn, user.ID); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/users")
	}
}

func renderUsers(c echo.Context, conn *sql.DB, msg string) error {
	users, err := db.ListUsers(conn)
	if err != nil {
		return err
	}
	return views.AdminUsers(users, currentUser(c), msg).Render(c.Request().Context(), c.Response())
}

func loadUser(conn *sql.DB, rawID string) (db.User, error) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return db.User{}, echo.NewHTTPError(http.StatusBadRequest)
	}
	user, err := db.UserByID(conn, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.User{}, echo.NewHTTPError(http.StatusNotFound)
	}
	return user, err
}
