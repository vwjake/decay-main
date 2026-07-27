package db

import (
	"database/sql"
	"path"
	"strings"
)

// PostImagesSubdir keeps blog images apart from flyers, gallery photos, and
// product shots inside the uploads directory.
const PostImagesSubdir = "blog"

// PostImage is one image uploaded for a blog post. It's referenced from the
// post body by URL; the row exists so the edit page can list it and so the
// file is cleaned up with the post.
type PostImage struct {
	ID       int64
	PostID   int64
	Filename string
}

// Path is the URL of the full-size original.
func (p PostImage) Path() string { return "/uploads/" + PostImagesSubdir + "/" + p.Filename }

// WebPath is the web-sized copy — the one worth putting in a post body and
// showing in the edit-page strip.
func (p PostImage) WebPath() string {
	return "/uploads/" + PostImagesSubdir + "/web/" + strings.TrimSuffix(p.Filename, path.Ext(p.Filename)) + ".jpg"
}

// Markdown is the snippet the edit page inserts into the body to show this
// image. Alt text is left blank for the author to fill in.
func (p PostImage) Markdown() string { return "![](" + p.WebPath() + ")" }

// ListPostImages returns a post's images, newest first.
func ListPostImages(conn *sql.DB, postID int64) ([]PostImage, error) {
	rows, err := conn.Query(
		`SELECT id, post_id, filename FROM post_images WHERE post_id = ? ORDER BY id DESC`,
		postID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var images []PostImage
	for rows.Next() {
		var img PostImage
		if err := rows.Scan(&img.ID, &img.PostID, &img.Filename); err != nil {
			return nil, err
		}
		images = append(images, img)
	}
	return images, rows.Err()
}

// CreatePostImage records an uploaded image against a post.
func CreatePostImage(conn *sql.DB, postID int64, filename string) error {
	_, err := conn.Exec(
		`INSERT INTO post_images (post_id, filename) VALUES (?, ?)`,
		postID, filename,
	)
	return err
}

// DeletePostImage removes one image row and returns its filename so the
// caller can also delete the file. It's scoped to the post so a mismatched
// id can't delete another post's image.
func DeletePostImage(conn *sql.DB, postID, imageID int64) (string, error) {
	var filename string
	err := conn.QueryRow(
		`SELECT filename FROM post_images WHERE id = ? AND post_id = ?`,
		imageID, postID,
	).Scan(&filename)
	if err != nil {
		return "", err
	}
	if _, err := conn.Exec(`DELETE FROM post_images WHERE id = ?`, imageID); err != nil {
		return "", err
	}
	return filename, nil
}
