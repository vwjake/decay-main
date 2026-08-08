package db

import (
	"database/sql"
	"path"
	"regexp"
	"strings"
)

// GroupsSubdir keeps group hero images apart from the other uploads.
const GroupsSubdir = "groups"

// Group is a recurring meetup with its own detail page.
type Group struct {
	ID          int64
	Slug        string
	Name        string
	Summary     string
	Description string
	// Pills are short tags shown under the title, one per line as stored.
	Pills string
	// HeroImage is a filename under uploads/groups/, empty when there's none.
	HeroImage string
	HeroAlt   string
	// Body is the titled sections in the small markup ParseBody understands.
	Body string
	// MatchTerms are the terms (one per line) that tie events to this group.
	MatchTerms string
	Category   string
	Position   int
	Enabled    bool
}

// MatchTermList splits the stored terms, dropping blanks.
func (g Group) MatchTermList() []string {
	var out []string
	for _, line := range strings.Split(g.MatchTerms, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func (g Group) HasHero() bool { return g.HeroImage != "" }

// HeroPath is the web-sized copy the page displays.
func (g Group) HeroPath() string {
	return "/uploads/" + GroupsSubdir + "/web/" + strings.TrimSuffix(g.HeroImage, path.Ext(g.HeroImage)) + ".jpg"
}

// Path is the group's public URL.
func (g Group) Path() string { return "/groups/" + g.Slug }

// PillList splits the stored pills into individual tags, dropping blanks.
func (g Group) PillList() []string {
	var out []string
	for _, line := range strings.Split(g.Pills, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// Sections parses the body into blocks a template can render.
func (g Group) Sections() []GroupBlock { return ParseBody(g.Body) }

// GroupBlock is one titled section of a group's page: a heading with either
// bullet items, body paragraphs, or both.
type GroupBlock struct {
	Heading    string
	Bullets    [][]Segment
	Paragraphs [][]Segment
}

// Segment is a run of a line that's either plain text or a link. Splitting
// the text this way lets the template render links without ever handing raw
// HTML to the browser.
type Segment struct {
	Text string
	URL  string // non-empty means render Text as a link to URL
}

func (s Segment) IsLink() bool { return s.URL != "" }

var urlPattern = regexp.MustCompile(`https?://[^\s]+`)

// parseSegments splits a line into plain and link runs, autolinking bare
// URLs. Trailing sentence punctuation is kept out of the link target.
func parseSegments(line string) []Segment {
	locs := urlPattern.FindAllStringIndex(line, -1)
	if locs == nil {
		return []Segment{{Text: line}}
	}
	var segs []Segment
	last := 0
	for _, loc := range locs {
		if loc[0] > last {
			segs = append(segs, Segment{Text: line[last:loc[0]]})
		}
		url := line[loc[0]:loc[1]]
		// Don't swallow punctuation that's really part of the sentence.
		trimmed := strings.TrimRight(url, ".,);:!?")
		if tail := url[len(trimmed):]; tail != "" {
			segs = append(segs, Segment{Text: trimmed, URL: trimmed})
			segs = append(segs, Segment{Text: tail})
		} else {
			segs = append(segs, Segment{Text: url, URL: url})
		}
		last = loc[1]
	}
	if last < len(line) {
		segs = append(segs, Segment{Text: line[last:]})
	}
	return segs
}

// ParseBody turns the stored body markup into renderable blocks. "# Title"
// opens a section; "- item" adds a bullet; any other non-blank line is a
// paragraph. Content before the first heading still renders, headingless.
func ParseBody(body string) []GroupBlock {
	var blocks []GroupBlock
	var cur *GroupBlock
	ensure := func() *GroupBlock {
		if cur == nil {
			blocks = append(blocks, GroupBlock{})
			cur = &blocks[len(blocks)-1]
		}
		return cur
	}

	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "# "):
			blocks = append(blocks, GroupBlock{Heading: strings.TrimSpace(line[2:])})
			cur = &blocks[len(blocks)-1]
		case strings.HasPrefix(line, "- "):
			b := ensure()
			b.Bullets = append(b.Bullets, parseSegments(strings.TrimSpace(line[2:])))
		default:
			b := ensure()
			b.Paragraphs = append(b.Paragraphs, parseSegments(line))
		}
	}
	return blocks
}

const groupColumns = `id, slug, name, summary, description, pills, hero_image, hero_alt, body, match_terms, category, position, enabled`

func scanGroups(rows *sql.Rows) ([]Group, error) {
	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Slug, &g.Name, &g.Summary, &g.Description, &g.Pills, &g.HeroImage, &g.HeroAlt, &g.Body, &g.MatchTerms, &g.Category, &g.Position, &g.Enabled); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// normalizeMatch lowercases and turns underscores and hyphens into spaces,
// so a term like "No Tape" matches an event titled "NO_TAPE".
func normalizeMatch(s string) string {
	return strings.NewReplacer("_", " ", "-", " ").Replace(strings.ToLower(s))
}

// UpcomingForGroup returns up to limit upcoming events that belong to the
// group, soonest first. A term matches when it appears in an event's title
// or type. A group with no terms matches nothing — its page just shows no
// schedule rather than the whole calendar.
func UpcomingForGroup(conn *sql.DB, g Group, limit int) ([]Event, error) {
	terms := g.MatchTermList()
	if len(terms) == 0 {
		return nil, nil
	}
	for i, t := range terms {
		terms[i] = normalizeMatch(t)
	}

	upcoming, err := UpcomingEvents(conn)
	if err != nil {
		return nil, err
	}
	var out []Event
	for _, ev := range upcoming {
		hay := normalizeMatch(ev.Title + " " + ev.EventType)
		for _, t := range terms {
			if strings.Contains(hay, t) {
				out = append(out, ev)
				break
			}
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ListGroups returns every group in display order, for admin management.
func ListGroups(conn *sql.DB) ([]Group, error) {
	rows, err := conn.Query(`SELECT ` + groupColumns + ` FROM groups ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroups(rows)
}

// EnabledGroups returns just the groups shown to the public, in order.
func EnabledGroups(conn *sql.DB) ([]Group, error) {
	rows, err := conn.Query(`SELECT ` + groupColumns + ` FROM groups WHERE enabled = 1 ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroups(rows)
}

// GroupBySlug fetches one group for its public page. A disabled group is
// reported as missing so an unlisted slug can't be reached directly.
func GroupBySlug(conn *sql.DB, slug string) (Group, error) {
	rows, err := conn.Query(`SELECT `+groupColumns+` FROM groups WHERE slug = ? AND enabled = 1`, slug)
	if err != nil {
		return Group{}, err
	}
	defer rows.Close()
	groups, err := scanGroups(rows)
	if err != nil {
		return Group{}, err
	}
	if len(groups) == 0 {
		return Group{}, sql.ErrNoRows
	}
	return groups[0], nil
}

// GroupByID fetches one group for the admin panel, enabled or not.
func GroupByID(conn *sql.DB, id int64) (Group, error) {
	rows, err := conn.Query(`SELECT `+groupColumns+` FROM groups WHERE id = ?`, id)
	if err != nil {
		return Group{}, err
	}
	defer rows.Close()
	groups, err := scanGroups(rows)
	if err != nil {
		return Group{}, err
	}
	if len(groups) == 0 {
		return Group{}, sql.ErrNoRows
	}
	return groups[0], nil
}

// CreateGroup inserts a group and returns its new id.
func CreateGroup(conn *sql.DB, g Group) (int64, error) {
	res, err := conn.Exec(
		`INSERT INTO groups (slug, name, summary, description, pills, hero_alt, body, match_terms, category, position, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.Slug, g.Name, g.Summary, g.Description, g.Pills, g.HeroAlt, g.Body, g.MatchTerms, g.Category, g.Position, g.Enabled,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateGroup saves edits to a group's text fields. The hero image is set
// separately through SetGroupHero.
func UpdateGroup(conn *sql.DB, g Group) error {
	_, err := conn.Exec(
		`UPDATE groups SET slug = ?, name = ?, summary = ?, description = ?, pills = ?, hero_alt = ?, body = ?, match_terms = ?, category = ?, position = ?, enabled = ? WHERE id = ?`,
		g.Slug, g.Name, g.Summary, g.Description, g.Pills, g.HeroAlt, g.Body, g.MatchTerms, g.Category, g.Position, g.Enabled, g.ID,
	)
	return err
}

// SetGroupHero points a group at an uploaded hero image, returning the
// filename it replaced so the caller can delete it.
func SetGroupHero(conn *sql.DB, id int64, filename string) (string, error) {
	var previous string
	if err := conn.QueryRow(`SELECT hero_image FROM groups WHERE id = ?`, id).Scan(&previous); err != nil {
		return "", err
	}
	if _, err := conn.Exec(`UPDATE groups SET hero_image = ? WHERE id = ?`, filename, id); err != nil {
		return "", err
	}
	return previous, nil
}

// GroupSlugTaken reports whether a slug is already used by another group.
func GroupSlugTaken(conn *sql.DB, slug string, exceptID int64) (bool, error) {
	var count int
	err := conn.QueryRow(`SELECT count(*) FROM groups WHERE slug = ? AND id <> ?`, slug, exceptID).Scan(&count)
	return count > 0, err
}

// DeleteGroup removes the row and returns its hero filename so the caller
// can also remove the file from disk.
func DeleteGroup(conn *sql.DB, id int64) (string, error) {
	var hero string
	if err := conn.QueryRow(`SELECT hero_image FROM groups WHERE id = ?`, id).Scan(&hero); err != nil {
		return "", err
	}
	if _, err := conn.Exec(`DELETE FROM groups WHERE id = ?`, id); err != nil {
		return "", err
	}
	return hero, nil
}
