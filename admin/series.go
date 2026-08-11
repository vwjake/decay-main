package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"decay-main/db"
	"decay-main/images"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

// registerSeriesRoutes wires the series page: view a repeating event's whole
// run and push shared details or a new flyer out to every occurrence that
// hasn't happened yet.
func registerSeriesRoutes(g *echo.Group, conn *sql.DB, uploadsDir string) {
	g.GET("/series/:id", showSeries(conn))
	g.POST("/series/:id", pushSeriesDetails(conn))
	g.POST("/series/:id/flyer", pushSeriesFlyer(conn, uploadsDir))
}

func seriesPath(seriesID int64) string {
	return "/admin/series/" + strconv.FormatInt(seriesID, 10)
}

// loadSeries fetches every event sharing seriesID, in date order. A series
// id with no matching events (its anchor event was deleted, or the id in
// the URL was never a series) reads as not found.
func loadSeries(conn *sql.DB, rawID string) (int64, []db.Event, error) {
	seriesID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return 0, nil, echo.NewHTTPError(http.StatusBadRequest)
	}
	events, err := db.EventsInSeries(conn, seriesID)
	if err != nil {
		return 0, nil, err
	}
	if len(events) == 0 {
		return 0, nil, echo.NewHTTPError(http.StatusNotFound)
	}
	return seriesID, events, nil
}

func showSeries(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		seriesID, events, err := loadSeries(conn, c.Param("id"))
		if err != nil {
			return err
		}
		data := seriesPageData(seriesID, events)
		if n := c.QueryParam("synced"); n != "" {
			data.FlashNotice = "Synced to " + n + " upcoming event" + plural1(n) + "."
		}
		return views.AdminSeries(data, currentUser(c)).Render(c.Request().Context(), c.Response())
	}
}

func plural1(n string) string {
	if n == "1" {
		return ""
	}
	return "s"
}

// seriesPageData picks the representative event a sync form is prefilled
// from — the soonest upcoming occurrence, or the most recent one if the
// whole series is in the past — and splits the rest into upcoming/past for
// display.
func seriesPageData(seriesID int64, events []db.Event) views.SeriesPageData {
	now := time.Now()
	rep := events[len(events)-1]
	for _, ev := range events {
		if ev.StartsAt.After(now) {
			rep = ev
			break
		}
	}
	var upcoming, past []db.Event
	for _, ev := range events {
		if ev.StartsAt.After(now) {
			upcoming = append(upcoming, ev)
		} else {
			past = append(past, ev)
		}
	}
	return views.SeriesPageData{
		SeriesID:       seriesID,
		Representative: rep,
		Upcoming:       upcoming,
		Past:           past,
	}
}

// pushSeriesDetails applies the shared fields — title, type, description,
// location, link — to every occurrence that hasn't happened yet. Past
// events are left alone as a record of what they actually showed.
func pushSeriesDetails(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		seriesID, events, err := loadSeries(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			data := seriesPageData(seriesID, events)
			data.ErrorMsg = msg
			return views.AdminSeries(data, currentUser(c)).Render(c.Request().Context(), c.Response())
		}

		title := strings.TrimSpace(c.FormValue("title"))
		eventType := strings.TrimSpace(c.FormValue("event_type"))
		if title == "" || eventType == "" {
			return rerender("Title and type are required.")
		}
		location := strings.TrimSpace(c.FormValue("location"))
		description := c.FormValue("description")
		link := strings.TrimSpace(c.FormValue("link"))
		if link == "" {
			link = "#"
		}

		now := time.Now()
		updated := 0
		for _, ev := range events {
			if !ev.StartsAt.After(now) {
				continue
			}
			ev.Title = title
			ev.EventType = eventType
			ev.Location = location
			ev.Description = description
			ev.Link = link
			if err := db.UpdateEvent(conn, ev); err != nil {
				return err
			}
			updated++
		}
		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("%s?synced=%d", seriesPath(seriesID), updated))
	}
}

// pushSeriesFlyer uploads one flyer and points every occurrence that hasn't
// happened yet at it, cleaning up whichever old flyer files end up unused
// once the swap is done (the same rule uploadFlyer uses — never delete a
// file another event still references).
func pushSeriesFlyer(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		seriesID, events, err := loadSeries(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			data := seriesPageData(seriesID, events)
			data.ErrorMsg = msg
			return views.AdminSeries(data, currentUser(c)).Render(c.Request().Context(), c.Response())
		}

		fileHeader, err := c.FormFile("flyer")
		if err != nil {
			return rerender("Choose an image to upload.")
		}
		flyerDir := filepath.Join(uploadsDir, flyersSubdir)
		filename, err := saveImage(fileHeader, flyerDir)
		if err != nil {
			if errors.Is(err, errNotAnImage) {
				return rerender("That file doesn't look like an image. Use jpg, png, gif, or webp.")
			}
			return err
		}
		if err := images.MakeWeb(
			filepath.Join(flyerDir, filename),
			filepath.Join(flyerDir, "web", images.WebName(filename)),
		); err != nil {
			return err
		}

		now := time.Now()
		replaced := map[string]bool{}
		updated := 0
		for _, ev := range events {
			if !ev.StartsAt.After(now) {
				continue
			}
			previous, err := db.SetEventFlyer(conn, ev.ID, filename)
			if err != nil {
				return err
			}
			if previous != "" && previous != filename {
				replaced[previous] = true
			}
			updated++
		}
		for old := range replaced {
			inUse, err := db.FlyerInUse(conn, old)
			if err != nil {
				return err
			}
			if !inUse {
				_ = os.Remove(filepath.Join(flyerDir, old))
				_ = os.Remove(filepath.Join(flyerDir, "web", images.WebName(old)))
			}
		}
		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("%s?synced=%d", seriesPath(seriesID), updated))
	}
}
