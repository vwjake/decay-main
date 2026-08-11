package admin

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"decay-main/db"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

// registerGroupSeriesRoutes wires a one-time cleanup tool: find events that
// repeat under the same title and type but predate series tracking (made
// one at a time, or by the old repeat tool before it linked copies), and
// let an admin confirm which of them to link into a series.
func registerGroupSeriesRoutes(g *echo.Group, conn *sql.DB) {
	g.GET("/events/group-series", showGroupSeries(conn))
	g.POST("/events/group-series", applyGroupSeries(conn))
}

func showGroupSeries(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		groups, err := candidateSeriesGroups(conn)
		if err != nil {
			return err
		}
		notice := ""
		if n := c.QueryParam("grouped"); n != "" {
			notice = "Grouped " + n + " series."
		}
		return views.AdminGroupSeries(groups, currentUser(c), notice).Render(c.Request().Context(), c.Response())
	}
}

// candidateSeriesGroups finds ungrouped events sharing a title and event
// type — two or more, since one alone isn't a repeat. Grouped in Go rather
// than SQL, the same way the events list splits series from one-offs.
func candidateSeriesGroups(conn *sql.DB) ([]views.CandidateGroup, error) {
	events, err := db.UngroupedEvents(conn)
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	var groups []views.CandidateGroup
	for _, ev := range events {
		key := ev.Title + "\x00" + ev.EventType
		if i, ok := index[key]; ok {
			groups[i].Events = append(groups[i].Events, ev)
			continue
		}
		index[key] = len(groups)
		groups = append(groups, views.CandidateGroup{
			Title:     ev.Title,
			EventType: ev.EventType,
			Events:    []db.Event{ev},
		})
	}
	var out []views.CandidateGroup
	for _, g := range groups {
		if len(g.Events) >= 2 {
			out = append(out, g)
		}
	}
	return out, nil
}

// applyGroupSeries links whichever checked groups came back, each as its
// own series — the event ids travel in the checkbox value itself rather
// than an index, so a group can't be misapplied if the candidate list
// changed between the page loading and the form posting.
func applyGroupSeries(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		params, err := c.FormParams()
		if err != nil {
			return err
		}
		grouped := 0
		for _, raw := range params["group"] {
			var ids []int64
			for _, p := range strings.Split(raw, ",") {
				id, err := strconv.ParseInt(p, 10, 64)
				if err != nil {
					continue
				}
				ids = append(ids, id)
			}
			if len(ids) < 2 {
				continue
			}
			if _, err := db.GroupIntoSeries(conn, ids); err != nil {
				return err
			}
			grouped++
		}
		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/admin/events/group-series?grouped=%d", grouped))
	}
}
