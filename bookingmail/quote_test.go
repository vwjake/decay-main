package bookingmail

import "testing"

func TestSplitQuotedGmailAttribution(t *testing.T) {
	body := "Sounds good, see you at 6.\n\nOn Mon, Apr 1, 2026 at 9:12 AM DECAY <booking@decayolympia.org> wrote:\n> Load in at 6\n> Thanks"
	newText, quoted := splitQuoted(body)
	if newText != "Sounds good, see you at 6." {
		t.Errorf("newText = %q", newText)
	}
	if quoted == "" {
		t.Error("expected a quoted chain to be found")
	}
}

func TestSplitQuotedWrappedAttribution(t *testing.T) {
	body := "Yes please.\n\nOn Wed, Jun 10, 2026 at 4:02 PM Someone Longname <a@b.com>,\nwrote:\n> earlier text"
	newText, _ := splitQuoted(body)
	if newText != "Yes please." {
		t.Errorf("newText = %q", newText)
	}
}

func TestSplitQuotedOutlookSeparator(t *testing.T) {
	body := "Confirmed.\n\n________________________________\nFrom: DECAY <booking@decayolympia.org>\nSent: Monday"
	newText, _ := splitQuoted(body)
	if newText != "Confirmed." {
		t.Errorf("newText = %q", newText)
	}
}

func TestSplitQuotedBareAngle(t *testing.T) {
	body := "ok!\n> previous\n> lines"
	newText, quoted := splitQuoted(body)
	if newText != "ok!" {
		t.Errorf("newText = %q", newText)
	}
	if quoted == "" {
		t.Error("expected quoted text")
	}
}

func TestSplitQuotedNoQuoteFound(t *testing.T) {
	body := "Just a note.\n\nThanks,\nJake"
	newText, quoted := splitQuoted(body)
	if newText != body {
		t.Errorf("newText = %q, want unchanged body", newText)
	}
	if quoted != "" {
		t.Errorf("quoted = %q, want empty", quoted)
	}
}

func TestSplitQuotedAllQuotedKeepsBodyVisible(t *testing.T) {
	body := "> only quoted text\n> nothing else"
	newText, quoted := splitQuoted(body)
	if newText != body {
		t.Errorf("newText = %q, want the whole body kept visible when trimming would hide everything", newText)
	}
	if quoted != "" {
		t.Errorf("quoted = %q, want nothing hidden in that case", quoted)
	}
}

func TestSnippetStripsQuotedChainAndTruncates(t *testing.T) {
	got := snippet("> old thing\nActual reply here", 120)
	if got != "Actual reply here" {
		t.Errorf("snippet = %q", got)
	}

	long := ""
	for i := 0; i < 200; i++ {
		long += "ab"
	}
	got = snippet(long, 10)
	if got != "ababababab…" {
		t.Errorf("truncated snippet = %q", got)
	}
}

func TestNormalizeAddressesDropsMailboxAndJunk(t *testing.T) {
	got := normalizeAddresses([]string{"BOOKING@decayolympia.org", "Foo@Bar.com", "not-an-email", ""}, "booking@decayolympia.org")
	if len(got) != 1 || got[0] != "foo@bar.com" {
		t.Errorf("normalizeAddresses = %v", got)
	}
}

func TestHTMLToTextStripsTagsAndEntities(t *testing.T) {
	html := "<html><body><p>April 3rd works. <b>Load-in at 6pm.</b></p><p>&mdash; DECAY</p></body></html>"
	got := htmlToText(html)
	if got != "April 3rd works. Load-in at 6pm.\n— DECAY" {
		t.Errorf("htmlToText = %q", got)
	}
}
