package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestPostBySlug covers the lookup behind /blog/<slug>: a published post
// is found, a draft is not, and a missing slug reports ErrNoRows.
func TestPostBySlug(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := CreatePost(conn, "live-one", "Live One", "Body text.", true); err != nil {
		t.Fatal(err)
	}
	if err := CreatePost(conn, "draft-one", "Draft One", "Draft body.", false); err != nil {
		t.Fatal(err)
	}

	post, err := PostBySlug(conn, "live-one")
	if err != nil {
		t.Fatalf("published post not found: %v", err)
	}
	if post.Title != "Live One" {
		t.Errorf("got title %q, want %q", post.Title, "Live One")
	}
	if !post.Published() {
		t.Error("published post reports itself as a draft")
	}

	if _, err := PostBySlug(conn, "draft-one"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("draft lookup returned %v, want sql.ErrNoRows", err)
	}
	if _, err := PostBySlug(conn, "nope"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("missing slug returned %v, want sql.ErrNoRows", err)
	}
}

// TestPostBySlugAfterPublish covers the path the admin panel actually
// takes: a post is created as a draft and published afterwards.
func TestPostBySlugAfterPublish(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := CreatePost(conn, "later", "Later", "Body.", false); err != nil {
		t.Fatal(err)
	}
	posts, err := ListAllPosts(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 {
		t.Fatalf("got %d posts, want 1", len(posts))
	}
	if err := PublishPost(conn, posts[0].ID); err != nil {
		t.Fatal(err)
	}

	post, err := PostBySlug(conn, "later")
	if err != nil {
		t.Fatalf("post not found after publishing: %v", err)
	}
	if post.PublishedAt == nil || time.Since(*post.PublishedAt) > time.Hour {
		t.Errorf("unexpected published_at: %v", post.PublishedAt)
	}
}
