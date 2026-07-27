package db

import (
	"path/filepath"
	"testing"
)

func TestNormalizeFormURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"  https://cloud.decay.events/apps/forms/s/abc  ", "https://cloud.decay.events/apps/forms/s/abc", true},
		{"http://example.com/form", "http://example.com/form", true},
		{"", "", false},
		{"not a url", "", false},
		{"ftp://example.com/x", "", false},
		{"/apps/forms/s/abc", "", false}, // relative, no host
	}
	for _, c := range cases {
		got, ok := NormalizeFormURL(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("NormalizeFormURL(%q) = %q,%v; want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestExternalForms(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Two enabled (out of order), one disabled.
	if _, err := CreateExternalForm(conn, ExternalForm{Title: "Impact", URL: "https://x/impact", Position: 2, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateExternalForm(conn, ExternalForm{Title: "Diversity", URL: "https://x/div", Position: 1, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	hiddenID, err := CreateExternalForm(conn, ExternalForm{Title: "Draft", URL: "https://x/draft", Position: 0, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}

	// Admin list has all three; public list drops the disabled one.
	all, err := ListExternalForms(conn)
	if err != nil || len(all) != 3 {
		t.Fatalf("ListExternalForms = %d (%v), want 3", len(all), err)
	}
	pub, err := EnabledExternalForms(conn)
	if err != nil || len(pub) != 2 {
		t.Fatalf("EnabledExternalForms = %d (%v), want 2", len(pub), err)
	}
	// Ordered by position: Diversity (1) before Impact (2).
	if pub[0].Title != "Diversity" || pub[1].Title != "Impact" {
		t.Errorf("public order = %q, %q; want Diversity, Impact", pub[0].Title, pub[1].Title)
	}

	// Enabling the hidden form brings it into the public list.
	f, err := ExternalFormByID(conn, hiddenID)
	if err != nil {
		t.Fatal(err)
	}
	f.Enabled = true
	if err := UpdateExternalForm(conn, f); err != nil {
		t.Fatal(err)
	}
	if pub, _ := EnabledExternalForms(conn); len(pub) != 3 {
		t.Errorf("after enable, public = %d, want 3", len(pub))
	}

	// Delete removes it.
	if err := DeleteExternalForm(conn, hiddenID); err != nil {
		t.Fatal(err)
	}
	if all, _ := ListExternalForms(conn); len(all) != 2 {
		t.Errorf("after delete, all = %d, want 2", len(all))
	}
}
