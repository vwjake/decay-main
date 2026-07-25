package db

import "testing"

func TestDollars(t *testing.T) {
	cases := map[int64]string{
		0:       "$0",
		500:     "$5",
		1050:    "$10.50",
		123456:  "$1,234.56",
		100000:  "$1,000",
		-2550:   "-$25.50",
		1234567: "$12,345.67",
	}
	for cents, want := range cases {
		if got := Dollars(cents); got != want {
			t.Errorf("Dollars(%d) = %q, want %q", cents, got, want)
		}
	}
}

func TestParseDollars(t *testing.T) {
	ok := map[string]int64{
		"":         0,
		"40":       4000,
		"$40":      4000,
		"1,234.50": 123450,
		"40.5":     4050,
		".99":      99,
		"0.07":     7,
		"-25.50":   -2550,
	}
	for in, want := range ok {
		got, err := ParseDollars(in)
		if err != nil {
			t.Errorf("ParseDollars(%q) errored: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseDollars(%q) = %d, want %d", in, got, want)
		}
	}

	for _, bad := range []string{"abc", "1.234", "12x"} {
		if _, err := ParseDollars(bad); err == nil {
			t.Errorf("ParseDollars(%q) = nil error, want error", bad)
		}
	}
}

func TestParseDollarsRoundTrip(t *testing.T) {
	// dollarsPlain output must parse back to the same cents.
	for _, cents := range []int64{0, 700, 4000, 123450, 99} {
		got, err := ParseDollars(dollarsPlain(cents))
		if err != nil || got != cents {
			t.Errorf("round trip of %d: got %d, err %v", cents, got, err)
		}
	}
}

func TestQuarterOfAndRange(t *testing.T) {
	loc := pacificZone(t)
	q := QuarterOf(at(t, loc, "2026-07-25 20:00"), loc)
	if q.Q != 3 || q.Year != 2026 {
		t.Fatalf("QuarterOf = %v, want Q3 2026", q)
	}
	if q.String() != "2026-Q3" || q.Label() != "Q3 2026" {
		t.Errorf("labels: %q / %q", q.String(), q.Label())
	}
	from, to := q.Range(loc)
	if from.Month() != 7 || from.Day() != 1 || to.Month() != 10 || to.Day() != 1 {
		t.Errorf("Range = %s .. %s", from, to)
	}
}

func TestQuarterStepAndParse(t *testing.T) {
	q := Quarter{Year: 2026, Q: 1}
	if p := q.Prev(); p.Year != 2025 || p.Q != 4 {
		t.Errorf("Prev = %v", p)
	}
	if n := (Quarter{Year: 2026, Q: 4}).Next(); n.Year != 2027 || n.Q != 1 {
		t.Errorf("Next = %v", n)
	}
	got, ok := ParseQuarter("2026-Q3")
	if !ok || got.Year != 2026 || got.Q != 3 {
		t.Errorf("ParseQuarter ok=%v got=%v", ok, got)
	}
	for _, bad := range []string{"", "2026-Q5", "nonsense", "2026-Q0"} {
		if _, ok := ParseQuarter(bad); ok {
			t.Errorf("ParseQuarter(%q) ok, want not ok", bad)
		}
	}
}
