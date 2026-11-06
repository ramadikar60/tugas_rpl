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
// PROJECTS — Section 6
// ============================================================

// ListProjects — daftar project user
func ListProjects(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	status := q.Get("status")
	archived := q.Get("archived") == "true"

	query := `SELECT id, user_id, name, COALESCE(description,''), icon, color,
	          status, progress, start_date, due_date, completed_at,
	          is_favorite, is_archived, created_at, updated_at
	          FROM projects WHERE user_id = ?`
	args := []interface{}{userID}

	if archived {
		query += " AND is_archived = TRUE"
	} else {
		query += " AND is_archived = FALSE"
	}

	if status != "" && status != "all" {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY is_favorite DESC, updated_at DESC"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, uid int64
		var name, description, icon, color, status string
		var progress int
		var startDate, dueDate, completedAt sql.NullTime
		var isFavorite, isArchived bool
		var createdAt, updatedAt time.Time

		_ = rows.Scan(&id, &uid, &name, &description, &icon, &color,
			&status, &progress, &startDate, &dueDate, &completedAt,
			&isFavorite, &isArchived, &createdAt, &updatedAt)

		item := map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
			"icon":        icon,
			"color":       color,
			"status":      status,
			"progress":    progress,
			"is_favorite": isFavorite,
			"is_archived": isArchived,
			"created_at":  createdAt,
			"updated_at":  updatedAt,
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

		// Count stats
		var noteCount, taskCount, taskDone int
		_ = config.DB.QueryRow("SELECT COUNT(*) FROM project_notes WHERE project_id=?", id).Scan(&noteCount)
		_ = config.DB.QueryRow("SELECT COUNT(*) FROM tasks WHERE project_id=?", id).Scan(&taskCount)
		_ = config.DB.QueryRow("SELECT COUNT(*) FROM tasks WHERE project_id=? AND status='done'", id).Scan(&taskDone)

		item["note_count"] = noteCount
		item["task_count"] = taskCount
		item["task_done"] = taskDone

		result = append(result, item)
	}

	writeJSON(w, 200, result)
}

