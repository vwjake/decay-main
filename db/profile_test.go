package db

import (
	"errors"
	"strings"
	"testing"
)

// TestSanitizeBlurbStripsMarkup is the "no executable code in the column"
// rule. The templates escape what they render, so this is the second lock:
// what's stored is text, whatever gets posted.
func TestSanitizeBlurbStripsMarkup(t *testing.T) {
	cases := map[string]string{
		// script and style go body and all; other tags leave their text.
		`<script>alert(1)</script>`:                "",
		`<SCRIPT>alert(1)</SCRIPT>`:                "",
		`<script src="evil.js"></script>after`:     "after",
		`before<script>alert(1)`:                   "before",
		`<strong>kept</strong>`:                    "kept",
		`<img src=x onerror=alert(1)>`:             "",
		`<a href="javascript:alert(1)">click</a>`:  "click",
		`hi <b>there</b>`:                          "hi there",
		"plain words stay":                         "plain words stay",
		"unclosed <tag that never ends":            "unclosed",
		"lone > bracket":                           "lone  bracket",
		"first\r\nsecond":                          "first\nsecond",
		"gaps\n\n\n\n\ncollapse":                   "gaps\n\ncollapse",
		"  trimmed  ":                              "trimmed",
		"null\x00and\x07bell":                      "nulland" + "bell",
		"emoji ✿ and accents café stay":            "emoji ✿ and accents café stay",
		"</p><svg/onload=alert(1)>done":            "done",
		"<style>body{display:none}</style>visible": "visible",
		"<!-- comment -->kept":                     "kept",
		"a < b and c > d":                          "a  d",
	}
	for in, want := range cases {
		if got := SanitizeBlurb(in); got != want {
			t.Errorf("SanitizeBlurb(%q) = %q, want %q", in, got, want)
		}
	}

	// Nothing that survives can reopen a tag.
	for _, in := range []string{`<script>x</script>`, `<<script>>`, `<img src=x>`} {
		got := SanitizeBlurb(in)
		if strings.ContainsAny(got, "<>") {
			t.Errorf("SanitizeBlurb(%q) = %q, still carries a bracket", in, got)
		}
	}
}

func TestProfileRoundTrip(t *testing.T) {
	conn := testDB(t)
	id, err := CreateUser(conn, "keys", "", "long-enough-password", RoleKeyholder)
	if err != nil {
		t.Fatal(err)
	}

	if err := UpdateProfile(conn, id, "Key Holder", "  opens the place <b>early</b>  "); err != nil {
		t.Fatal(err)
	}
	user, err := UserByID(conn, id)
	if err != nil {
		t.Fatal(err)
	}
	if user.DisplayName != "Key Holder" || user.Name() != "Key Holder" {
		t.Errorf("display name = %q", user.DisplayName)
	}
	if user.Blurb != "opens the place early" {
		t.Errorf("blurb = %q, want the sanitized, trimmed text", user.Blurb)
	}
	if !user.HasBlurb() {
		t.Error("HasBlurb should be true")
	}

	// A blurb over the cap is refused rather than quietly cut short.
	long := strings.Repeat("x", MaxBlurbLength+1)
	if err := UpdateProfile(conn, id, "Key Holder", long); !errors.Is(err, ErrBlurbTooLong) {
		t.Errorf("over-long blurb returned %v, want ErrBlurbTooLong", err)
	}
	if err := SetBlurb(conn, id, long); !errors.Is(err, ErrBlurbTooLong) {
		t.Errorf("SetBlurb(over-long) returned %v, want ErrBlurbTooLong", err)
	}
	if user, _ := UserByID(conn, id); user.Blurb != "opens the place early" {
		t.Errorf("a rejected blurb overwrote the stored one: %q", user.Blurb)
	}

	// Markup that sanitizes down to nothing is under the cap, not over it.
	if err := SetBlurb(conn, id, "<p>"+strings.Repeat("y", MaxBlurbLength)+"</p>"); err != nil {
		t.Errorf("tag-wrapped blurb at the cap was refused: %v", err)
	}
}

func TestUserPhoto(t *testing.T) {
	conn := testDB(t)
	id, err := CreateUser(conn, "snap", "", "long-enough-password", RoleManager)
	if err != nil {
		t.Fatal(err)
	}

	user, _ := UserByID(conn, id)
	if user.HasPhoto() {
		t.Error("a new account should have no photo")
	}

	previous, err := SetUserPhoto(conn, id, "123.png")
	if err != nil {
		t.Fatal(err)
	}
	if previous != "" {
		t.Errorf("first upload replaced %q", previous)
	}
	user, _ = UserByID(conn, id)
	if !user.HasPhoto() {
		t.Fatal("photo wasn't saved")
	}
	// Pages serve the web-sized JPEG copy, whatever was uploaded.
	if want := "/uploads/avatars/web/123.jpg"; user.PhotoPath() != want {
		t.Errorf("PhotoPath() = %q, want %q", user.PhotoPath(), want)
	}

	// Replacing hands back the old filename so its files can be deleted.
	previous, err = SetUserPhoto(conn, id, "456.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if previous != "123.png" {
		t.Errorf("replaced photo reported as %q, want 123.png", previous)
	}

	photo, err := DeleteUser(conn, id)
	if err != nil {
		t.Fatal(err)
	}
	if photo != "456.jpg" {
		t.Errorf("deleted account reported photo %q, want 456.jpg", photo)
	}
}
