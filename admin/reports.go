package admin

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"decay-main/db"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

func registerReportRoutes(g *echo.Group, conn *sql.DB) {
	g.GET("/reports", viewReport(conn))
	g.GET("/reports/entry", reportsEntry(conn))
	g.POST("/reports/events/:id", saveReport(conn))
	g.POST("/reports/donations", addDonation(conn))
	g.POST("/reports/donations/:id/delete", deleteDonation(conn))
}

// reportPeriod is the span a report covers, plus how it was chosen so the
// page can highlight the right control and title itself.
type reportPeriod struct {
	From, To  time.Time
	Quarter   db.Quarter
	IsQuarter bool
}

// Title names the period for the page heading.
func (p reportPeriod) Title() string {
	if p.IsQuarter {
		return p.Quarter.Label()
	}
	// The stored range is half-open [from, to); show the inclusive last day.
	last := p.To.AddDate(0, 0, -1)
	return p.From.Format("Jan 2, 2006") + " – " + last.Format("Jan 2, 2006")
}

func (p reportPeriod) FromStr() string { return p.From.Format("2006-01-02") }
func (p reportPeriod) ToStr() string {
	return p.To.AddDate(0, 0, -1).Format("2006-01-02")
}

// resolvePeriod reads the query params into a period, defaulting to the
// current quarter. A custom from/to wins when both parse; anything
// unparseable falls back rather than erroring.
func resolvePeriod(c echo.Context) reportPeriod {
	if raw := c.QueryParam("quarter"); raw != "" {
		if q, ok := db.ParseQuarter(raw); ok {
			from, to := q.Range(pacific)
			return reportPeriod{From: from, To: to, Quarter: q, IsQuarter: true}
		}
	}
	from, fromOK := parseDate(c.QueryParam("from"))
	to, toOK := parseDate(c.QueryParam("to"))
	if fromOK && toOK && to.After(from) {
		// The picker's "to" is an inclusive day; store it half-open.
		return reportPeriod{From: from, To: to.AddDate(0, 0, 1)}
	}

	q := db.QuarterOf(time.Now(), pacific)
	fromQ, toQ := q.Range(pacific)
	return reportPeriod{From: fromQ, To: toQ, Quarter: q, IsQuarter: true}
}

func parseDate(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02", raw, pacific)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func viewReport(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		period := resolvePeriod(c)
		stats, err := db.RangeStats(conn, period.From, period.To)
		if err != nil {
			return err
		}
		donations, err := db.ListDonations(conn, period.From, period.To)
		if err != nil {
			return err
		}

		// Offer the current quarter and the seven before it to jump between.
		current := db.QuarterOf(time.Now(), pacific)
		quarters := make([]db.Quarter, 0, 8)
		q := current
		for i := 0; i < 8; i++ {
			quarters = append(quarters, q)
			q = q.Prev()
		}

		return views.AdminReport(views.ReportPage{
			Title:     period.Title(),
			IsQuarter: period.IsQuarter,
			Selected:  period.Quarter,
			FromStr:   period.FromStr(),
			ToStr:     period.ToStr(),
			Quarters:  quarters,
			Stats:     stats,
			Donations: donations,
		}, currentUser(c)).Render(c.Request().Context(), c.Response())
	}
}

func reportsEntry(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		return renderEntry(c, conn, "")
	}
}

func renderEntry(c echo.Context, conn *sql.DB, msg string) error {
	past, err := db.PastEvents(conn)
	if err != nil {
		return err
	}
	past, page := db.Paginate(past, db.PageNumber(c.QueryParam("page")), db.PerPageAdmin)
	page.Path = "/admin/reports/entry"

	ids := make([]int64, len(past))
	for i, e := range past {
		ids[i] = e.ID
	}
	reports, err := db.EventReportsFor(conn, ids)
	if err != nil {
		return err
	}
	donations, err := db.DonationTotalsFor(conn, ids)
	if err != nil {
		return err
	}

	rows := make([]views.EntryRow, len(past))
	for i, e := range past {
		rows[i] = views.EntryRow{
			Event:         e,
			Report:        reports[e.ID], // zero value reads as "not recorded"
			DonationCents: donations[e.ID],
		}
	}
	return views.AdminReportsEntry(rows, page, currentUser(c), msg).Render(c.Request().Context(), c.Response())
}

func saveReport(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}

		attendance, err := parseOptionalInt(c.FormValue("attendance"))
		if err != nil {
			return renderEntry(c, conn, "Attendance has to be a whole number.")
		}
		var door *int64
		if raw := strings.TrimSpace(c.FormValue("door")); raw != "" {
			cents, err := db.ParseDollars(raw)
			if err != nil {
				return renderEntry(c, conn, "Door money has to be a dollar amount.")
			}
			door = &cents
		}

		if err := db.SaveEventReport(conn, id, attendance, door, strings.TrimSpace(c.FormValue("notes"))); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, entryReturn(c))
	}
}

func addDonation(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		cents, err := db.ParseDollars(c.FormValue("amount"))
		if err != nil || cents <= 0 {
			return renderEntry(c, conn, "Enter a donation amount greater than zero.")
		}

		received := time.Now().In(pacific)
		if raw := c.FormValue("received_at"); raw != "" {
			if t, ok := parseDate(raw); ok {
				received = t
			}
		}

		var eventID *int64
		if raw := strings.TrimSpace(c.FormValue("event_id")); raw != "" {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest)
			}
			eventID = &id
		}

		if err := db.AddDonation(conn, eventID, cents,
			strings.TrimSpace(c.FormValue("source")),
			strings.TrimSpace(c.FormValue("note")), received); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, entryReturn(c))
	}
}

func deleteDonation(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.DeleteDonation(conn, id); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, entryReturn(c))
	}
}

// entryReturn keeps the poster on the page they submitted from, so saving a
// row deep in the list doesn't bounce them back to page 1.
func entryReturn(c echo.Context) string {
	if page := c.FormValue("page"); page != "" && page != "1" {
		return "/admin/reports/entry?page=" + page
	}
	return "/admin/reports/entry"
}

// parseOptionalInt reads a blank field as "not recorded" (nil) and any
// other value as a whole number.
func parseOptionalInt(raw string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	return &n, nil
}
