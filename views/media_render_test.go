package views

import (
	"context"
	"io"
	"strings"
	"testing"

	"decay-main/db"
)

func TestMediaViewsRender(t *testing.T) {
	me := db.User{Username: "smoke", Role: db.RoleMaster}
	videos := []db.Video{
		{ID: 1, YouTubeID: "dQw4w9WgXcQ", Title: "Live set", Position: 1},
		{ID: 2, YouTubeID: "aaaaaaaaaaa", Position: 2}, // no title
	}

	// Home renders with and without videos.
	if err := Home(nil, nil, videos).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("Home with videos: %v", err)
	}
	if err := Home(nil, nil, nil).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("Home no videos: %v", err)
	}

	// The embed uses the nocookie host, not the tracking one.
	var buf strings.Builder
	if err := Home(nil, nil, videos).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "youtube-nocookie.com/embed/dQw4w9WgXcQ") {
		t.Error("embed does not use youtube-nocookie host")
	}

	if err := AdminMedia(videos, me, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminMedia: %v", err)
	}
	if err := AdminMedia(nil, me, "Bad link.").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminMedia empty: %v", err)
	}
}
