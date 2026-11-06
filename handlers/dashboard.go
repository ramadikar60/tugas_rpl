package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ============================================================
// DASHBOARD & ANALYTICS — Section 15
// ============================================================

// nullIntToInt — helper konversi sql.NullInt64 ke int (0 jika NULL)
func nullIntToInt(n sql.NullInt64) int {
	if n.Valid {
		return int(n.Int64)
	}
	return 0
}

// DashboardStats — ringkasan semua statistik
func DashboardStats(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	result := map[string]interface{}{}

	// ---- Notes stats ----
	noteStats := map[string]int{
		"total": 0, "active": 0, "archived": 0, "trashed": 0,
		"favorites": 0, "pinned": 0, "today": 0, "this_week": 0, "this_month": 0,
	}

	var (
		nTotal, nActive, nArchived, nTrashed sql.NullInt64
		nFavorites, nPinned, nToday          sql.NullInt64
		nThisWeek, nThisMonth                sql.NullInt64
	)

	err := config.DB.QueryRow(`
		SELECT 
		    COUNT(*),
		    COALESCE(SUM(CASE WHEN is_trashed=FALSE AND is_archived=FALSE THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN is_archived=TRUE AND is_trashed=FALSE THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN is_trashed=TRUE THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN is_favorite=TRUE THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN is_pinned=TRUE THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN DATE(created_at)=CURDATE() THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN YEARWEEK(created_at)=YEARWEEK(NOW()) THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN MONTH(created_at)=MONTH(NOW()) AND YEAR(created_at)=YEAR(NOW()) THEN 1 ELSE 0 END), 0)
		FROM notes WHERE user_id = ?`, userID).
		Scan(&nTotal, &nActive, &nArchived, &nTrashed, &nFavorites, &nPinned,
			&nToday, &nThisWeek, &nThisMonth)

	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "notes stats: " + err.Error()})
		return
	}

	noteStats["total"] = nullIntToInt(nTotal)
	noteStats["active"] = nullIntToInt(nActive)
	noteStats["archived"] = nullIntToInt(nArchived)
	noteStats["trashed"] = nullIntToInt(nTrashed)
	noteStats["favorites"] = nullIntToInt(nFavorites)
	noteStats["pinned"] = nullIntToInt(nPinned)
	noteStats["today"] = nullIntToInt(nToday)
	noteStats["this_week"] = nullIntToInt(nThisWeek)
	noteStats["this_month"] = nullIntToInt(nThisMonth)
	result["notes"] = noteStats

	// ---- Tasks stats ----
	taskStats := map[string]int{
		"total": 0, "pending": 0, "in_progress": 0, "done": 0, "cancelled": 0,
		"overdue": 0, "today": 0, "this_week": 0, "completion_rate": 0,
	}

	var (
		tTotal, tPending, tInProgress, tDone, tCancelled sql.NullInt64
		tOverdue, tToday, tThisWeek                      sql.NullInt64
	)

	err = config.DB.QueryRow(`
		SELECT 
		    COUNT(*),
		    COALESCE(SUM(CASE WHEN status='pending' THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN status='in_progress' THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN status='done' THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN status='cancelled' THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN due_date < NOW() AND status NOT IN ('done','cancelled') THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN DATE(due_date)=CURDATE() AND status NOT IN ('done','cancelled') THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN completed_at IS NOT NULL AND YEARWEEK(completed_at)=YEARWEEK(NOW()) THEN 1 ELSE 0 END), 0)
		FROM tasks WHERE user_id = ?`, userID).
		Scan(&tTotal, &tPending, &tInProgress, &tDone, &tCancelled,
			&tOverdue, &tToday, &tThisWeek)

	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "tasks stats: " + err.Error()})
		return
	}

	taskStats["total"] = nullIntToInt(tTotal)
	taskStats["pending"] = nullIntToInt(tPending)
	taskStats["in_progress"] = nullIntToInt(tInProgress)
	taskStats["done"] = nullIntToInt(tDone)
	taskStats["cancelled"] = nullIntToInt(tCancelled)
	taskStats["overdue"] = nullIntToInt(tOverdue)
	taskStats["today"] = nullIntToInt(tToday)
	taskStats["this_week"] = nullIntToInt(tThisWeek)

	if taskStats["total"] > 0 {
		taskStats["completion_rate"] = (taskStats["done"] * 100) / taskStats["total"]
	}
	result["tasks"] = taskStats

	// ---- Storage ----
	var totalSize sql.NullInt64
	var attachmentCount int
	_ = config.DB.QueryRow(`
		SELECT COALESCE(SUM(size), 0), COUNT(*) FROM attachments WHERE user_id = ?`,
		userID).Scan(&totalSize, &attachmentCount)

	var quota sql.NullInt64
	_ = config.DB.QueryRow("SELECT storage_quota FROM users WHERE id=?", userID).Scan(&quota)

	usedSize := int64(0)
	if totalSize.Valid {
		usedSize = totalSize.Int64
	}

	quotaVal := int64(0)
	if quota.Valid {
		quotaVal = quota.Int64
	}

	storage := map[string]interface{}{
		"used":             usedSize,
		"quota":            quotaVal,
		"attachment_count": attachmentCount,
		"percent":          0,
	}
	if quotaVal > 0 {
		storage["percent"] = (usedSize * 100) / quotaVal
	}
	result["storage"] = storage

	// ---- Productivity ----
	prodStats := map[string]int{
		"pomodoro_today": 0, "pomodoro_week": 0,
		"active_habits": 0, "active_goals": 0, "completed_goals": 0,
	}

	var pomToday, pomWeek, activeHabits, activeGoals, completedGoals int

	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM pomodoro_sessions
		WHERE user_id = ? AND DATE(started_at)=CURDATE() AND completed=TRUE`,
		userID).Scan(&pomToday)
	prodStats["pomodoro_today"] = pomToday

	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM pomodoro_sessions
		WHERE user_id = ? AND YEARWEEK(started_at)=YEARWEEK(NOW()) AND completed=TRUE`,
		userID).Scan(&pomWeek)
	prodStats["pomodoro_week"] = pomWeek

	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM habits WHERE user_id = ? AND is_active = TRUE`,
		userID).Scan(&activeHabits)
	prodStats["active_habits"] = activeHabits

	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM goals WHERE user_id = ? AND status='active'`,
		userID).Scan(&activeGoals)
	prodStats["active_goals"] = activeGoals

	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM goals WHERE user_id = ? AND status='completed'`,
		userID).Scan(&completedGoals)
	prodStats["completed_goals"] = completedGoals

	result["productivity"] = prodStats

	writeJSON(w, 200, result)
}

// ProductivityChart — data untuk chart (7 hari / 30 hari)
//
// Query params:
//
//	days  — default 30
func ProductivityChart(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	daysStr := r.URL.Query().Get("days")
	days := 30
	if daysStr != "" {
		if n, err := parseInt(daysStr); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}

	startDate := time.Now().AddDate(0, 0, -days+1).Format("2006-01-02")

	// Query per hari: notes created, tasks completed, pomodoro sessions
	rows, err := config.DB.Query(`
		SELECT 
		    DATE(d.day) AS date,
		    COALESCE((SELECT COUNT(*) FROM notes WHERE user_id = ? AND DATE(created_at) = DATE(d.day)), 0) AS notes_created,
		    COALESCE((SELECT COUNT(*) FROM tasks WHERE user_id = ? AND DATE(completed_at) = DATE(d.day)), 0) AS tasks_completed,
		    COALESCE((SELECT COUNT(*) FROM pomodoro_sessions WHERE user_id = ? AND DATE(started_at) = DATE(d.day) AND completed = TRUE), 0) AS pomodoro_count
		FROM (
		    SELECT DATE(?) + INTERVAL seq DAY AS day
		    FROM (
		        SELECT 0 AS seq UNION SELECT 1 UNION SELECT 2 UNION SELECT 3 UNION SELECT 4
		        UNION SELECT 5 UNION SELECT 6 UNION SELECT 7 UNION SELECT 8 UNION SELECT 9
		        UNION SELECT 10 UNION SELECT 11 UNION SELECT 12 UNION SELECT 13 UNION SELECT 14
		        UNION SELECT 15 UNION SELECT 16 UNION SELECT 17 UNION SELECT 18 UNION SELECT 19
		        UNION SELECT 20 UNION SELECT 21 UNION SELECT 22 UNION SELECT 23 UNION SELECT 24
		        UNION SELECT 25 UNION SELECT 26 UNION SELECT 27 UNION SELECT 28 UNION SELECT 29
		    ) seq_table
		) d
		WHERE d.day <= CURDATE()
		ORDER BY d.day ASC`,
		userID, userID, userID, startDate)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var date time.Time
		var notesCreated, tasksCompleted, pomodoroCount int
		_ = rows.Scan(&date, &notesCreated, &tasksCompleted, &pomodoroCount)
		result = append(result, map[string]interface{}{
			"date":            date.Format("2006-01-02"),
			"notes_created":   notesCreated,
			"tasks_completed": tasksCompleted,
			"pomodoro_count":  pomodoroCount,
		})
	}

	writeJSON(w, 200, result)
}

// TopItems — most used tags, notes, folders
func TopItems(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	result := map[string]interface{}{}

	// Top tags
	tagRows, _ := config.DB.Query(`
		SELECT t.id, t.name, t.color, COUNT(nt.note_id) AS usage_count
		FROM tags t
		LEFT JOIN note_tags nt ON nt.tag_id = t.id
		WHERE t.user_id = ?
		GROUP BY t.id, t.name, t.color
		ORDER BY usage_count DESC, t.name ASC
		LIMIT 10`, userID)
	tags := []map[string]interface{}{}
	if tagRows != nil {
		defer tagRows.Close()
		for tagRows.Next() {
			var id int64
			var name, color string
			var count int
			_ = tagRows.Scan(&id, &name, &color, &count)
			tags = append(tags, map[string]interface{}{
				"id": id, "name": name, "color": color, "usage_count": count,
			})
		}
	}
	result["top_tags"] = tags

	// Top notebooks (folder dengan catatan terbanyak)
	nbRows, _ := config.DB.Query(`
		SELECT nb.id, nb.name, nb.color, COUNT(n.id) AS note_count
		FROM notebooks nb
		LEFT JOIN notes n ON n.notebook_id = nb.id AND n.is_trashed = FALSE
		WHERE nb.user_id = ?
		GROUP BY nb.id, nb.name, nb.color
		ORDER BY note_count DESC, nb.name ASC
		LIMIT 10`, userID)
	folders := []map[string]interface{}{}
	if nbRows != nil {
		defer nbRows.Close()
		for nbRows.Next() {
			var id int64
			var name, color string
			var count int
			_ = nbRows.Scan(&id, &name, &color, &count)
			folders = append(folders, map[string]interface{}{
				"id": id, "name": name, "color": color, "note_count": count,
			})
		}
	}
	result["top_folders"] = folders

	// Most edited notes (recently updated)
	noteRows, _ := config.DB.Query(`
		SELECT id, title, updated_at, word_count
		FROM notes WHERE user_id = ? AND is_trashed = FALSE
		ORDER BY updated_at DESC LIMIT 10`, userID)
	notes := []map[string]interface{}{}
	if noteRows != nil {
		defer noteRows.Close()
		for noteRows.Next() {
			var id int64
			var title string
			var updatedAt time.Time
			var wordCount int
			_ = noteRows.Scan(&id, &title, &updatedAt, &wordCount)
			notes = append(notes, map[string]interface{}{
				"id": id, "title": title, "updated_at": updatedAt, "word_count": wordCount,
			})
		}
	}
	result["recent_notes"] = notes

	writeJSON(w, 200, result)
}
