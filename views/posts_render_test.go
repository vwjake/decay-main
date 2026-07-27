package views

import (
	"context"
	"io"
	"strings"
	"testing"

	"decay-main/db"
)

// TestPostAdminViewsRender exercises the blog admin list and edit screens,
// and pins the Markdown toolbar onto both body fields — the convenience the
// edit/create forms depend on app.js to wire up.
func TestPostAdminViewsRender(t *testing.T) {
	me := db.User{Username: "smoke", Role: db.RoleMaster}
	posts := []db.Post{
		{ID: 1, Slug: "hello", Title: "Hello", Body: "# Hi\n\nBody."},
		{ID: 2, Slug: "draft", Title: "A draft"}, // unpublished
	}

	var list strings.Builder
	if err := AdminPosts(posts, me, "").Render(context.Background(), &list); err != nil {
		t.Fatalf("AdminPosts render: %v", err)
	}
	// The list offers an explicit edit link and a toolbar on the new-post body.
	if !strings.Contains(list.String(), "/admin/posts/1") {
		t.Error("posts list is missing an edit link to a post")
	}
	if !strings.Contains(list.String(), `data-md="bold"`) {
		t.Error("new-post form is missing the Markdown toolbar")
	}

	var edit strings.Builder
	if err := AdminPostEdit(posts[0], me, "").Render(context.Background(), &edit); err != nil {
		t.Fatalf("AdminPostEdit render: %v", err)
	}
	if !strings.Contains(edit.String(), `data-md="bold"`) {
		t.Error("edit form is missing the Markdown toolbar")
	}
	// The existing body must survive into the textarea for editing.
	if !strings.Contains(edit.String(), "# Hi") {
		t.Error("edit form didn't render the post body into the textarea")
	}

	if err := AdminPostEdit(posts[0], me, "Oops.").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminPostEdit error-state render: %v", err)
	}
}
