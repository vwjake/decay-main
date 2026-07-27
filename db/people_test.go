package db

import (
	"path/filepath"
	"testing"
)

// TestPeopleOrdering checks profiles come back by position then id, and
// that create/update/delete round-trip.
func TestPeopleOrdering(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := Seed(conn); err != nil {
		t.Fatal(err)
	}

	// Seeding fills in the five board members at positions 0–4.
	seeded, err := ListPeople(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeded) != 5 || seeded[0].Name != "Moon Fery" {
		t.Fatalf("seed = %d people, first %q", len(seeded), firstName(seeded))
	}

	// A staff member added at a higher position sorts after the board.
	id, err := CreatePerson(conn, Person{Name: "Sam Staff", Role: "Coordinator", Bio: "Runs events.", Position: 10})
	if err != nil {
		t.Fatal(err)
	}
	people, err := ListPeople(conn)
	if err != nil {
		t.Fatal(err)
	}
	if got := people[len(people)-1].Name; got != "Sam Staff" {
		t.Errorf("last person = %q, want Sam Staff", got)
	}

	// Update moves them to the front.
	p, err := PersonByID(conn, id)
	if err != nil {
		t.Fatal(err)
	}
	p.Position = -1
	if err := UpdatePerson(conn, p); err != nil {
		t.Fatal(err)
	}
	people, _ = ListPeople(conn)
	if people[0].Name != "Sam Staff" {
		t.Errorf("after reorder first = %q, want Sam Staff", people[0].Name)
	}

	// Delete returns the (empty) photo name and drops the row.
	if _, err := DeletePerson(conn, id); err != nil {
		t.Fatal(err)
	}
	people, _ = ListPeople(conn)
	if len(people) != 5 {
		t.Errorf("after delete = %d people, want 5", len(people))
	}
}

func firstName(people []Person) string {
	if len(people) == 0 {
		return ""
	}
	return people[0].Name
}
