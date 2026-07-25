package views

import (
	"context"
	"io"
	"testing"

	"decay-main/db"
)

func TestGroupViewsRender(t *testing.T) {
	groups := []db.Group{
		{ID: 1, Slug: "open-draw", Name: "Open Draw", Summary: "Casual drawing hang.",
			Description: "Bring your tools.", Pills: "Drop-in\nAll levels", Enabled: true,
			Body: "# What to bring\n- Paper\n- Pens\n\n# New here?\nSay hi. Discord: https://discord.gg/abc."},
		{ID: 2, Slug: "no-hero", Name: "No Hero", Summary: "No image, no pills, no body.", Enabled: true},
	}

	if err := Groups(groups).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("Groups render: %v", err)
	}
	for _, g := range groups {
		if err := GroupPage(g).Render(context.Background(), io.Discard); err != nil {
			t.Fatalf("GroupPage(%s) render: %v", g.Slug, err)
		}
	}
}

func TestAdminGroupViewsRender(t *testing.T) {
	me := db.User{Username: "smoke", Role: db.RoleMaster}
	groups := []db.Group{
		{ID: 1, Slug: "open-draw", Name: "Open Draw", Enabled: true, Position: 0},
		{ID: 2, Slug: "mutual-aid", Name: "Mutual Aid", Enabled: false, Position: 4},
	}
	if err := AdminGroups(groups, me, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminGroups render: %v", err)
	}
	// Edit page for both an enabled and a hidden group (checkbox state).
	for _, g := range groups {
		if err := AdminGroupEdit(g, me, "").Render(context.Background(), io.Discard); err != nil {
			t.Fatalf("AdminGroupEdit(%s) render: %v", g.Slug, err)
		}
	}
}
