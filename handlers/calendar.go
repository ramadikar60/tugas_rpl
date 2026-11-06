package handlers

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ============================================================
// CALENDAR — Section 4
// ============================================================

// CalendarEvents — ambil semua event (notes, tasks, reminders) untuk bulan tertentu
//
// Query params:
//
//	year  — 2024 (default: tahun sekarang)
//	month — 1-12 (default: bulan sekarang)
//	day   — opsional, kalau ada → filter 1 hari saja
func CalendarEvents(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	if v := q.Get("year"); v != "" {
		if n, err := parseInt(v); err == nil {
			year = n
		}
	}
	if v := q.Get("month"); v != "" {
		if n, err := parseInt(v); err == nil && n >= 1 && n <= 12 {
			month = n
		}
	}

	// Tanggal awal & akhir bulan
	startOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	// Kalau ada filter day
	day := q.Get("day")
	if day != "" {
		if d, err := parseInt(day); err == nil && d >= 1 && d <= 31 {
			startOfMonth = time.Date(year, time.Month(month), d, 0, 0, 0, 0, time.Local)
			endOfMonth = startOfMonth.AddDate(0, 0, 1)
		}
	}

	events := []map[string]interface{}{}

	// ---- NOTES ----
	noteRows, err := config.DB.Query(`
		SELECT id, title, created_at, updated_at, notebook_id
		FROM notes
		WHERE user_id = ?
		  AND is_trashed = FALSE
		  AND (created_at >= ? AND created_at < ?)
		ORDER BY created_at ASC`,
		userID, startOfMonth, endOfMonth)
	if err == nil {
		defer noteRows.Close()
		for noteRows.Next() {
			var id int64
			var title string
			var createdAt, updatedAt time.Time
			var nbID sql.NullInt64
			_ = noteRows.Scan(&id, &title, &createdAt, &updatedAt, &nbID)

			event := map[string]interface{}{
				"type":       "note",
				"id":         id,
				"title":      title,
				"date":       createdAt.Format("2006-01-02"),
				"created_at": createdAt,
				"updated_at": updatedAt,
			}
			if nbID.Valid {
				event["notebook_id"] = nbID.Int64
			}
			events = append(events, event)
		}
	}

	// ---- TASKS ----
	taskRows, err := config.DB.Query(`
		SELECT id, title, status, priority, due_date, completed_at
		FROM tasks
		WHERE user_id = ?
		  AND due_date IS NOT NULL
		  AND due_date >= ? AND due_date < ?
		ORDER BY due_date ASC`,
		userID, startOfMonth, endOfMonth)
	if err == nil {
		defer taskRows.Close()
		for taskRows.Next() {
			var id int64
			var title, status, priority string
			var dueDate time.Time
			var completedAt sql.NullTime
			_ = taskRows.Scan(&id, &title, &status, &priority, &dueDate, &completedAt)

			event := map[string]interface{}{
				"type":     "task",
				"id":       id,
				"title":    title,
				"status":   status,
				"priority": priority,
				"date":     dueDate.Format("2006-01-02"),
				"due_date": dueDate,
			}
			if completedAt.Valid {
				event["completed_at"] = completedAt.Time
			}
			events = append(events, event)
		}
	}

	// ---- REMINDERS (task dengan reminder) ----
	reminderRows, err := config.DB.Query(`
		SELECT id, title, reminder_at, status
		FROM tasks
		WHERE user_id = ?
		  AND reminder_at IS NOT NULL
		  AND reminder_at >= ? AND reminder_at < ?
		  AND status NOT IN ('done', 'cancelled')
		ORDER BY reminder_at ASC`,
		userID, startOfMonth, endOfMonth)
	if err == nil {
		defer reminderRows.Close()
		for reminderRows.Next() {
			var id int64
			var title, status string
			var reminderAt time.Time
			_ = reminderRows.Scan(&id, &title, &reminderAt, &status)

			events = append(events, map[string]interface{}{
				"type":        "reminder",
				"id":          id,
				"title":       title,
				"status":      status,
				"date":        reminderAt.Format("2006-01-02"),
				"reminder_at": reminderAt,
			})
		}
	}

	// Group events by date
	byDate := map[string][]map[string]interface{}{}
	for _, e := range events {
		d := e["date"].(string)
		byDate[d] = append(byDate[d], e)
	}

	writeJSON(w, 200, map[string]interface{}{
		"year":    year,
		"month":   month,
		"events":  events,
		"by_date": byDate,
	})
}

