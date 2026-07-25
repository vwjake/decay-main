package db

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Dollars renders a cents amount as "$1,234.50" — a whole-dollar amount
// drops the cents ("$1,234"), matching how Product.Price reads.
func Dollars(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	whole := cents / 100
	frac := cents % 100

	// Group the whole-dollar part in threes.
	digits := strconv.FormatInt(whole, 10)
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}

	out := "$" + b.String()
	if frac != 0 {
		out += fmt.Sprintf(".%02d", frac)
	}
	if neg {
		return "-" + out
	}
	return out
}

// ParseDollars reads a dollar amount typed into a form ("1,234.50", "$40",
// "40.5") and returns it in cents. An empty string is 0, so a blank field
// records nothing rather than erroring.
func ParseDollars(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("$", "", ",", "", " ", "").Replace(s)
	if s == "" {
		return 0, nil
	}

	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")

	whole, frac := s, ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		whole, frac = s[:dot], s[dot+1:]
	}
	if len(frac) > 2 {
		return 0, errors.New("amount has more than two decimal places")
	}
	if whole == "" {
		whole = "0"
	}

	dollars, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, errors.New("not a valid dollar amount")
	}
	// Pad "5" -> "50" cents, "" -> "00".
	for len(frac) < 2 {
		frac += "0"
	}
	cents64, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, errors.New("not a valid dollar amount")
	}

	total := dollars*100 + cents64
	if neg {
		total = -total
	}
	return total, nil
}
