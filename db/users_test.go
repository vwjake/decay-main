package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestAuthenticate(t *testing.T) {
	conn := testDB(t)
	if _, err := CreateUser(conn, "jake", "Jake", "correct-horse-battery", RoleMaster); err != nil {
		t.Fatal(err)
	}

	user, err := Authenticate(conn, "jake", "correct-horse-battery")
	if err != nil {
		t.Fatalf("correct password rejected: %v", err)
	}
	if user.Username != "jake" || !user.Can(PermUsers) {
		t.Errorf("unexpected user %+v", user)
	}

	// Usernames are case-insensitive.
	if _, err := Authenticate(conn, "JAKE", "correct-horse-battery"); err != nil {
		t.Errorf("uppercase username rejected: %v", err)
	}

	// A wrong password and an unknown account are indistinguishable.
	if _, err := Authenticate(conn, "jake", "wrong"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("wrong password returned %v, want sql.ErrNoRows", err)
	}
	if _, err := Authenticate(conn, "nobody", "whatever"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("unknown user returned %v, want sql.ErrNoRows", err)
	}
}

func TestPasswordIsHashed(t *testing.T) {
	conn := testDB(t)
	const password = "plaintext-would-be-bad"
	if _, err := CreateUser(conn, "jake", "", password, RoleMaster); err != nil {
		t.Fatal(err)
	}

	var stored string
	if err := conn.QueryRow(`SELECT password_hash FROM users WHERE username = 'jake'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, password) {
		t.Error("the password is recoverable from the stored hash")
	}
	if !strings.HasPrefix(stored, "$2") {
		t.Errorf("stored value %q is not a bcrypt hash", stored)
	}
}

func TestCreateUserRejects(t *testing.T) {
	conn := testDB(t)
	if _, err := CreateUser(conn, "jake", "", "long-enough-password", RoleMaster); err != nil {
		t.Fatal(err)
	}

	if _, err := CreateUser(conn, "jake", "", "another-long-password", RoleMaster); !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("duplicate username returned %v, want ErrUsernameTaken", err)
	}
	// Case-insensitively duplicate, too.
	if _, err := CreateUser(conn, "JAKE", "", "another-long-password", RoleMaster); !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("case-variant duplicate returned %v, want ErrUsernameTaken", err)
	}
	if _, err := CreateUser(conn, "shorty", "", "short", RoleMaster); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("short password returned %v, want ErrPasswordTooShort", err)
	}
	if _, err := CreateUser(conn, "ghost", "", "long-enough-password", "wizard"); err == nil {
		t.Error("an unknown role was accepted")
	}
}

func TestSetPassword(t *testing.T) {
	conn := testDB(t)
	id, err := CreateUser(conn, "jake", "", "first-password-here", RoleMaster)
	if err != nil {
		t.Fatal(err)
	}

	if err := SetPassword(conn, id, "short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("short password returned %v, want ErrPasswordTooShort", err)
	}
	if err := SetPassword(conn, id, "second-password-here"); err != nil {
		t.Fatal(err)
	}
	if _, err := Authenticate(conn, "jake", "second-password-here"); err != nil {
		t.Errorf("new password rejected: %v", err)
	}
	if _, err := Authenticate(conn, "jake", "first-password-here"); !errors.Is(err, sql.ErrNoRows) {
		t.Error("the old password still works")
	}
}

func TestPermissions(t *testing.T) {
	if !Can(RoleMaster, PermUsers) {
		t.Error("master should be able to manage accounts")
	}
	for _, p := range AllPermissions {
		if !Can(RoleMaster, p) {
			t.Errorf("master is missing %q", p)
		}
	}
	// An unrecognised role must grant nothing rather than defaulting open.
	for _, p := range AllPermissions {
		if Can("retired-role", p) {
			t.Errorf("unknown role granted %q", p)
		}
	}
}

func TestEnsureFirstUserOnlyOnce(t *testing.T) {
	conn := testDB(t)

	created, err := EnsureFirstUser(conn, "admin", "bootstrap-password")
	if err != nil || !created {
		t.Fatalf("first bootstrap: created=%v err=%v", created, err)
	}

	// A second call must not create another account or reset the password,
	// so the env vars stop being live credentials after setup.
	created, err = EnsureFirstUser(conn, "admin", "a-different-password")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("bootstrap ran a second time")
	}
	users, err := ListUsers(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("got %d accounts, want 1", len(users))
	}
	if _, err := Authenticate(conn, "admin", "a-different-password"); !errors.Is(err, sql.ErrNoRows) {
		t.Error("changing ADMIN_PASSWORD changed the existing account's password")
	}
	if _, err := Authenticate(conn, "admin", "bootstrap-password"); err != nil {
		t.Errorf("original bootstrap password stopped working: %v", err)
	}
}

func TestCountUsersWith(t *testing.T) {
	conn := testDB(t)
	if _, err := CreateUser(conn, "one", "", "password-one-here", RoleMaster); err != nil {
		t.Fatal(err)
	}
	n, err := CountUsersWith(conn, PermUsers)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("got %d account managers, want 1", n)
	}

	if _, err := CreateUser(conn, "two", "", "password-two-here", RoleMaster); err != nil {
		t.Fatal(err)
	}
	if n, _ := CountUsersWith(conn, PermUsers); n != 2 {
		t.Errorf("got %d account managers, want 2", n)
	}
}
