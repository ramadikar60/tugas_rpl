package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ============================================================
// TIMELINE — Section 5
// ============================================================

// Timeline — ambil activity timeline
//
// Query params:
//
//	type   — all | note | task | version | activity
//	limit  — default 50
//	offset — default 0
//	from   — YYYY-MM-DD (opsional)
//	to     — YYYY-MM-DD (opsional)
func Timeline(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	itemType := q.Get("type")
	if itemType == "" {
		itemType = "all"
	}

	limit := 50
	offset := 0
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v >= 0 {
		offset = v
	}

	dateFrom := q.Get("from")
	dateTo := q.Get("to")

	events := []map[string]interface{}{}

	// Helper untuk date filter
	buildDateFilter := func(column string) (string, []interface{}) {
		where := ""
		args := []interface{}{}
		if dateFrom != "" {
			where += " AND " + column + " >= ?"
			args = append(args, dateFrom+" 00:00:00")
		}
		if dateTo != "" {
			where += " AND " + column + " <= ?"
			args = append(args, dateTo+" 23:59:59")
		}
		return where, args
	}

	// ---- NOTES ----
	if itemType == "all" || itemType == "note" {
		filter, args := buildDateFilter("created_at")
		query := `SELECT id, title, created_at, updated_at FROM notes
		          WHERE user_id = ? AND is_trashed = FALSE` + filter +
			` ORDER BY created_at DESC LIMIT ?`
		args = append([]interface{}{userID}, args...)
		args = append(args, limit)

		rows, err := config.DB.Query(query, args...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id int64
				var title string
				var createdAt, updatedAt time.Time
				_ = rows.Scan(&id, &title, &createdAt, &updatedAt)

				// Event: Note created
				events = append(events, map[string]interface{}{
					"type":        "note_created",
					"category":    "note",
					"id":          id,
					"title":       title,
					"description": "Membuat catatan",
					"timestamp":   createdAt,
				})

				// Event: Note updated (kalau beda > 1 menit)
				if updatedAt.Sub(createdAt) > time.Minute {
					events = append(events, map[string]interface{}{
						"type":        "note_updated",
						"category":    "note",
						"id":          id,
						"title":       title,
						"description": "Memperbarui catatan",
						"timestamp":   updatedAt,
					})
				}
			}
		}
	}

	// ---- TASKS ----
	if itemType == "all" || itemType == "task" {
		filter, args := buildDateFilter("created_at")
		query := `SELECT id, title, status, priority, created_at, completed_at FROM tasks
		          WHERE user_id = ?` + filter +
			` ORDER BY created_at DESC LIMIT ?`
		args = append([]interface{}{userID}, args...)
		args = append(args, limit)

		rows, err := config.DB.Query(query, args...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id int64
				var title, status, priority string
				var createdAt time.Time
				var completedAt sql.NullTime
				_ = rows.Scan(&id, &title, &status, &priority, &createdAt, &completedAt)

				// Event: Task created
				events = append(events, map[string]interface{}{
					"type":        "task_created",
					"category":    "task",
					"id":          id,
					"title":       title,
					"description": "Membuat tugas (" + priority + ")",
					"timestamp":   createdAt,
				})

				// Event: Task completed
				if completedAt.Valid {
					events = append(events, map[string]interface{}{
						"type":        "task_completed",
						"category":    "task",
						"id":          id,
						"title":       title,
						"description": "Menyelesaikan tugas",
						"timestamp":   completedAt.Time,
					})
				}
			}
		}
	}

	// ---- ACTIVITY LOG ----
	if itemType == "all" || itemType == "activity" {
		filter, args := buildDateFilter("created_at")
		query := `SELECT id, action, COALESCE(item_type,''), item_id, COALESCE(description,''), created_at
		          FROM activity_log
		          WHERE user_id = ?` + filter +
			` ORDER BY created_at DESC LIMIT ?`
		args = append([]interface{}{userID}, args...)
		args = append(args, limit)

		rows, err := config.DB.Query(query, args...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id int64
				var action, itemType, description string
				var itemID sql.NullInt64
				var createdAt time.Time
				_ = rows.Scan(&id, &action, &itemType, &itemID, &description, &createdAt)

				events = append(events, map[string]interface{}{
					"type":        "activity",
					"category":    "activity",
					"id":          id,
					"title":       description,
					"description": action,
					"timestamp":   createdAt,
				})
			}
		}
	}

	// Sort by timestamp DESC
	sortEventsByTime(events)

	// Apply offset & limit
	if offset >= len(events) {
		events = []map[string]interface{}{}
	} else {
		end := offset + limit
		if end > len(events) {
			end = len(events)
		}
		events = events[offset:end]
	}

	writeJSON(w, 200, map[string]interface{}{
		"events": events,
		"total":  len(events),
		"offset": offset,
		"limit":  limit,
	})
}

