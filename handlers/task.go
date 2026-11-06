package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
	"notebook-app/models"
)

// ============================================================
// LIST TASKS
// Query params: status, priority, note_id, notebook_id, due, q, sort
// ============================================================
func ListTasks(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	query := `SELECT t.id, t.user_id, t.note_id, t.notebook_id, t.title, t.description,
	          t.status, t.priority, t.due_date, t.reminder_at, t.reminder_sent,
	          t.completed_at, t.sort_order, t.created_at, t.updated_at
	          FROM tasks t`
	args := []interface{}{}
	where := []string{"t.user_id = ?"}
	args = append(args, userID)

	status := q.Get("status")
	if status != "" && status != "all" {
		where = append(where, "t.status = ?")
		args = append(args, status)
	}

	priority := q.Get("priority")
	if priority != "" {
		where = append(where, "t.priority = ?")
		args = append(args, priority)
	}

	noteID := q.Get("note_id")
	if noteID != "" {
		where = append(where, "t.note_id = ?")
		args = append(args, noteID)
	}

	notebookID := q.Get("notebook_id")
	if notebookID != "" {
		where = append(where, "t.notebook_id = ?")
		args = append(args, notebookID)
	}

	due := q.Get("due")
	switch due {
	case "today":
		where = append(where, "DATE(t.due_date) = CURDATE()")
	case "week":
		where = append(where, "t.due_date BETWEEN NOW() AND DATE_ADD(NOW(), INTERVAL 7 DAY)")
	case "month":
		where = append(where, "t.due_date BETWEEN NOW() AND DATE_ADD(NOW(), INTERVAL 30 DAY)")
	case "overdue":
		where = append(where, "t.due_date < NOW() AND t.status NOT IN ('done','cancelled')")
	case "upcoming":
		where = append(where, "t.due_date >= NOW() AND t.status NOT IN ('done','cancelled')")
	case "no_due":
		where = append(where, "t.due_date IS NULL")
	}

	keyword := strings.TrimSpace(q.Get("q"))
	if keyword != "" {
		where = append(where, "(t.title LIKE ? OR t.description LIKE ?)")
		kw := "%" + keyword + "%"
		args = append(args, kw, kw)
	}

	query += " WHERE " + strings.Join(where, " AND ")

	sortBy := q.Get("sort")
	switch sortBy {
	case "due":
		query += " ORDER BY t.due_date IS NULL, t.due_date ASC, t.priority DESC"
	case "priority":
		query += " ORDER BY FIELD(t.priority, 'urgent','high','medium','low'), t.due_date IS NULL, t.due_date ASC"
	case "title":
		query += " ORDER BY t.title ASC"
	case "created":
		query += " ORDER BY t.created_at DESC"
	default:
		query += " ORDER BY t.sort_order ASC, t.created_at DESC"
	}

	query += " LIMIT 500"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	tasks := []*models.Task{}
	now := time.Now()

	for rows.Next() {
		t := &models.Task{}
		var noteIDVal, notebookIDVal sql.NullInt64
		var dueDate, reminderAt, completedAt sql.NullTime
		var description sql.NullString

		_ = rows.Scan(&t.ID, &t.UserID, &noteIDVal, &notebookIDVal, &t.Title, &description,
			&t.Status, &t.Priority, &dueDate, &reminderAt, &t.ReminderSent,
			&completedAt, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt)

		if noteIDVal.Valid {
			v := noteIDVal.Int64
			t.NoteID = &v
		}
		if notebookIDVal.Valid {
			v := notebookIDVal.Int64
			t.NotebookID = &v
		}
		if description.Valid {
			t.Description = description.String
		}
		if dueDate.Valid {
			v := dueDate.Time
			t.DueDate = &v
			t.IsOverdue = v.Before(now) && t.Status != "done" && t.Status != "cancelled"
			t.DaysUntilDue = int(v.Sub(now).Hours() / 24)
		}
		if reminderAt.Valid {
			v := reminderAt.Time
			t.ReminderAt = &v
		}
		if completedAt.Valid {
			v := completedAt.Time
			t.CompletedAt = &v
		}
		tasks = append(tasks, t)
	}

	writeJSON(w, 200, tasks)
}

