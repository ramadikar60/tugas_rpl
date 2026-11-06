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
// POMODORO TIMER — Section 14
// ============================================================

// StartPomodoro — mulai sesi pomodoro
func StartPomodoro(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		TaskID   *int64 `json:"task_id"`
		Duration int    `json:"duration"`
		Type     string `json:"type"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	if req.Duration <= 0 {
		req.Duration = 25
	}
	if req.Type == "" {
		req.Type = "focus"
	}

	res, err := config.DB.Exec(`
		INSERT INTO pomodoro_sessions (user_id, task_id, duration, type)
		VALUES (?, ?, ?, ?)`,
		userID, req.TaskID, req.Duration, req.Type)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{
		"id":       id,
		"duration": req.Duration,
		"type":     req.Type,
	})
}

// CompletePomodoro — tandai sesi selesai
func CompletePomodoro(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	_, err := config.DB.Exec(`
		UPDATE pomodoro_sessions SET completed = TRUE, ended_at = NOW()
		WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Sesi selesai"})
}

// PomodoroStats — statistik pomodoro hari ini & minggu ini
func PomodoroStats(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	// Pakai map[string]int karena semua value adalah angka
	stats := map[string]int{
		"today_sessions": 0,
		"today_minutes":  0,
		"week_sessions":  0,
		"week_minutes":   0,
		"total_sessions": 0,
		"total_minutes":  0,
	}

	// ---- Today ----
	var todaySessions, todayMinutes int
	err := config.DB.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(duration), 0)
		FROM pomodoro_sessions
		WHERE user_id = ? AND DATE(started_at) = CURDATE() 
		  AND type = 'focus' AND completed = TRUE`,
		userID).Scan(&todaySessions, &todayMinutes)
	if err == nil {
		stats["today_sessions"] = todaySessions
		stats["today_minutes"] = todayMinutes
	}

	// ---- This week ----
	var weekSessions, weekMinutes int
	err = config.DB.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(duration), 0)
		FROM pomodoro_sessions
		WHERE user_id = ? AND YEARWEEK(started_at) = YEARWEEK(NOW()) 
		  AND type = 'focus' AND completed = TRUE`,
		userID).Scan(&weekSessions, &weekMinutes)
	if err == nil {
		stats["week_sessions"] = weekSessions
		stats["week_minutes"] = weekMinutes
	}

	// ---- Total ----
	var totalSessions, totalMinutes int
	err = config.DB.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(duration), 0)
		FROM pomodoro_sessions
		WHERE user_id = ? AND type = 'focus' AND completed = TRUE`,
		userID).Scan(&totalSessions, &totalMinutes)
	if err == nil {
		stats["total_sessions"] = totalSessions
		stats["total_minutes"] = totalMinutes
	}

	writeJSON(w, 200, stats)
}

// ============================================================
// DAILY PLANNER
// ============================================================

// GetDailyPlan — ambil plan untuk tanggal tertentu
func GetDailyPlan(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	dateStr := r.URL.Query().Get("date")

	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Format tanggal salah"})
		return
	}

	var id int64
	var notes, focus string
	var mood, energy int
	var isReviewed bool
	var createdAt, updatedAt time.Time

	err := config.DB.QueryRow(`
		SELECT id, COALESCE(notes,''), COALESCE(focus,''), mood, energy, is_reviewed,
		       created_at, updated_at
		FROM daily_plans WHERE user_id = ? AND plan_date = ?`,
		userID, dateStr).Scan(&id, &notes, &focus, &mood, &energy, &isReviewed, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		// Belum ada plan
		writeJSON(w, 200, map[string]interface{}{
			"date":   dateStr,
			"empty":  true,
			"mood":   3,
			"energy": 3,
		})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Ambil tasks untuk hari ini
	tasks := []map[string]interface{}{}
	rows, err := config.DB.Query(`
		SELECT id, title, status, priority FROM tasks
		WHERE user_id = ? AND DATE(COALESCE(due_date, created_at)) = ?
		ORDER BY FIELD(priority, 'urgent', 'high', 'medium', 'low'), created_at`,
		userID, dateStr)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var tid int64
			var title, status, priority string
			if err := rows.Scan(&tid, &title, &status, &priority); err == nil {
				tasks = append(tasks, map[string]interface{}{
					"id":       tid,
					"title":    title,
					"status":   status,
					"priority": priority,
				})
			}
		}
	}

	// Ambil notes untuk hari ini
	notesArr := []map[string]interface{}{}
	rows2, err := config.DB.Query(`
		SELECT id, title FROM notes
		WHERE user_id = ? AND DATE(created_at) = ? AND is_trashed = FALSE
		ORDER BY created_at`, userID, dateStr)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var nid int64
			var title string
			if err := rows2.Scan(&nid, &title); err == nil {
				notesArr = append(notesArr, map[string]interface{}{
					"id":    nid,
					"title": title,
				})
			}
		}
	}

	writeJSON(w, 200, map[string]interface{}{
		"id":          id,
		"date":        dateStr,
		"notes":       notes,
		"focus":       focus,
		"mood":        mood,
		"energy":      energy,
		"is_reviewed": isReviewed,
		"tasks":       tasks,
		"day_notes":   notesArr,
		"created_at":  createdAt,
		"updated_at":  updatedAt,
	})
}

// SaveDailyPlan — simpan/update plan
func SaveDailyPlan(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Date       string `json:"date"`
		Notes      string `json:"notes"`
		Focus      string `json:"focus"`
		Mood       int    `json:"mood"`
		Energy     int    `json:"energy"`
		IsReviewed bool   `json:"is_reviewed"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	if req.Date == "" {
		req.Date = time.Now().Format("2006-01-02")
	}
	if req.Mood <= 0 {
		req.Mood = 3
	}
	if req.Energy <= 0 {
		req.Energy = 3
	}

	// Upsert
	res, err := config.DB.Exec(`
		INSERT INTO daily_plans (user_id, plan_date, notes, focus, mood, energy, is_reviewed)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		    notes = VALUES(notes),
		    focus = VALUES(focus),
		    mood = VALUES(mood),
		    energy = VALUES(energy),
		    is_reviewed = VALUES(is_reviewed)`,
		userID, req.Date, req.Notes, req.Focus, req.Mood, req.Energy, req.IsReviewed)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 200, map[string]interface{}{"id": id, "message": "Saved"})
}