// NoteTimeline — timeline untuk 1 catatan (perubahan + versi)
func NoteTimeline(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "id")

	// Verifikasi ownership
	var noteOwner int64
	err := config.DB.QueryRow("SELECT user_id FROM notes WHERE id=?", noteID).Scan(&noteOwner)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Note tidak ditemukan"})
		return
	}

	isOwner := noteOwner == userID

	events := []map[string]interface{}{}

	// Note created
	var title string
	var createdAt, updatedAt time.Time
	err = config.DB.QueryRow(`
		SELECT title, created_at, updated_at FROM notes
		WHERE id = ?`, noteID).Scan(&title, &createdAt, &updatedAt)
	if err == nil {
		events = append(events, map[string]interface{}{
			"type":        "created",
			"title":       "Catatan dibuat",
			"description": title,
			"timestamp":   createdAt,
		})

		if updatedAt.Sub(createdAt) > time.Minute {
			events = append(events, map[string]interface{}{
				"type":        "updated",
				"title":       "Terakhir diubah",
				"description": title,
				"timestamp":   updatedAt,
			})
		}
	}

	// Versions
	if isOwner {
		rows, err := config.DB.Query(`
			SELECT id, version_number, word_count, is_auto, created_at
			FROM note_versions
			WHERE note_id = ?
			ORDER BY created_at DESC
			LIMIT 50`, noteID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id int64
				var vn, wc int
				var isAuto bool
				var t time.Time
				_ = rows.Scan(&id, &vn, &wc, &isAuto, &t)

				label := "Versi v" + strconv.FormatInt(int64(vn), 10)
				desc := strconv.Itoa(wc) + " kata"
				if isAuto {
					desc += " · auto"
				}

				events = append(events, map[string]interface{}{
					"type":        "version",
					"id":          id,
					"title":       label,
					"description": desc,
					"timestamp":   t,
				})
			}
		}
	}

	// Activity log untuk note ini
	rows2, err := config.DB.Query(`
		SELECT action, COALESCE(description,''), created_at
		FROM activity_log
		WHERE user_id = ? AND item_type = 'note' AND item_id = ?
		ORDER BY created_at DESC
		LIMIT 30`, noteOwner, noteID)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var action, description string
			var t time.Time
			_ = rows2.Scan(&action, &description, &t)
			events = append(events, map[string]interface{}{
				"type":        "activity",
				"title":       action,
				"description": description,
				"timestamp":   t,
			})
		}
	}

	sortEventsByTime(events)

	writeJSON(w, 200, events)
}

// TaskTimeline — timeline 1 task (perubahan status)
func TaskTimeline(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	taskID, _ := pathInt(r, "id")

	// Verifikasi ownership
	var taskOwner int64
	err := config.DB.QueryRow("SELECT user_id FROM tasks WHERE id=?", taskID).Scan(&taskOwner)
	if err != nil || taskOwner != userID {
		writeJSON(w, 404, map[string]string{"error": "Task tidak ditemukan"})
		return
	}

	events := []map[string]interface{}{}

	// Created + completed
	var title string
	var createdAt time.Time
	var completedAt sql.NullTime
	err = config.DB.QueryRow(`
		SELECT title, created_at, completed_at FROM tasks WHERE id = ?`,
		taskID).Scan(&title, &createdAt, &completedAt)
	if err == nil {
		events = append(events, map[string]interface{}{
			"type":        "created",
			"title":       "Tugas dibuat",
			"description": title,
			"timestamp":   createdAt,
		})
		if completedAt.Valid {
			events = append(events, map[string]interface{}{
				"type":        "completed",
				"title":       "Tugas diselesaikan",
				"description": title,
				"timestamp":   completedAt.Time,
			})
		}
	}

	// History
	rows, err := config.DB.Query(`
		SELECT action, COALESCE(old_value,''), COALESCE(new_value,''), created_at
		FROM task_history
		WHERE task_id = ?
		ORDER BY created_at DESC
		LIMIT 50`, taskID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var action, oldVal, newVal string
			var t time.Time
			_ = rows.Scan(&action, &oldVal, &newVal, &t)

			desc := ""
			if oldVal != "" && newVal != "" {
				desc = oldVal + " → " + newVal
			} else if newVal != "" {
				desc = newVal
			}

			events = append(events, map[string]interface{}{
				"type":        "history",
				"title":       action,
				"description": desc,
				"timestamp":   t,
			})
		}
	}

	sortEventsByTime(events)

	writeJSON(w, 200, events)
}

// ActivityHeatmap — data heatmap (kontribusi per hari)
//
// Query params:
//
//	year — default tahun sekarang
func ActivityHeatmap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	now := time.Now()
	year := now.Year()
	if v := q.Get("year"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			year = n
		}
	}

	startDate := time.Date(year, 1, 1, 0, 0, 0, 0, time.Local)
	endDate := startDate.AddDate(1, 0, 0)

	days := map[string]int{}

	// Notes created
	rows, _ := config.DB.Query(`
		SELECT DATE(created_at) AS d, COUNT(*) AS c
		FROM notes
		WHERE user_id = ? AND is_trashed = FALSE
		  AND created_at >= ? AND created_at < ?
		GROUP BY DATE(created_at)`,
		userID, startDate, endDate)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var d time.Time
			var c int
			_ = rows.Scan(&d, &c)
			key := d.Format("2006-01-02")
			days[key] += c
		}
	}

	// Tasks completed
	rows2, _ := config.DB.Query(`
		SELECT DATE(completed_at) AS d, COUNT(*) AS c
		FROM tasks
		WHERE user_id = ? AND completed_at IS NOT NULL
		  AND completed_at >= ? AND completed_at < ?
		GROUP BY DATE(completed_at)`,
		userID, startDate, endDate)
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var d time.Time
			var c int
			_ = rows2.Scan(&d, &c)
			key := d.Format("2006-01-02")
			days[key] += c
		}
	}

	// Convert to list
	result := []map[string]interface{}{}
	for date, count := range days {
		result = append(result, map[string]interface{}{
			"date":  date,
			"count": count,
		})
	}

	writeJSON(w, 200, map[string]interface{}{
		"year": year,
		"days": result,
	})
}

// ============================================================
// Helper
// ============================================================
func sortEventsByTime(events []map[string]interface{}) {
	// Sort DESC by timestamp
	for i := 0; i < len(events); i++ {
		for j := i + 1; j < len(events); j++ {
			ti, _ := events[i]["timestamp"].(time.Time)
			tj, _ := events[j]["timestamp"].(time.Time)
			if tj.After(ti) {
				events[i], events[j] = events[j], events[i]
			}
		}
	}
}

var _ = strings.TrimSpace
