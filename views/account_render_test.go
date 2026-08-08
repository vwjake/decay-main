package views

import (
	"context"
	"io"
	"strings"
	"testing"

	"decay-main/db"
)

// TestAccountViewsRender exercises the account templates with a bare
// account and a filled-in one, catching runtime template failures a
// type-check wouldn't.
func TestAccountViewsRender(t *testing.T) {
	bare := db.User{ID: 1, Username: "keys", Role: db.RoleKeyholder}
	filled := db.User{
		ID: 2, Username: "moon", DisplayName: "Moon Fery", Role: db.RoleManager,
		Photo: "1.jpg", Blurb: "sound person.\nhere most nights.",
	}

	for _, me := range []db.User{bare, filled} {
		if err := AdminAccount(me, AccountPage{}).Render(context.Background(), io.Discard); err != nil {
			t.Fatalf("AdminAccount(%s) render: %v", me.Username, err)
		}
	}
	page := AccountPage{Saved: true, ProfileError: "too long", PasswordError: "wrong"}
	if err := AdminAccount(filled, page).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminAccount with flashes: %v", err)
	}

	users := []db.User{bare, filled}
	if err := AdminUsers(users, filled, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminUsers render: %v", err)
	}
	if err := AdminUserEdit(bare, filled, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminUserEdit render: %v", err)
	}
}

// TestBlurbIsEscaped is the render-side half of the "no executable code"
// rule: db.SanitizeBlurb keeps markup out of the column, and this keeps
// anything already in there from reaching the browser as markup.
func TestBlurbIsEscaped(t *testing.T) {
	// A blurb straight from the column, as if it predated the sanitizer.
	u := db.User{ID: 1, Username: "x", Role: db.RoleKeyholder, Blurb: `<script>alert(1)</script>`}

	var out strings.Builder
	if err := ProfileCard(u).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "<script>") {
		t.Errorf("a blurb reached the page as markup:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "&lt;script&gt;") {
		t.Errorf("expected the blurb escaped, got:\n%s", out.String())
	}
}

// TestMasterRoleIsNotOfferedInForms checks the templates read the actor's
// assignable roles rather than every role there is — the accounts page is
// where a hidden role would leak.
func TestMasterRoleIsNotOfferedInForms(t *testing.T) {
	manager := db.User{ID: 1, Username: "mgr", Role: db.RoleManager}
	keyholder := db.User{ID: 2, Username: "keys", Role: db.RoleKeyholder}

	for name, render := range map[string]func(io.Writer) error{
		"AdminUsers": func(w io.Writer) error {
			return AdminUsers([]db.User{manager, keyholder}, manager, "").Render(context.Background(), w)
		},
		"AdminUserEdit": func(w io.Writer) error {
			return AdminUserEdit(keyholder, manager, "").Render(context.Background(), w)
		},
	} {
		var out strings.Builder
		if err := render(&out); err != nil {
			t.Fatalf("%s render: %v", name, err)
		}
		if strings.Contains(out.String(), `value="master"`) || strings.Contains(out.String(), "Master") {
			t.Errorf("%s offered the master role to a manager:\n%s", name, out.String())
		}
	}

	// A master still gets the full set.
	master := db.User{ID: 3, Username: "root", Role: db.RoleMaster}
	var out strings.Builder
	if err := AdminUsers([]db.User{master}, master, "").Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `value="master"`) {
		t.Error("a master wasn't offered the master role")
	}
}