// ListDailyPlans — list plan terakhir untuk history
func ListDailyPlans(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	rows, err := config.DB.Query(`
		SELECT plan_date, COALESCE(focus,''), mood, energy, is_reviewed
		FROM daily_plans WHERE user_id = ?
		ORDER BY plan_date DESC LIMIT 30`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var dateStr string
		var focus string
		var mood, energy int
		var isReviewed bool
		if err := rows.Scan(&dateStr, &focus, &mood, &energy, &isReviewed); err == nil {
			result = append(result, map[string]interface{}{
				"date":        dateStr,
				"focus":       focus,
				"mood":        mood,
				"energy":      energy,
				"is_reviewed": isReviewed,
			})
		}
	}

	writeJSON(w, 200, result)
}

// ============================================================
// HABITS
// ============================================================

// ListHabits — daftar habit + status hari ini
func ListHabits(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	today := time.Now().Format("2006-01-02")

	rows, err := config.DB.Query(`
		SELECT h.id, h.name, COALESCE(h.description,''), h.icon, h.color,
		       h.frequency, h.target_per_week, h.is_active,
		       COALESCE((SELECT value FROM habit_logs hl 
		                 WHERE hl.habit_id = h.id AND hl.log_date = ?), 0) AS today_value,
		       COALESCE((SELECT COUNT(*) FROM habit_logs hl 
		                 WHERE hl.habit_id = h.id 
		                 AND YEARWEEK(hl.log_date) = YEARWEEK(NOW())), 0) AS week_count
		FROM habits h
		WHERE h.user_id = ? AND h.is_active = TRUE
		ORDER BY h.created_at DESC`, today, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name, description, icon, color, frequency string
		var targetPerWeek, todayValue, weekCount int
		var isActive bool

		if err := rows.Scan(&id, &name, &description, &icon, &color, &frequency,
			&targetPerWeek, &isActive, &todayValue, &weekCount); err != nil {
			continue
		}

		result = append(result, map[string]interface{}{
			"id":              id,
			"name":            name,
			"description":     description,
			"icon":            icon,
			"color":           color,
			"frequency":       frequency,
			"target_per_week": targetPerWeek,
			"is_active":       isActive,
			"today_value":     todayValue,
			"week_count":      weekCount,
		})
	}

	writeJSON(w, 200, result)
}

// CreateHabit
func CreateHabit(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		Icon          string `json:"icon"`
		Color         string `json:"color"`
		Frequency     string `json:"frequency"`
		TargetPerWeek int    `json:"target_per_week"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "Nama habit wajib diisi"})
		return
	}

	if req.Icon == "" {
		req.Icon = "star"
	}
	if req.Color == "" {
		req.Color = "#6366f1"
	}
	if req.Frequency == "" {
		req.Frequency = "daily"
	}
	if req.TargetPerWeek <= 0 {
		req.TargetPerWeek = 7
	}

	res, err := config.DB.Exec(`
		INSERT INTO habits (user_id, name, description, icon, color, frequency, target_per_week)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, req.Name, req.Description, req.Icon, req.Color, req.Frequency, req.TargetPerWeek)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": id, "name": req.Name})
}

// DeleteHabit
func DeleteHabit(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	if _, err := config.DB.Exec(
		"DELETE FROM habits WHERE id=? AND user_id=?", id, userID); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// ToggleHabit — log/unlog habit hari ini
func ToggleHabit(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Date  string `json:"date"`
		Value int    `json:"value"` // 1 = done, 0 = undo
	}
	_ = readJSON(r, &req)

	if req.Date == "" {
		req.Date = time.Now().Format("2006-01-02")
	}

	// Verifikasi ownership
	var count int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM habits WHERE id=? AND user_id=?",
		id, userID).Scan(&count)
	if count == 0 {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}

	if req.Value == 0 {
		// Hapus log
		if _, err := config.DB.Exec(
			"DELETE FROM habit_logs WHERE habit_id=? AND log_date=?",
			id, req.Date); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]interface{}{"value": 0})
	} else {
		// Upsert log
		if _, err := config.DB.Exec(`
			INSERT INTO habit_logs (habit_id, user_id, log_date, value)
			VALUES (?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE value = VALUES(value)`,
			id, userID, req.Date, req.Value); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]interface{}{"value": req.Value})
	}
}