// CalendarStats — ringkasan bulan (untuk header calendar)
func CalendarStats(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	if v := q.Get("year"); v != "" {
		if n, err := parseInt(v); err == nil {
			year = n
		}
	}
	if v := q.Get("month"); v != "" {
		if n, err := parseInt(v); err == nil && n >= 1 && n <= 12 {
			month = n
		}
	}

	startOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	stats := map[string]int{
		"notes":     0,
		"tasks":     0,
		"reminders": 0,
		"completed": 0,
	}

	// ---- Notes ----
	var notesCount int
	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM notes
		WHERE user_id = ? AND is_trashed = FALSE
		  AND created_at >= ? AND created_at < ?`,
		userID, startOfMonth, endOfMonth).Scan(&notesCount)
	stats["notes"] = notesCount

	// ---- Tasks ----
	var tasksCount int
	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM tasks
		WHERE user_id = ?
		  AND due_date >= ? AND due_date < ?`,
		userID, startOfMonth, endOfMonth).Scan(&tasksCount)
	stats["tasks"] = tasksCount

	// ---- Reminders ----
	var remindersCount int
	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM tasks
		WHERE user_id = ?
		  AND reminder_at >= ? AND reminder_at < ?
		  AND status NOT IN ('done', 'cancelled')`,
		userID, startOfMonth, endOfMonth).Scan(&remindersCount)
	stats["reminders"] = remindersCount

	// ---- Completed tasks ----
	var completedCount int
	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM tasks
		WHERE user_id = ?
		  AND status = 'done'
		  AND completed_at >= ? AND completed_at < ?`,
		userID, startOfMonth, endOfMonth).Scan(&completedCount)
	stats["completed"] = completedCount

	writeJSON(w, 200, stats)
}

// CalendarDayDetail — detail event 1 hari tertentu
func CalendarDayDetail(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	dateStr := r.URL.Query().Get("date") // YYYY-MM-DD

	if dateStr == "" {
		writeJSON(w, 400, map[string]string{"error": "Parameter date wajib diisi"})
		return
	}

	date, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Format tanggal salah"})
		return
	}

	startOfDay := date
	endOfDay := date.AddDate(0, 0, 1)

	events := []map[string]interface{}{}

	// Notes
	rows, _ := config.DB.Query(`
		SELECT id, title, created_at, updated_at FROM notes
		WHERE user_id = ? AND is_trashed = FALSE
		  AND created_at >= ? AND created_at < ?
		ORDER BY created_at ASC`,
		userID, startOfDay, endOfDay)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var title string
			var createdAt, updatedAt time.Time
			_ = rows.Scan(&id, &title, &createdAt, &updatedAt)
			events = append(events, map[string]interface{}{
				"type":       "note",
				"id":         id,
				"title":      title,
				"time":       createdAt.Format("15:04"),
				"created_at": createdAt,
			})
		}
	}

	// Tasks
	rows2, _ := config.DB.Query(`
		SELECT id, title, status, priority, due_date FROM tasks
		WHERE user_id = ? AND due_date >= ? AND due_date < ?
		ORDER BY due_date ASC`,
		userID, startOfDay, endOfDay)
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var id int64
			var title, status, priority string
			var dueDate time.Time
			_ = rows2.Scan(&id, &title, &status, &priority, &dueDate)
			events = append(events, map[string]interface{}{
				"type":     "task",
				"id":       id,
				"title":    title,
				"status":   status,
				"priority": priority,
				"time":     dueDate.Format("15:04"),
				"due_date": dueDate,
			})
		}
	}

	// Reminders
	rows3, _ := config.DB.Query(`
		SELECT id, title, reminder_at FROM tasks
		WHERE user_id = ? AND reminder_at >= ? AND reminder_at < ?
		  AND status NOT IN ('done', 'cancelled')
		ORDER BY reminder_at ASC`,
		userID, startOfDay, endOfDay)
	if rows3 != nil {
		defer rows3.Close()
		for rows3.Next() {
			var id int64
			var title string
			var reminderAt time.Time
			_ = rows3.Scan(&id, &title, &reminderAt)
			events = append(events, map[string]interface{}{
				"type":        "reminder",
				"id":          id,
				"title":       title,
				"time":        reminderAt.Format("15:04"),
				"reminder_at": reminderAt,
			})
		}
	}

	writeJSON(w, 200, map[string]interface{}{
		"date":   dateStr,
		"events": events,
	})
}

// CreateNoteOnDate — Calendar-to-Note (buat catatan langsung di tanggal tertentu)
func CreateNoteOnDate(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Date    string `json:"date"` // YYYY-MM-DD
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	if req.Date == "" {
		writeJSON(w, 400, map[string]string{"error": "Tanggal wajib diisi"})
		return
	}
	if req.Title == "" {
		req.Title = "Catatan " + req.Date
	}

	date, err := time.ParseInLocation("2006-01-02", req.Date, time.Local)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Format tanggal salah"})
		return
	}

	req.Content = sanitizeNoteHTML(req.Content)
	text := stripHTML(req.Content)
	words := len(strings.Fields(text))
	chars := len([]rune(text))

	// Insert dengan created_at custom
	res, err := config.DB.Exec(`
		INSERT INTO notes (user_id, title, content, content_text, word_count, char_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, req.Title, req.Content, text, words, chars,
		date.Format("2006-01-02 15:04:05"),
		date.Format("2006-01-02 15:04:05"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	writeJSON(w, 201, map[string]interface{}{
		"id":    id,
		"title": req.Title,
	})
}

// ============================================================
// Helper
// ============================================================
func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, &time.ParseError{}
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
