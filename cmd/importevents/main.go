// Command importevents converts the old PHP site's flat event JSON files
// into db/events.json, the seed data this site ships with.
//
// The old site (github.com/vwjake/decay) is still live and still the place
// events are entered, so this is meant to be re-run whenever its archive
// moves forward:
//
//	go run ./cmd/importevents -src ../decay/data/events/data
//
// It only rewrites db/events.json. Getting that into a database that has
// already been seeded means deleting decay.db and letting it seed again —
// see the README.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"decay-main/db"

	// The old records store wall-clock dates with no zone, so resolving
	// them needs the Olympia zone. Windows has no system tzdata.
	_ "time/tzdata"
)

// oldEvent covers only the fields worth carrying over. The source records
// have ~40 more (volunteer slots, financials, post-event reports) that
// belong to the old admin workflow, not to a public listing.
type oldEvent struct {
	Title       string `json:"title"`
	Date        string `json:"date"`     // 2026-04-21
	Time        string `json:"time"`     // 14:00
	EndTime     string `json:"end_time"` // 17:00, may be before Time
	Location    string `json:"location"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Performers  string `json:"performers"`
	Private     bool   `json:"private"`
	Flyer       string `json:"flyer"`
	// volunteers maps a role to {is_needed, volunteer}, but a batch of
	// older records store an empty array per role instead, so each slot
	// is decoded on its own and a mismatch is treated as "not needed".
	Volunteers map[string]json.RawMessage `json:"volunteers"`
	// CalDAVUID is the identity the old site uses when it pushes an event
	// to Nextcloud, so it's the one key that survives a round trip.
	CalDAVUID string `json:"caldav_uid"`
	// program is a string in older records and an array in newer ones.
	Program json.RawMessage `json:"program"`
	Links   []string        `json:"links"`
}

// SeedEvent is one row of db/events.json.
type SeedEvent struct {
	Source      string          `json:"source"`
	UID         string          `json:"uid"`
	Title       string          `json:"title"`
	EventType   string          `json:"event_type"`
	StartsAt    string          `json:"starts_at"`
	EndsAt      string          `json:"ends_at,omitempty"`
	Location    string          `json:"location"`
	Description string          `json:"description,omitempty"`
	Link        string          `json:"link,omitempty"`
	Slug        string          `json:"slug"`
	Flyer       string          `json:"flyer,omitempty"`
	Volunteers  []SeedVolunteer `json:"volunteers,omitempty"`
}

// SeedVolunteer is one role an event needed covered. Name is empty when
// the slot was never filled. Only the name comes across — the old records
// also hold volunteers' email and phone, which the new site has no use
// for and shouldn't be storing.
type SeedVolunteer struct {
	Role string `json:"role"`
	Name string `json:"name,omitempty"`
}

const venueAddress = "402 Washington St NE, Olympia WA"

// eventFile matches the old site's naming, MMDDYY-slug.json. Anything
// else in the directory (draft `entry-*.json`, the archive/ and backup/
// subdirectories) is not a live event.
var eventFile = regexp.MustCompile(`^\d{6}-.+\.json$`)

// programNames maps the old site's program slugs to the group names used
// in its site-content.json.
var programNames = map[string]string{
	"open-draw":          "Open Draw",
	"no-tape":            "No Tape",
	"decentralized-tech": "Decentralized Tech",
	"movie-club":         "Movie Club",
	"mutual-aid":         "Mutual Aid",
	"live-show":          "Live Show",
	"workshop":           "Workshop",
}

// categoryNames normalizes the free-text category field, which was typed
// by hand and so varies in case and spelling.
var categoryNames = map[string]string{
	"workshop":             "Workshop",
	"wrorkshop":            "Workshop",
	"drawing":              "Drawing",
	"meetup":               "Meetup",
	"film":                 "Film",
	"meeting":              "Meeting",
	"tech":                 "Tech",
	"music":                "Music",
	"activism":             "Activism",
	"live act/dance night": "Live Act",
}

func main() {
	src := flag.String("src", filepath.Join("..", "decay", "data", "events", "data"), "old site's live data/events/data directory")
	archive := flag.String("archive", filepath.Join("..", "decay", "data", "archive"), "old site's data/archive tree of past quarters (\"\" to skip)")
	out := flag.String("out", filepath.Join("db", "events.json"), "seed file to write")
	flyersOut := flag.String("flyers-out", filepath.Join("uploads", "flyers"), "where to copy flyer images (\"\" to skip)")
	flag.Parse()

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		log.Fatal(err)
	}

	// The live directory holds only the current window; once a quarter
	// ends its events are moved under data/archive/<year>/<quarter>/. Both
	// have to be read to get the whole history, and the live copy of a
	// file wins because it's the one still being edited.
	byName := map[string]SeedEvent{}
	skipped := 0

	archived, err := collect(*archive, loc, byName, &skipped)
	if err != nil {
		log.Fatal(err)
	}
	live, err := collect(*src, loc, byName, &skipped)
	if err != nil {
		log.Fatal(err)
	}

	events := make([]SeedEvent, 0, len(byName))
	for _, ev := range byName {
		events = append(events, ev)
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].StartsAt != events[j].StartsAt {
			return events[i].StartsAt < events[j].StartsAt
		}
		return events[i].Source < events[j].Source
	})

	// Slugs are unique per event, and sorting first means the suffix an
	// event gets is stable from one import to the next.
	taken := map[string]bool{}
	for i := range events {
		base := events[i].Slug
		for n := 2; taken[events[i].Slug]; n++ {
			events[i].Slug = fmt.Sprintf("%s-%d", base, n)
		}
		taken[events[i].Slug] = true
	}

	copied, err := copyFlyers(events, *src, *archive, *flyersOut)
	if err != nil {
		log.Fatal(err)
	}

	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d events to %s (%d archived, %d live, %d skipped)\n",
		len(events), *out, archived, live, skipped)
	fmt.Printf("copied %d flyer images to %s\n", copied, *flyersOut)
}

// copyFlyers finds each referenced flyer image in the old site's upload
// pools and copies it to dest. Recurring events share a flyer, so the
// same file is only copied once. An event whose image has gone missing
// has its flyer reference cleared rather than left pointing at nothing.
func copyFlyers(events []SeedEvent, src, archive, dest string) (int, error) {
	if dest == "" {
		return 0, nil
	}

	// Full-size uploads first, then each quarter's flyers directory.
	var pools []string
	if src != "" {
		pools = append(pools, filepath.Join(filepath.Dir(src), "uploads"))
	}
	if archive != "" {
		matches, err := filepath.Glob(filepath.Join(archive, "*", "Q*", "flyers"))
		if err != nil {
			return 0, err
		}
		sort.Strings(matches)
		pools = append(pools, matches...)
	}

	index := map[string]string{}
	for _, pool := range pools {
		entries, err := os.ReadDir(pool)
		if err != nil {
			continue // a quarter without flyers is normal
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if _, ok := index[entry.Name()]; !ok {
				index[entry.Name()] = filepath.Join(pool, entry.Name())
			}
		}
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return 0, err
	}

	copied := 0
	done := map[string]bool{}
	for i := range events {
		name := events[i].Flyer
		if name == "" {
			continue
		}
		from, ok := index[name]
		if !ok {
			log.Printf("flyer not found, dropping reference: %s (%s)", name, events[i].Source)
			events[i].Flyer = ""
			continue
		}
		if done[name] {
			continue
		}
		done[name] = true
		if err := copyFile(from, filepath.Join(dest, name)); err != nil {
			return copied, err
		}
		copied++
	}
	return copied, nil
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(to)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// collect walks a directory tree of old event records into byName, keyed
// by filename so a later call overwrites an earlier one's duplicates. It
// returns how many records it read.
func collect(dir string, loc *time.Location, byName map[string]SeedEvent, skipped *int) (int, error) {
	if dir == "" {
		return 0, nil
	}
	read := 0
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			// backup/ holds stale copies and test junk ("060666-666.json"),
			// not events anyone ran.
			if entry.Name() == "backup" {
				return fs.SkipDir
			}
			return nil
		}
		if !eventFile.MatchString(entry.Name()) {
			return nil
		}
		read++
		ev, err := convert(path, entry.Name(), loc)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if ev == nil {
			*skipped++
			return nil
		}
		byName[entry.Name()] = *ev
		return nil
	})
	return read, err
}

// convert reads one old record. It returns nil for records that can't be
// shown publicly: private events, and the half-filled drafts the old
// submission form leaves behind with no title or no date.
func convert(path, name string, loc *time.Location) (*SeedEvent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var old oldEvent
	if err := json.Unmarshal(raw, &old); err != nil {
		return nil, err
	}

	title := strings.TrimSpace(old.Title)
	if old.Private || title == "" || old.Date == "" {
		return nil, nil
	}

	clock := old.Time
	if clock == "" {
		clock = "00:00"
	}
	start, err := time.ParseInLocation("2006-01-02 15:04", old.Date+" "+clock, loc)
	if err != nil {
		return nil, fmt.Errorf("start time: %w", err)
	}

	var endsAt string
	if old.EndTime != "" {
		end, err := time.ParseInLocation("2006-01-02 15:04", old.Date+" "+old.EndTime, loc)
		if err != nil {
			return nil, fmt.Errorf("end time: %w", err)
		}
		// The old form stores a bare clock time, so a late show ending
		// at 01:00 lands before its own start. Those run past midnight.
		if !end.After(start) {
			end = end.AddDate(0, 0, 1)
		}
		endsAt = end.Format(time.RFC3339)
	}

	return &SeedEvent{
		Source:      name,
		UID:         uid(old, name),
		Title:       title,
		EventType:   eventType(old),
		StartsAt:    start.Format(time.RFC3339),
		EndsAt:      endsAt,
		Location:    location(old.Location),
		Description: description(old),
		Link:        firstLink(old.Links),
		Slug:        db.Slug(start, title),
		Flyer:       strings.TrimSpace(old.Flyer),
		Volunteers:  volunteers(old),
	}, nil
}

// volunteers flattens the old record's role map, keeping only roles that
// were actually asked for and only the volunteer's name.
func volunteers(old oldEvent) []SeedVolunteer {
	var out []SeedVolunteer
	for _, role := range db.VolunteerRoles {
		raw, ok := old.Volunteers[role]
		if !ok {
			continue
		}
		var info struct {
			IsNeeded  bool `json:"is_needed"`
			Volunteer *struct {
				Name string `json:"name"`
			} `json:"volunteer"`
		}
		if err := json.Unmarshal(raw, &info); err != nil {
			continue // the empty-array form: nothing recorded
		}
		name := ""
		if info.Volunteer != nil {
			name = strings.TrimSpace(info.Volunteer.Name)
		}
		// A filled slot implies the role was needed, even if the flag
		// was later cleared.
		if !info.IsNeeded && name == "" {
			continue
		}
		out = append(out, SeedVolunteer{Role: role, Name: name})
	}
	return out
}

// uid returns the event's calendar identity. Events the old site already
// pushed to Nextcloud have one, and reusing it means calendars already
// subscribed there don't end up with a second copy. The rest get a UID
// derived from their filename rather than a random one, so re-running
// this importer doesn't churn UIDs out from under subscribers.
func uid(old oldEvent, name string) string {
	if u := strings.TrimSpace(old.CalDAVUID); u != "" {
		return u
	}
	sum := sha256.Sum256([]byte("decay-event:" + name))
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// eventType prefers the hand-entered category and falls back to the
// program a recurring event belongs to.
func eventType(old oldEvent) string {
	if c := strings.TrimSpace(old.Category); c != "" {
		if name, ok := categoryNames[strings.ToLower(c)]; ok {
			return name
		}
		return titleCase(c)
	}
	for _, slug := range programs(old.Program) {
		if name, ok := programNames[slug]; ok {
			return name
		}
	}
	return "Event"
}

// titleCase capitalizes each word of an unrecognized category so a stray
// spelling still reads like the mapped ones.
func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// programs reads the program field, which older records store as a single
// slug string and newer ones as an array of them.
func programs(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil && single != "" {
		return []string{single}
	}
	return nil
}

func location(raw string) string {
	trimmed := strings.TrimSpace(raw)
	switch strings.ToLower(trimmed) {
	case "", "decay", "402 washington st ne olympia, wa":
		return venueAddress
	}
	return trimmed
}

// description joins the blurb with the performer list, which the old site
// stored separately and rendered as its own line.
func description(old oldEvent) string {
	body := strings.ReplaceAll(strings.TrimSpace(old.Description), "\r\n", "\n")
	if p := strings.TrimSpace(old.Performers); p != "" {
		if body != "" {
			body += "\n\n"
		}
		body += p
	}
	return body
}

func firstLink(links []string) string {
	for _, l := range links {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}