// HabitHeatmap — data untuk heatmap 1 habit
func HabitHeatmap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	// 90 hari terakhir
	startDate := time.Now().AddDate(0, 0, -90).Format("2006-01-02")

	rows, err := config.DB.Query(`
		SELECT log_date, value FROM habit_logs
		WHERE habit_id = ? AND user_id = ? AND log_date >= ?
		ORDER BY log_date ASC`, id, userID, startDate)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var dateStr string
		var value int
		if err := rows.Scan(&dateStr, &value); err == nil {
			result = append(result, map[string]interface{}{
				"date":  dateStr,
				"value": value,
			})
		}
	}

	writeJSON(w, 200, result)
}

// ============================================================
// GOALS
// ============================================================

// ListGoals
func ListGoals(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	status := r.URL.Query().Get("status")

	query := `SELECT id, title, COALESCE(description,''), COALESCE(category,''),
	          goal_type, target_value, current_value, unit,
	          start_date, due_date, status, completed_at, created_at
	          FROM goals WHERE user_id = ?`
	args := []interface{}{userID}

	if status != "" && status != "all" {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY FIELD(status, 'active', 'paused', 'completed', 'cancelled'), created_at DESC"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var title, description, category, goalType, unit, statusVal string
		var targetValue, currentValue int
		var startDate, dueDate, completedAt sql.NullTime
		var createdAt time.Time

		if err := rows.Scan(&id, &title, &description, &category, &goalType,
			&targetValue, &currentValue, &unit, &startDate, &dueDate,
			&statusVal, &completedAt, &createdAt); err != nil {
			continue
		}

		progress := 0
		if targetValue > 0 {
			progress = (currentValue * 100) / targetValue
			if progress > 100 {
				progress = 100
			}
		}

		item := map[string]interface{}{
			"id":            id,
			"title":         title,
			"description":   description,
			"category":      category,
			"goal_type":     goalType,
			"target_value":  targetValue,
			"current_value": currentValue,
			"unit":          unit,
			"status":        statusVal,
			"progress":      progress,
			"created_at":    createdAt,
		}
		if startDate.Valid {
			item["start_date"] = startDate.Time.Format("2006-01-02")
		}
		if dueDate.Valid {
			item["due_date"] = dueDate.Time.Format("2006-01-02")
		}
		if completedAt.Valid {
			item["completed_at"] = completedAt.Time
		}
		result = append(result, item)
	}

	writeJSON(w, 200, result)
}

// CreateGoal
func CreateGoal(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Category    string  `json:"category"`
		GoalType    string  `json:"goal_type"`
		TargetValue int     `json:"target_value"`
		Unit        string  `json:"unit"`
		StartDate   *string `json:"start_date"`
		DueDate     *string `json:"due_date"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Title) == "" {
		writeJSON(w, 400, map[string]string{"error": "Judul goal wajib diisi"})
		return
	}

	if req.GoalType == "" {
		req.GoalType = "short_term"
	}
	if req.TargetValue <= 0 {
		req.TargetValue = 100
	}
	if req.Unit == "" {
		req.Unit = "%"
	}

	var startArg, dueArg interface{}
	if req.StartDate != nil && *req.StartDate != "" {
		startArg = *req.StartDate
	}
	if req.DueDate != nil && *req.DueDate != "" {
		dueArg = *req.DueDate
	}

	res, err := config.DB.Exec(`
		INSERT INTO goals (user_id, title, description, category, goal_type,
		                   target_value, unit, start_date, due_date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, req.Title, req.Description, req.Category, req.GoalType,
		req.TargetValue, req.Unit, startArg, dueArg)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": id, "title": req.Title})
}

// UpdateGoalProgress
func UpdateGoalProgress(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		CurrentValue *int    `json:"current_value"`
		Status       *string `json:"status"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE goals SET "
	args := []interface{}{}

	if req.CurrentValue != nil {
		query += "current_value=?, "
		args = append(args, *req.CurrentValue)
	}
	if req.Status != nil {
		query += "status=?, "
		args = append(args, *req.Status)
		if *req.Status == "completed" {
			query += "completed_at=NOW(), "
		}
	}

	if len(args) == 0 {
		writeJSON(w, 400, map[string]string{"error": "Tidak ada field"})
		return
	}

	query = strings.TrimSuffix(query, ", ")
	query += " WHERE id=? AND user_id=?"
	args = append(args, id, userID)

	if _, err := config.DB.Exec(query, args...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

// DeleteGoal
func DeleteGoal(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	if _, err := config.DB.Exec(
		"DELETE FROM goals WHERE id=? AND user_id=?", id, userID); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}
