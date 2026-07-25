package db

import (
	"database/sql"
	"regexp"
	"strings"
)

// Video is one embedded YouTube video in the media section.
type Video struct {
	ID        int64
	YouTubeID string
	Title     string
	Position  int
}

// EmbedURL is the privacy-preserving embed URL — youtube-nocookie doesn't
// set tracking cookies until the viewer actually plays the video.
func (v Video) EmbedURL() string {
	return "https://www.youtube-nocookie.com/embed/" + v.YouTubeID
}

// WatchURL links out to the video on YouTube.
func (v Video) WatchURL() string {
	return "https://www.youtube.com/watch?v=" + v.YouTubeID
}

// ThumbURL is the still thumbnail, used in the admin list so the page
// doesn't load a live player per video.
func (v Video) ThumbURL() string {
	return "https://img.youtube.com/vi/" + v.YouTubeID + "/mqdefault.jpg"
}

var (
	// A YouTube id is exactly 11 URL-safe characters.
	youtubeIDPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	youtubeURLPattern = regexp.MustCompile(`(?:v=|/embed/|/shorts/|youtu\.be/)([A-Za-z0-9_-]{11})`)
)

// ParseYouTubeID pulls the video id out of a pasted URL — watch, youtu.be,
// embed, or shorts form — or accepts a bare id. Returns ok=false when the
// input has no recognisable id, so the admin form can reject it.
func ParseYouTubeID(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if youtubeIDPattern.MatchString(raw) {
		return raw, true
	}
	if m := youtubeURLPattern.FindStringSubmatch(raw); m != nil {
		return m[1], true
	}
	return "", false
}

const videoColumns = `id, youtube_id, title, position`

func scanVideos(rows *sql.Rows) ([]Video, error) {
	var videos []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.ID, &v.YouTubeID, &v.Title, &v.Position); err != nil {
			return nil, err
		}
		videos = append(videos, v)
	}
	return videos, rows.Err()
}

// ListVideos returns the media videos in display order.
func ListVideos(conn *sql.DB) ([]Video, error) {
	rows, err := conn.Query(`SELECT ` + videoColumns + ` FROM media_videos ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVideos(rows)
}

// VideoByID fetches one video for editing.
func VideoByID(conn *sql.DB, id int64) (Video, error) {
	rows, err := conn.Query(`SELECT `+videoColumns+` FROM media_videos WHERE id = ?`, id)
	if err != nil {
		return Video{}, err
	}
	defer rows.Close()
	videos, err := scanVideos(rows)
	if err != nil {
		return Video{}, err
	}
	if len(videos) == 0 {
		return Video{}, sql.ErrNoRows
	}
	return videos[0], nil
}

// CreateVideo adds a video and returns its new id.
func CreateVideo(conn *sql.DB, v Video) (int64, error) {
	res, err := conn.Exec(
		`INSERT INTO media_videos (youtube_id, title, position) VALUES (?, ?, ?)`,
		v.YouTubeID, v.Title, v.Position,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateVideo saves edits to a video's id, title, and order.
func UpdateVideo(conn *sql.DB, v Video) error {
	_, err := conn.Exec(
		`UPDATE media_videos SET youtube_id = ?, title = ?, position = ? WHERE id = ?`,
		v.YouTubeID, v.Title, v.Position, v.ID,
	)
	return err
}

// DeleteVideo removes a video.
func DeleteVideo(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM media_videos WHERE id = ?`, id)
	return err
}
