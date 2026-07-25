package db

import (
	"path/filepath"
	"testing"
)

// TestPhotoGroupTagging covers tagging a photo to a group, retagging,
// clearing the tag, and PhotosForGroup returning only the tagged ones.
func TestPhotoGroupTagging(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	gid, err := CreateGroup(conn, Group{Slug: "no-tape", Name: "No Tape", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}

	// One photo tagged at upload, one untagged.
	if err := CreatePhoto(conn, "a.jpg", "At the jam", &gid); err != nil {
		t.Fatal(err)
	}
	if err := CreatePhoto(conn, "b.jpg", "Untagged", nil); err != nil {
		t.Fatal(err)
	}

	tagged, err := PhotosForGroup(conn, gid)
	if err != nil {
		t.Fatal(err)
	}
	if len(tagged) != 1 || tagged[0].Filename != "a.jpg" {
		t.Fatalf("PhotosForGroup = %+v, want just a.jpg", tagged)
	}
	if !tagged[0].InGroup(gid) {
		t.Error("InGroup false for a tagged photo")
	}

	// Tag the second photo via UpdatePhoto, then clear the first.
	all, err := ListPhotos(conn)
	if err != nil {
		t.Fatal(err)
	}
	var aID, bID int64
	for _, p := range all {
		switch p.Filename {
		case "a.jpg":
			aID = p.ID
		case "b.jpg":
			bID = p.ID
		}
	}
	if err := UpdatePhoto(conn, bID, "Now tagged", &gid); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePhoto(conn, aID, "At the jam", nil); err != nil {
		t.Fatal(err)
	}

	tagged, err = PhotosForGroup(conn, gid)
	if err != nil {
		t.Fatal(err)
	}
	if len(tagged) != 1 || tagged[0].Filename != "b.jpg" {
		t.Fatalf("after retag PhotosForGroup = %+v, want just b.jpg", tagged)
	}
}
