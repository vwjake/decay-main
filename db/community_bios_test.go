package db

import (
	"path/filepath"
	"testing"
)

func TestCommunityBios(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Two public (out of order), one kept on file but not shown.
	if _, err := CreateCommunityBio(conn, CommunityBio{Name: "Bo", Role: "DJ", Public: true, Position: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateCommunityBio(conn, CommunityBio{Name: "Ada", Pronouns: "she/her", Bio: "First para.\n\nSecond para.", Public: true, Position: 1}); err != nil {
		t.Fatal(err)
	}
	hiddenID, err := CreateCommunityBio(conn, CommunityBio{Name: "Cy", Public: false})
	if err != nil {
		t.Fatal(err)
	}

	// Admin list has all three; public list drops the non-public one, ordered.
	all, err := ListCommunityBios(conn)
	if err != nil || len(all) != 3 {
		t.Fatalf("ListCommunityBios = %d (%v), want 3", len(all), err)
	}
	pub, err := PublicCommunityBios(conn)
	if err != nil || len(pub) != 2 {
		t.Fatalf("PublicCommunityBios = %d (%v), want 2", len(pub), err)
	}
	if pub[0].Name != "Ada" || pub[1].Name != "Bo" {
		t.Errorf("public order = %q, %q; want Ada, Bo", pub[0].Name, pub[1].Name)
	}
	// Bio splits into paragraphs on blank lines.
	if got := pub[0].Paragraphs(); len(got) != 2 {
		t.Errorf("Paragraphs = %v, want 2 blocks", got)
	}

	// Publishing the hidden bio brings it onto the public list.
	b, err := CommunityBioByID(conn, hiddenID)
	if err != nil {
		t.Fatal(err)
	}
	b.Public = true
	if err := UpdateCommunityBio(conn, b); err != nil {
		t.Fatal(err)
	}
	if pub, _ := PublicCommunityBios(conn); len(pub) != 3 {
		t.Errorf("after publish, public = %d, want 3", len(pub))
	}

	// Delete removes it.
	if err := DeleteCommunityBio(conn, hiddenID); err != nil {
		t.Fatal(err)
	}
	if all, _ := ListCommunityBios(conn); len(all) != 2 {
		t.Errorf("after delete, all = %d, want 2", len(all))
	}
}
