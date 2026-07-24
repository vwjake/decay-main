package db

import (
	"fmt"
	"strconv"
)

// PerPagePublic and PerPageAdmin are how many events a page holds. The
// public lists are roomy cards, the admin one is table rows, so the admin
// page fits more before it stops being scannable.
const (
	PerPagePublic = 40
	PerPageAdmin  = 50
)

// Page describes where a paginated list is and how to get to the rest of
// it. Path is the URL the links are built from, e.g. "/events".
type Page struct {
	Number  int
	Total   int
	Count   int // items across every page
	Path    string
	PerPage int
}

func (p Page) HasPrev() bool { return p.Number > 1 }
func (p Page) HasNext() bool { return p.Number < p.Total }

// Multiple reports whether there's more than one page, i.e. whether the
// controls are worth rendering at all.
func (p Page) Multiple() bool { return p.Total > 1 }

func (p Page) PrevURL() string { return p.urlFor(p.Number - 1) }
func (p Page) NextURL() string { return p.urlFor(p.Number + 1) }

// urlFor keeps page 1 on the bare path so the canonical URL of a list
// doesn't carry a redundant ?page=1.
func (p Page) urlFor(n int) string {
	if n <= 1 {
		return p.Path
	}
	return p.Path + "?page=" + strconv.Itoa(n)
}

// Label reads "Page 2 of 14".
func (p Page) Label() string { return fmt.Sprintf("Page %d of %d", p.Number, p.Total) }

// Range reads "41–80 of 530", so it's clear how much list there is.
func (p Page) Range() string {
	if p.Count == 0 {
		return "none"
	}
	first := (p.Number-1)*p.PerPage + 1
	last := first + p.PerPage - 1
	if last > p.Count {
		last = p.Count
	}
	return fmt.Sprintf("%d–%d of %d", first, last, p.Count)
}

// Paginate returns one page of items and a description of where it sits.
// An out-of-range page number is clamped rather than erroring, so a
// hand-edited or stale ?page= lands somewhere sensible instead of on an
// error page.
func Paginate[T any](items []T, page, perPage int) ([]T, Page) {
	if perPage < 1 {
		perPage = 1
	}
	total := (len(items) + perPage - 1) / perPage
	if total < 1 {
		total = 1
	}
	if page < 1 {
		page = 1
	}
	if page > total {
		page = total
	}

	start := (page - 1) * perPage
	end := start + perPage
	if start > len(items) {
		start = len(items)
	}
	if end > len(items) {
		end = len(items)
	}

	return items[start:end], Page{
		Number:  page,
		Total:   total,
		Count:   len(items),
		PerPage: perPage,
	}
}

// PageNumber reads a ?page= value, treating anything unparseable as the
// first page.
func PageNumber(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 1
	}
	return n
}
