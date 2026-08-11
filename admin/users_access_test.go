package admin

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"decay-main/bookingmail"
	"decay-main/db"
	"decay-main/mail"

	"github.com/labstack/echo/v4"
)

// panelFor boots the whole /admin route tree against a throwaway database
// and returns a client already signed in as one of the accounts. Going
// through login rather than faking a session means these tests exercise
// the same middleware a browser would.
func panelFor(t *testing.T, signInAs string) (*http.Client, string, map[string]db.User) {
	t.Helper()

	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	accounts := map[string]db.User{}
	for _, role := range []string{db.RoleMaster, db.RoleManager, db.RoleKeyholder} {
		id, err := db.CreateUser(conn, role+"-user", "", "long-enough-password", role)
		if err != nil {
			t.Fatal(err)
		}
		u, err := db.UserByID(conn, id)
		if err != nil {
			t.Fatal(err)
		}
		accounts[role] = u
	}

	e := echo.New()
	Register(e, conn, []byte("test-session-secret-000000000000"), t.TempDir(),
		time.UTC, "", bookingmail.New(bookingmail.Config{}), false, mail.FromEnv(), "http://localhost:8080")
	server := httptest.NewServer(e)
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	res, err := client.PostForm(server.URL+"/admin/login", url.Values{
		"username": {signInAs + "-user"},
		"password": {"long-enough-password"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("signing in as %s: status %d", signInAs, res.StatusCode)
	}
	return client, server.URL, accounts
}

func get(t *testing.T, client *http.Client, url string) (int, string) {
	t.Helper()
	res, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(body)
}

func post(t *testing.T, client *http.Client, url string, form url.Values) (int, string) {
	t.Helper()
	res, err := client.PostForm(url, form)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(body)
}

// TestManagerCannotSeeMasters is the whole point of the hidden role: a
// manager runs the accounts page without ever learning a master exists.
func TestManagerCannotSeeMasters(t *testing.T) {
	client, base, accounts := panelFor(t, db.RoleManager)
	master := accounts[db.RoleMaster]

	status, body := get(t, client, base+"/admin/users")
	if status != http.StatusOK {
		t.Fatalf("a manager got %d from the accounts page", status)
	}
	if strings.Contains(body, master.Username) {
		t.Error("the accounts list showed a master account to a manager")
	}
	if strings.Contains(body, "Master") {
		t.Error("the accounts page mentioned the master role to a manager")
	}
	for _, want := range []string{"manager-user", "keyholder-user"} {
		if !strings.Contains(body, want) {
			t.Errorf("the accounts list is missing %s", want)
		}
	}

	// Every route that takes an :id must hide the account, not merely
	// leave it out of the list. A 403 would confirm it's there.
	masterPath := base + "/admin/users/" + strconv.FormatInt(master.ID, 10)
	if status, _ := get(t, client, masterPath); status != http.StatusNotFound {
		t.Errorf("GET a master's edit page returned %d, want 404", status)
	}
	for _, path := range []string{masterPath, masterPath + "/password", masterPath + "/delete", masterPath + "/photo/delete"} {
		if status, _ := post(t, client, path, url.Values{"role": {db.RoleKeyholder}, "password": {"another-long-password"}}); status != http.StatusNotFound {
			t.Errorf("POST %s returned %d, want 404", path, status)
		}
	}

	// And a manager can't mint one, on either form.
	status, _ = post(t, client, base+"/admin/users", url.Values{
		"username": {"sneaky"}, "password": {"long-enough-password"}, "role": {db.RoleMaster},
	})
	if status != http.StatusOK { // the form re-renders with an error
		t.Errorf("creating a master returned %d", status)
	}
	if _, body := get(t, client, base+"/admin/users"); strings.Contains(body, "sneaky") {
		t.Error("a manager created a master account")
	}

	keyholderPath := base + "/admin/users/" + strconv.FormatInt(accounts[db.RoleKeyholder].ID, 10)
	if status, _ := post(t, client, keyholderPath, url.Values{"role": {db.RoleMaster}}); status != http.StatusOK {
		t.Errorf("promoting to master returned %d", status)
	}
	if _, body := get(t, client, keyholderPath); !strings.Contains(body, "Keyholder") {
		t.Error("a manager promoted an account to master")
	}
}

// TestMasterSeesEverything is the other half: the role is hidden, not
// absent.
func TestMasterSeesEverything(t *testing.T) {
	client, base, _ := panelFor(t, db.RoleMaster)
	status, body := get(t, client, base+"/admin/users")
	if status != http.StatusOK {
		t.Fatalf("a master got %d from the accounts page", status)
	}
	for _, want := range []string{"master-user", "manager-user", "keyholder-user", `value="master"`} {
		if !strings.Contains(body, want) {
			t.Errorf("a master's accounts page is missing %q", want)
		}
	}
}

// TestKeyholderAccess pins what the narrow role can and can't reach, and
// that it still has an account page of its own.
func TestKeyholderAccess(t *testing.T) {
	client, base, _ := panelFor(t, db.RoleKeyholder)

	if status, _ := get(t, client, base+"/admin/users"); status != http.StatusForbidden {
		t.Errorf("a keyholder got %d from the accounts page, want 403", status)
	}
	if status, _ := get(t, client, base+"/admin/events"); status != http.StatusOK {
		t.Errorf("a keyholder got %d from the events page, want 200", status)
	}
	if status, _ := get(t, client, base+"/admin/products"); status != http.StatusForbidden {
		t.Errorf("a keyholder got %d from the shop page, want 403", status)
	}

	status, body := get(t, client, base+"/admin/account")
	if status != http.StatusOK {
		t.Fatalf("a keyholder got %d from their own account page", status)
	}
	if strings.Contains(body, `href="/admin/users"`) {
		t.Error("the nav offered a keyholder the accounts page")
	}
}

// TestAccountPageSavesSanitizedBlurb walks the account page the way its
// owner would: write a blurb, get it back stripped of markup.
func TestAccountPageSavesSanitizedBlurb(t *testing.T) {
	client, base, _ := panelFor(t, db.RoleKeyholder)

	if status, _ := post(t, client, base+"/admin/account", url.Values{
		"display_name": {"Key Holder"},
		"blurb":        {`opens up <script>alert(1)</script> most <b>nights</b>`},
	}); status != http.StatusOK {
		t.Fatalf("saving the profile returned %d", status)
	}

	status, body := get(t, client, base+"/admin/account")
	if status != http.StatusOK {
		t.Fatalf("account page returned %d", status)
	}
	if !strings.Contains(body, "Key Holder") {
		t.Error("the display name didn't save")
	}
	if !strings.Contains(body, "opens up  most nights") {
		t.Errorf("the sanitized blurb isn't on the page:\n%s", body)
	}
	if strings.Contains(body, "<script>alert(1)</script>") || strings.Contains(body, "alert(1)") {
		t.Error("script content survived into the page")
	}

	// Over the cap is refused, and says so.
	if _, body := post(t, client, base+"/admin/account", url.Values{
		"blurb": {strings.Repeat("x", db.MaxBlurbLength+1)},
	}); !strings.Contains(body, db.ErrBlurbTooLong.Error()) {
		t.Error("an over-long blurb was accepted without complaint")
	}
}
