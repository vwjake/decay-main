package db

import "database/sql"

// groupSeed is one group carried over from the old site's site-content.json.
// The nested "sections" and "notes" become the small body markup ParseBody
// reads: "# Heading" per section, "- item" per bullet, plain lines as
// paragraphs.
type groupSeed struct {
	slug, name, summary, description, pills, body, matchTerms string
	enabled                                                   bool
}

var groupSeeds = []groupSeed{
	{
		slug:        "open-draw",
		name:        "Open Draw",
		summary:     "Casual drawing hang for artists of every level. Bring your own supplies or use what's available, share prompts, and get feedback in a low-pressure setting.",
		description: "Open Draw is a casual drawing hang for artists of every level. Bring your own tools or use what’s on hand, swap prompts, and work alongside other people who like to make things.",
		pills:       "Drop-in friendly\nAll skill levels\nQuiet & social corners",
		matchTerms:  "Open Draw",
		enabled:     true,
		body: `# What to bring
- Sketchbook or loose paper
- Pens, pencils, markers, or any portable medium you enjoy
- Works-in-progress you’d like feedback on (optional)

# What to expect
- Laid-back, drop-in friendly environment
- Informal prompts for anyone who wants inspiration
- Peer feedback without pressure

# Want to join?
- Show up, say hi, and grab a seat
- Ask around if you need materials—someone usually has extras
- Stay for a quick sketch or the whole session

# New here?
Bring a friend or come solo—either way you’ll find someone to trade ideas with. If you prefer quiet, find a tucked-away corner; if you want to collaborate, ask for prompts and jump in.`,
	},
	{
		slug:        "no-tape",
		name:        "No Tape",
		summary:     "Open practice and jam space for musicians to improvise, test ideas, and collaborate without the pressure of a formal show.",
		description: "No Tape is an open practice space for musicians to improvise, collaborate, and workshop songs without the pressure of a formal show. Audio-video synthesis and stage lights are also practiced and experimented with.",
		pills:       "Collaborative jams\nAmplified & acoustic\nListeners welcome",
		matchTerms:  "No Tape",
		enabled:     true,
		body: `# Bring along
- Instruments, pedals, laptops, or other gear you want to experiment with
- Your own cables if you can
- Ideas you’re willing to try out with new collaborators

# The vibe
- Rotating lineups and spontaneous pairings
- Room for feedback, experimentation, and learning
- All skill levels welcome—listening is participation, too

# How to join
- Drop in with your gear and introduce yourself
- Plug in when there's space and ask for a match-up if you want one
- Share what you're experimenting with so others can riff with you
- Join the Discord: https://discord.gg/bEYhdenj4t

# New here?
Let folks know if you’re trying collaborative jams for the first time. Someone can help you set levels, find a partner, or just listen in until you’re ready to play.`,
	},
	{
		slug:        "decentralized-tech",
		name:        "Decentralized Tech",
		summary:     "Hands-on meetups for people interested in peer-to-peer tools, self-hosting, and resilient community technology.",
		description: "Decentralized Tech meetups explore peer-to-peer tools, self-hosting, and resilient community technology through demos and collaborative tinkering.",
		pills:       "Hands-on demos\nPeer learning\nBeginner friendly",
		matchTerms:  "Decentralized Tech",
		enabled:     true,
		body: `# Topics we cover
- Mesh networking, offline-first tools, and local-first software
- Running your own services, from chat to media hosting
- Security basics and consentful tech practices

# What to bring
- Laptop or single-board computer if you want to hack along
- Questions about projects you’re working on
- A collaborative mindset—no prior experience required

# How to participate
- Show up with an idea to demo or just listen in
- Split into small groups based on interest and float between them
- Pair up with someone to explore a tool side-by-side
- Join the Signal chat: https://signal.group/#CjQKIHNC7--A2MWzgwBOcdrfoKNP3WJvtB-m7CNwRXzMDKUbEhA8tkWd3ZazJHUbcilp0sGE
- We also self-host a message board at chat.decay.events

# New here?
Tell us what you want to learn or share—we’ll point you toward a table that matches your interest. If you’d rather observe, that’s welcome too.`,
	},
	{
		slug:        "movie-club",
		name:        "Movie Club",
		summary:     "Collective film nights with rotating hosts. Expect themed lineups, post-screening chats, and an easy-going vibe.",
		description: "Movie Club gathers people to watch films together, swap recommendations, and talk about what we just saw. Expect rotating hosts and themes.",
		pills:       "Rotating curators\nSnacks encouraged\nDiscussion after",
		matchTerms:  "Movie Club",
		enabled:     true,
		body: `# How screenings work
- Rotating curators set the lineup and handle playback
- Short intros to frame the film and content notices
- Casual discussion and snacks after the credits

# What to bring
- Comfortable clothing and a friend
- Snacks or non-alcoholic drinks to share (optional)
- Film suggestions for future nights

# Participate
- Arrive a few minutes early to settle in
- Offer a quick intro if you picked the movie
- Share your thoughts during the discussion or just listen in

# New here?
Movie Club is low-pressure. If you’re shy, sit near the back and enjoy. If you’re excited to host a future night, mention it during discussion and we’ll help with setup.`,
	},
	{
		slug:        "mutual-aid",
		name:        "Mutual Aid",
		summary:     "Neighbors supporting neighbors through skill-sharing, resource swaps, and responsive community care projects.",
		description: "Mutual Aid at DECAY connects neighbors who want to support one another through skill-sharing, resource swaps, and rapid response when needs come up.",
		pills:       "Community care\nResource swaps\nSkill sharing",
		matchTerms:  "Mutual Aid",
		// Disabled on the old site; kept unlisted until it's ready to run.
		enabled: false,
		body: `# How we work
- Share needs and offers openly so the group can coordinate
- Pool supplies for pop-up distributions and care kits
- Organize rides, childcare swaps, and other practical help

# Bring or request
- Extra supplies like hygiene items, shelf-stable food, or weather gear
- Skills such as translation, repair, cooking, or digital help
- Any immediate needs you’re comfortable sharing with the group

# Participate
- Show up ready to listen and collaborate
- Introduce yourself and ask how to plug into the next action
- Pair up with someone to learn the ropes during your first visit

# New here?
Whether you have supplies, time, or just curiosity, there’s a role for you. Let us know your comfort level and we’ll connect you with a small crew so you’re not navigating alone.`,
	},
}

// seedGroups inserts the carried-over groups on first run only.
func seedGroups(conn *sql.DB) error {
	var count int
	if err := conn.QueryRow(`SELECT count(*) FROM groups`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	for i, g := range groupSeeds {
		if _, err := conn.Exec(
			`INSERT INTO groups (slug, name, summary, description, pills, body, match_terms, position, enabled)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			g.slug, g.name, g.summary, g.description, g.pills, g.body, g.matchTerms, i, g.enabled,
		); err != nil {
			return err
		}
	}
	return nil
}

// backfillGroupMatchTerms fills in the default match terms for the seeded
// groups on databases that were created before the column existed — the
// terms tie the carried-over programs to their events. It only touches
// rows whose terms are still blank, so an admin's own edits stand.
func backfillGroupMatchTerms(conn *sql.DB) error {
	for _, g := range groupSeeds {
		if g.matchTerms == "" {
			continue
		}
		if _, err := conn.Exec(
			`UPDATE groups SET match_terms = ? WHERE slug = ? AND match_terms = ''`,
			g.matchTerms, g.slug,
		); err != nil {
			return err
		}
	}
	return nil
}
