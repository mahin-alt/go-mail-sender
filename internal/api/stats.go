package api

import (
	"log"
	"net/http"
	"strconv"
	"time"
	_ "time/tzdata" // embed the time-zone database so ?tz= works on minimal containers too

	"sendmail/internal/store"
)

// Bounds for the daily series.
const (
	defaultStatsDays = 14
	maxStatsDays     = 90
)

// GetStats handles GET /api/v1/stats: the numbers behind the Overview page.
//
// Query params:
//
//	days  length of the daily chart series, 1-90 (default 14)
//	tz    IANA time-zone name such as "Asia/Dhaka" (default "UTC"); decides
//	      where "today" starts and how emails are grouped into days
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	days := defaultStatsDays
	if v := q.Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxStatsDays {
			writeError(w, http.StatusBadRequest, "days must be a whole number between 1 and "+strconv.Itoa(maxStatsDays))
			return
		}
		days = n
	}

	loc := time.UTC
	if v := q.Get("tz"); v != "" {
		// "Local" would mean the server's zone, which the caller cannot know.
		l, err := time.LoadLocation(v)
		if err != nil || v == "Local" {
			writeError(w, http.StatusBadRequest, `tz must be an IANA time-zone name such as "UTC" or "Asia/Dhaka"`)
			return
		}
		loc = l
	}

	if h.store == nil {
		writeError(w, http.StatusInternalServerError, "database store is not configured")
		return
	}

	stats, err := h.store.GetStats(r.Context(), store.StatsOptions{Days: days, Location: loc})
	if err != nil {
		log.Printf("get stats failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
