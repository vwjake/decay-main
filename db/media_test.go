package db

import (
	"path/filepath"
	"testing"
)

func TestParseYouTubeID(t *testing.T) {
	ok := map[string]string{
		"dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ":            "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ":                           "dQw4w9WgXcQ",
		"https://www.youtube.com/embed/dQw4w9WgXcQ":              "dQw4w9WgXcQ",
		"https://youtube.com/shorts/dQw4w9WgXcQ":                 "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=42s":      "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?list=PLabc&v=dQw4w9WgXcQ": "dQw4w9WgXcQ",
	}
	for in, want := range ok {
		got, valid := ParseYouTubeID(in)
		if !valid || got != want {
			t.Errorf("ParseYouTubeID(%q) = %q, %v; want %q", in, got, valid, want)
		}
	}

	for _, bad := range []string{"", "not a url", "https://vimeo.com/12345", "abc", "https://www.youtube.com/@no_tape"} {
		if got, valid := ParseYouTubeID(bad); valid {
			t.Errorf("ParseYouTubeID(%q) = %q, valid; want invalid", bad, got)
		}
	}
}

func TestVideoRoundTrip(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := CreateVideo(conn, Video{YouTubeID: "bbbbbbbbbbb", Title: "Second", Position: 2}); err != nil {
		t.Fatal(err)
	}
	first, err := CreateVideo(conn, Video{YouTubeID: "aaaaaaaaaaa", Title: "First", Position: 1})
	if err != nil {
		t.Fatal(err)
	}

	list, err := ListVideos(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Title != "First" {
		t.Fatalf("order wrong: %+v", list)
	}
	if list[0].EmbedURL() != "https://www.youtube-nocookie.com/embed/aaaaaaaaaaa" {
		t.Errorf("EmbedURL = %q", list[0].EmbedURL())
	}

	v, _ := VideoByID(conn, first)
	v.Title = "Renamed"
	if err := UpdateVideo(conn, v); err != nil {
		t.Fatal(err)
	}
	if got, _ := VideoByID(conn, first); got.Title != "Renamed" {
		t.Errorf("update failed: %q", got.Title)
	}

	if err := DeleteVideo(conn, first); err != nil {
		t.Fatal(err)
	}
	if list, _ := ListVideos(conn); len(list) != 1 {
		t.Errorf("after delete = %d, want 1", len(list))
	}
}