// ============================================================
// GET TASK
// ============================================================
func GetTask(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	t := &models.Task{}
	var noteIDVal, notebookIDVal sql.NullInt64
	var dueDate, reminderAt, completedAt sql.NullTime
	var description sql.NullString

	err := config.DB.QueryRow(`
		SELECT id, user_id, note_id, notebook_id, title, description,
		       status, priority, due_date, reminder_at, reminder_sent,
		       completed_at, sort_order, created_at, updated_at
		FROM tasks WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&t.ID, &t.UserID, &noteIDVal, &notebookIDVal, &t.Title, &description,
			&t.Status, &t.Priority, &dueDate, &reminderAt, &t.ReminderSent,
			&completedAt, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt)

	if err == sql.ErrNoRows {
		writeJSON(w, 404, map[string]string{"error": "Task tidak ditemukan"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	if noteIDVal.Valid {
		v := noteIDVal.Int64
		t.NoteID = &v
	}
	if notebookIDVal.Valid {
		v := notebookIDVal.Int64
		t.NotebookID = &v
	}
	if description.Valid {
		t.Description = description.String
	}
	if dueDate.Valid {
		v := dueDate.Time
		t.DueDate = &v
	}
	if reminderAt.Valid {
		v := reminderAt.Time
		t.ReminderAt = &v
	}
	if completedAt.Valid {
		v := completedAt.Time
		t.CompletedAt = &v
	}

	writeJSON(w, 200, t)
}

// ============================================================
// CREATE TASK
// ============================================================
func CreateTask(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		NoteID      *int64  `json:"note_id"`
		NotebookID  *int64  `json:"notebook_id"`
		Priority    string  `json:"priority"`
		DueDate     *string `json:"due_date"`
		ReminderAt  *string `json:"reminder_at"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		writeJSON(w, 400, map[string]string{"error": "Judul task wajib diisi"})
		return
	}
	if req.Priority == "" {
		req.Priority = "medium"
	}

	var dueDateArg, reminderArg interface{}
	if req.DueDate != nil && *req.DueDate != "" {
		if t, err := parseDateTimeInput(*req.DueDate); err == nil {
			dueDateArg = t
		}
	}
	if req.ReminderAt != nil && *req.ReminderAt != "" {
		if t, err := parseDateTimeInput(*req.ReminderAt); err == nil {
			reminderArg = t
		}
	}

	res, err := config.DB.Exec(`
		INSERT INTO tasks (user_id, note_id, notebook_id, title, description, priority, due_date, reminder_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, req.NoteID, req.NotebookID, req.Title, req.Description,
		req.Priority, dueDateArg, reminderArg)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	logTaskAction(id, userID, "created", "", req.Title)

	writeJSON(w, 201, map[string]interface{}{"id": id, "title": req.Title})
}

// ============================================================
// UPDATE TASK
// ============================================================
func UpdateTask(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
		Priority    *string `json:"priority"`
		DueDate     *string `json:"due_date"`
		ReminderAt  *string `json:"reminder_at"`
		NotebookID  *int64  `json:"notebook_id"`
		SortOrder   *int    `json:"sort_order"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	// Ambil data lama untuk audit
	var oldStatus, oldPriority string
	_ = config.DB.QueryRow("SELECT status, priority FROM tasks WHERE id=? AND user_id=?",
		id, userID).Scan(&oldStatus, &oldPriority)

	// Build query dinamis dengan handling special cases (NULL, NOW())
	setClauses := []string{}
	args := []interface{}{}

	if req.Title != nil {
		setClauses = append(setClauses, "title = ?")
		args = append(args, *req.Title)
	}
	if req.Description != nil {
		setClauses = append(setClauses, "description = ?")
		args = append(args, *req.Description)
	}
	if req.Status != nil {
		setClauses = append(setClauses, "status = ?")
		args = append(args, *req.Status)

		if *req.Status == "done" {
			setClauses = append(setClauses, "completed_at = NOW()")
		} else {
			setClauses = append(setClauses, "completed_at = NULL")
		}
	}
	if req.Priority != nil {
		setClauses = append(setClauses, "priority = ?")
		args = append(args, *req.Priority)
	}
	if req.DueDate != nil {
		if *req.DueDate == "" {
			setClauses = append(setClauses, "due_date = NULL")
		} else {
			if t, err := parseDateTimeInput(*req.DueDate); err == nil {
				setClauses = append(setClauses, "due_date = ?")
				args = append(args, t)
			}
		}
	}
	if req.ReminderAt != nil {
		if *req.ReminderAt == "" {
			setClauses = append(setClauses, "reminder_at = NULL")
			setClauses = append(setClauses, "reminder_sent = FALSE")
		} else {
			if t, err := parseDateTimeInput(*req.ReminderAt); err == nil {
				setClauses = append(setClauses, "reminder_at = ?")
				args = append(args, t)
				setClauses = append(setClauses, "reminder_sent = FALSE")
			}
		}
	}
	if req.NotebookID != nil {
		setClauses = append(setClauses, "notebook_id = ?")
		args = append(args, *req.NotebookID)
	}
	if req.SortOrder != nil {
		setClauses = append(setClauses, "sort_order = ?")
		args = append(args, *req.SortOrder)
	}

	if len(setClauses) == 0 {
		writeJSON(w, 400, map[string]string{"error": "Tidak ada field"})
		return
	}

	query := "UPDATE tasks SET " + strings.Join(setClauses, ", ") +
		" WHERE id = ? AND user_id = ?"
	args = append(args, id, userID)

	if _, err := config.DB.Exec(query, args...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Log perubahan
	if req.Status != nil && *req.Status != oldStatus {
		logTaskAction(id, userID, "status_changed", oldStatus, *req.Status)
	}
	if req.Priority != nil && *req.Priority != oldPriority {
		logTaskAction(id, userID, "priority_changed", oldPriority, *req.Priority)
	}

	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

// ============================================================
// DELETE TASK
// ============================================================
func DeleteTask(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	_, err := config.DB.Exec("DELETE FROM tasks WHERE id=? AND user_id=?", id, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// ============================================================
// TOGGLE TASK STATUS
// ============================================================
func ToggleTaskStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var currentStatus string
	err := config.DB.QueryRow(
		"SELECT status FROM tasks WHERE id=? AND user_id=?",
		id, userID).Scan(&currentStatus)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Task tidak ditemukan"})
		return
	}

	newStatus := "done"
	if currentStatus == "done" {
		newStatus = "pending"
	}

	var query string
	if newStatus == "done" {
		query = "UPDATE tasks SET status = ?, completed_at = NOW() WHERE id = ? AND user_id = ?"
	} else {
		query = "UPDATE tasks SET status = ?, completed_at = NULL WHERE id = ? AND user_id = ?"
	}

	_, _ = config.DB.Exec(query, newStatus, id, userID)

	logTaskAction(id, userID, "status_changed", currentStatus, newStatus)

	writeJSON(w, 200, map[string]interface{}{
		"status": newStatus,
	})
}

// ============================================================
// TASK STATS — FIXED VERSION
// ============================================================
func TaskStats(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	stats := map[string]int{
		"total":       0,
		"pending":     0,
		"in_progress": 0,
		"done":        0,
		"cancelled":   0,
		"overdue":     0,
		"today":       0,
		"week":        0,
		"urgent":      0,
		"high":        0,
	}

	// ---- Status counts ----
	rows, err := config.DB.Query(`
		SELECT status, COUNT(*) FROM tasks WHERE user_id = ? GROUP BY status`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int
			_ = rows.Scan(&status, &count)
			stats[status] = count
			stats["total"] += count
		}
	}

	// ---- Overdue ----
	var overdue int
	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM tasks
		WHERE user_id = ? AND due_date < NOW()
		  AND status NOT IN ('done','cancelled')`, userID).Scan(&overdue)
	stats["overdue"] = overdue

	// ---- Today ----
	var today int
	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM tasks
		WHERE user_id = ? AND DATE(due_date) = CURDATE()
		  AND status NOT IN ('done','cancelled')`, userID).Scan(&today)
	stats["today"] = today

	// ---- This week ----
	var week int
	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM tasks
		WHERE user_id = ?
		  AND due_date BETWEEN NOW() AND DATE_ADD(NOW(), INTERVAL 7 DAY)
		  AND status NOT IN ('done','cancelled')`, userID).Scan(&week)
	stats["week"] = week

	// ---- Urgent ----
	var urgent int
	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM tasks
		WHERE user_id = ? AND priority = 'urgent'
		  AND status NOT IN ('done','cancelled')`, userID).Scan(&urgent)
	stats["urgent"] = urgent

	// ---- High priority ----
	var high int
	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM tasks
		WHERE user_id = ? AND priority = 'high'
		  AND status NOT IN ('done','cancelled')`, userID).Scan(&high)
	stats["high"] = high

	writeJSON(w, 200, stats)
}

// ============================================================
// SYNC CHECKLIST → TASKS
// ============================================================
func SyncChecklistToTasks(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "note_id")

	var content string
	var notebookID sql.NullInt64
	err := config.DB.QueryRow(`
		SELECT content, notebook_id FROM notes
		WHERE id = ? AND user_id = ?`, noteID, userID).
		Scan(&content, &notebookID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Note tidak ditemukan"})
		return
	}

	tasks := parseChecklistItems(content)

	created := 0
	for _, t := range tasks {
		var nbArg interface{}
		if notebookID.Valid {
			nbArg = notebookID.Int64
		}

		_, err := config.DB.Exec(`
			INSERT INTO tasks (user_id, note_id, notebook_id, title, status)
			VALUES (?, ?, ?, ?, ?)`,
			userID, noteID, nbArg, t.Text, t.Status)
		if err == nil {
			created++
		}
	}

	writeJSON(w, 200, map[string]interface{}{
		"created": created,
		"total":   len(tasks),
	})
}

type checklistItem struct {
	Text   string
	Status string
}

func parseChecklistItems(html string) []checklistItem {
	items := []checklistItem{}
	lines := strings.Split(html, "<li")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "data-list=") {
			continue
		}

		var status string
		if strings.Contains(line, `data-list="checked"`) {
			status = "done"
		} else if strings.Contains(line, `data-list="unchecked"`) {
			status = "pending"
		} else {
			continue
		}

		start := strings.Index(line, ">")
		end := strings.Index(line, "</li>")
		if start < 0 || end < 0 || end <= start {
			continue
		}

		text := line[start+1 : end]
		text = stripHTML(text)
		text = strings.TrimSpace(text)

		if text != "" {
			items = append(items, checklistItem{Text: text, Status: status})
		}
	}

	return items
}

// ============================================================
// TASK REMINDER (dipanggil cron)
// ============================================================
func CheckDueReminders() {
	rows, err := config.DB.Query(`
		SELECT t.id, t.user_id, t.title, t.due_date
		FROM tasks t
		WHERE t.reminder_at <= NOW()
		  AND t.reminder_sent = FALSE
		  AND t.status NOT IN ('done','cancelled')`)
	if err != nil {
		return
	}
	defer rows.Close()

	type rem struct {
		id      int64
		userID  int64
		title   string
		dueDate sql.NullTime
	}
	list := []rem{}

	for rows.Next() {
		var rr rem
		_ = rows.Scan(&rr.id, &rr.userID, &rr.title, &rr.dueDate)
		list = append(list, rr)
	}

	for _, rr := range list {
		msg := "Task: " + rr.title
		if rr.dueDate.Valid {
			msg += " — jatuh tempo " + rr.dueDate.Time.Format("2 Jan 2006 15:04")
		}

		_, _ = config.DB.Exec(`
			INSERT INTO notifications (user_id, type, title, message, link)
			VALUES (?, 'task_reminder', ?, ?, ?)`,
			rr.userID, "Pengingat Tugas", msg, "/tasks?id="+strconv.FormatInt(rr.id, 10))

		_, _ = config.DB.Exec(
			"UPDATE tasks SET reminder_sent=TRUE WHERE id=?", rr.id)
	}
}

// ============================================================
// TASK HISTORY
// ============================================================
func logTaskAction(taskID, userID int64, action, oldVal, newVal string) {
	var oldArg, newArg interface{}
	if oldVal != "" {
		oldArg = oldVal
	}
	if newVal != "" {
		newArg = newVal
	}
	_, _ = config.DB.Exec(`
		INSERT INTO task_history (task_id, user_id, action, old_value, new_value)
		VALUES (?, ?, ?, ?, ?)`,
		taskID, userID, action, oldArg, newArg)
}

func ListTaskHistory(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	taskID, _ := pathInt(r, "id")

	rows, err := config.DB.Query(`
		SELECT id, action, COALESCE(old_value,''), COALESCE(new_value,''), created_at
		FROM task_history
		WHERE task_id = ? AND user_id = ?
		ORDER BY created_at DESC
		LIMIT 50`, taskID, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var action, oldVal, newVal string
		var t time.Time
		_ = rows.Scan(&id, &action, &oldVal, &newVal, &t)
		result = append(result, map[string]interface{}{
			"id":        id,
			"action":    action,
			"old_value": oldVal,
			"new_value": newVal,
			"time":      t,
		})
	}
	writeJSON(w, 200, result)
}

// ============================================================
// HELPER: Parse datetime dari berbagai format
// ============================================================
func parseDateTimeInput(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		time.RFC3339,
	}

	for _, format := range formats {
		if t, err := time.ParseInLocation(format, s, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, &time.ParseError{Value: s}
}