// GetProject — detail project + semua relasi
func GetProject(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var name, description, icon, color, status string
	var progress int
	var startDate, dueDate, completedAt sql.NullTime
	var isFavorite, isArchived bool
	var createdAt, updatedAt time.Time

	err := config.DB.QueryRow(`
		SELECT name, COALESCE(description,''), icon, color, status, progress,
		       start_date, due_date, completed_at, is_favorite, is_archived,
		       created_at, updated_at
		FROM projects WHERE id = ? AND user_id = ?`,
		id, userID).
		Scan(&name, &description, &icon, &color, &status, &progress,
			&startDate, &dueDate, &completedAt, &isFavorite, &isArchived,
			&createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		writeJSON(w, 404, map[string]string{"error": "Project tidak ditemukan"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	project := map[string]interface{}{
		"id":          id,
		"name":        name,
		"description": description,
		"icon":        icon,
		"color":       color,
		"status":      status,
		"progress":    progress,
		"is_favorite": isFavorite,
		"is_archived": isArchived,
		"created_at":  createdAt,
		"updated_at":  updatedAt,
	}
	if startDate.Valid {
		project["start_date"] = startDate.Time.Format("2006-01-02")
	}
	if dueDate.Valid {
		project["due_date"] = dueDate.Time.Format("2006-01-02")
	}
	if completedAt.Valid {
		project["completed_at"] = completedAt.Time
	}

	// Notes
	noteRows, _ := config.DB.Query(`
		SELECT n.id, n.title, n.updated_at FROM notes n
		JOIN project_notes pn ON pn.note_id = n.id
		WHERE pn.project_id = ? AND n.is_trashed = FALSE
		ORDER BY pn.added_at DESC`, id)
	notes := []map[string]interface{}{}
	if noteRows != nil {
		defer noteRows.Close()
		for noteRows.Next() {
			var nID int64
			var title string
			var updatedAt time.Time
			_ = noteRows.Scan(&nID, &title, &updatedAt)
			notes = append(notes, map[string]interface{}{
				"id":         nID,
				"title":      title,
				"updated_at": updatedAt,
			})
		}
	}
	project["notes"] = notes

	// Tasks
	taskRows, _ := config.DB.Query(`
		SELECT id, title, status, priority, due_date FROM tasks
		WHERE project_id = ?
		ORDER BY 
		    FIELD(status, 'in_progress', 'pending', 'done', 'cancelled'),
		    due_date IS NULL, due_date ASC`, id)
	tasks := []map[string]interface{}{}
	if taskRows != nil {
		defer taskRows.Close()
		for taskRows.Next() {
			var tID int64
			var title, status, priority string
			var dueDate sql.NullTime
			_ = taskRows.Scan(&tID, &title, &status, &priority, &dueDate)
			t := map[string]interface{}{
				"id":       tID,
				"title":    title,
				"status":   status,
				"priority": priority,
			}
			if dueDate.Valid {
				t["due_date"] = dueDate.Time
			}
			tasks = append(tasks, t)
		}
	}
	project["tasks"] = tasks

	// Activity
	actRows, _ := config.DB.Query(`
		SELECT pa.action, COALESCE(pa.description,''), pa.created_at,
		       COALESCE(u.username,'') AS username
		FROM project_activity pa
		LEFT JOIN users u ON u.id = pa.user_id
		WHERE pa.project_id = ?
		ORDER BY pa.created_at DESC
		LIMIT 30`, id)
	activities := []map[string]interface{}{}
	if actRows != nil {
		defer actRows.Close()
		for actRows.Next() {
			var action, description, username string
			var t time.Time
			_ = actRows.Scan(&action, &description, &t, &username)
			activities = append(activities, map[string]interface{}{
				"action":      action,
				"description": description,
				"username":    username,
				"created_at":  t,
			})
		}
	}
	project["activities"] = activities

	// Stats
	var noteCount, taskCount, taskDone int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM project_notes WHERE project_id=?", id).Scan(&noteCount)
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM tasks WHERE project_id=?", id).Scan(&taskCount)
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM tasks WHERE project_id=? AND status='done'", id).Scan(&taskDone)

	project["note_count"] = noteCount
	project["task_count"] = taskCount
	project["task_done"] = taskDone

	// Auto-calculate progress
	if taskCount > 0 {
		autoProgress := (taskDone * 100) / taskCount
		project["auto_progress"] = autoProgress
	} else {
		project["auto_progress"] = 0
	}

	writeJSON(w, 200, project)
}

// CreateProject
func CreateProject(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Icon        string  `json:"icon"`
		Color       string  `json:"color"`
		StartDate   *string `json:"start_date"`
		DueDate     *string `json:"due_date"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "Nama project wajib diisi"})
		return
	}

	if req.Icon == "" {
		req.Icon = "folder"
	}
	if req.Color == "" {
		req.Color = "#6366f1"
	}

	var startArg, dueArg interface{}
	if req.StartDate != nil && *req.StartDate != "" {
		startArg = *req.StartDate
	}
	if req.DueDate != nil && *req.DueDate != "" {
		dueArg = *req.DueDate
	}

	res, err := config.DB.Exec(`
		INSERT INTO projects (user_id, name, description, icon, color, start_date, due_date)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, req.Name, req.Description, req.Icon, req.Color, startArg, dueArg)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	// Log activity
	_, _ = config.DB.Exec(`
		INSERT INTO project_activity (project_id, user_id, action, description)
		VALUES (?, ?, 'created', ?)`,
		id, userID, "Project \""+req.Name+"\" dibuat")

	writeJSON(w, 201, map[string]interface{}{"id": id, "name": req.Name})
}

// UpdateProject
func UpdateProject(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Icon        *string `json:"icon"`
		Color       *string `json:"color"`
		Status      *string `json:"status"`
		Progress    *int    `json:"progress"`
		StartDate   *string `json:"start_date"`
		DueDate     *string `json:"due_date"`
		IsFavorite  *bool   `json:"is_favorite"`
		IsArchived  *bool   `json:"is_archived"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE projects SET "
	args := []interface{}{}
	add := func(frag string, v interface{}) {
		query += frag
		args = append(args, v)
	}

	if req.Name != nil {
		add("name=?, ", *req.Name)
	}
	if req.Description != nil {
		add("description=?, ", *req.Description)
	}
	if req.Icon != nil {
		add("icon=?, ", *req.Icon)
	}
	if req.Color != nil {
		add("color=?, ", *req.Color)
	}
	if req.Status != nil {
		add("status=?, ", *req.Status)
		// Auto set completed_at
		if *req.Status == "completed" {
			query += "completed_at=NOW(), "
		}
	}
	if req.Progress != nil {
		add("progress=?, ", *req.Progress)
	}
	if req.StartDate != nil {
		if *req.StartDate == "" {
			query += "start_date=NULL, "
		} else {
			add("start_date=?, ", *req.StartDate)
		}
	}
	if req.DueDate != nil {
		if *req.DueDate == "" {
			query += "due_date=NULL, "
		} else {
			add("due_date=?, ", *req.DueDate)
		}
	}
	if req.IsFavorite != nil {
		add("is_favorite=?, ", *req.IsFavorite)
	}
	if req.IsArchived != nil {
		add("is_archived=?, ", *req.IsArchived)
	}

	if len(args) == 0 && !strings.Contains(query, "NOW()") && !strings.Contains(query, "NULL") {
		writeJSON(w, 400, map[string]string{"error": "Tidak ada field"})
		return
	}

	query = strings.TrimSuffix(query, ", ")
	query += " WHERE id = ? AND user_id = ?"
	args = append(args, id, userID)

	if _, err := config.DB.Exec(query, args...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Log
	if req.Status != nil {
		_, _ = config.DB.Exec(`
			INSERT INTO project_activity (project_id, user_id, action, description)
			VALUES (?, ?, 'status_changed', ?)`,
			id, userID, "Status diubah ke "+*req.Status)
	}

	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

// DeleteProject
func DeleteProject(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	_, err := config.DB.Exec(
		"DELETE FROM projects WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// AddNoteToProject
func AddNoteToProject(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	projectID, _ := pathInt(r, "id")

	var req struct {
		NoteID int64 `json:"note_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	// Verifikasi ownership project & note
	var count int
	_ = config.DB.QueryRow(
		"SELECT COUNT(*) FROM projects WHERE id=? AND user_id=?",
		projectID, userID).Scan(&count)
	if count == 0 {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}
	_ = config.DB.QueryRow(
		"SELECT COUNT(*) FROM notes WHERE id=? AND user_id=?",
		req.NoteID, userID).Scan(&count)
	if count == 0 {
		writeJSON(w, 403, map[string]string{"error": "Note tidak ditemukan"})
		return
	}

	_, _ = config.DB.Exec(
		"INSERT IGNORE INTO project_notes (project_id, note_id) VALUES (?, ?)",
		projectID, req.NoteID)

	_, _ = config.DB.Exec(`
		INSERT INTO project_activity (project_id, user_id, action, description)
		VALUES (?, ?, 'note_added', 'Catatan ditambahkan ke project')`,
		projectID, userID)

	writeJSON(w, 200, map[string]string{"message": "Note ditambahkan"})
}

// RemoveNoteFromProject
func RemoveNoteFromProject(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	projectID, _ := pathInt(r, "id")
	noteID, _ := pathInt(r, "note_id")

	_, _ = config.DB.Exec(`
		DELETE pn FROM project_notes pn
		JOIN projects p ON p.id = pn.project_id
		WHERE pn.project_id = ? AND pn.note_id = ? AND p.user_id = ?`,
		projectID, noteID, userID)

	writeJSON(w, 200, map[string]string{"message": "Note dihapus dari project"})
}
