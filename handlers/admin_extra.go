package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"notebook-app/auth"
	"notebook-app/config"
)

// ============================================================
// CREATE USER (oleh admin)
// POST /api/admin/users
// ============================================================
func AdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"` // admin | user | viewer
		IsActive *bool  `json:"is_active"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.Username = strings.TrimSpace(req.Username)

	if req.Email == "" || req.Username == "" || req.Password == "" {
		writeJSON(w, 400, map[string]string{"error": "Email, username, dan password wajib diisi"})
		return
	}

	if req.Role == "" {
		req.Role = "user"
	}
	if req.Role != "admin" && req.Role != "user" && req.Role != "viewer" {
		writeJSON(w, 400, map[string]string{"error": "Role tidak valid (admin/user/viewer)"})
		return
	}

	// Email, username, dan password diperiksa dengan aturan yang sama seperti registrasi.
	if err := validateRegistration(req.Email, req.Username, req.Password); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	res, err := config.DB.Exec(`
		INSERT INTO users (email, username, password_hash, role, is_active)
		VALUES (?, ?, ?, ?, ?)`,
		req.Email, req.Username, hash, req.Role, isActive)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeJSON(w, 400, map[string]string{"error": "Email atau username sudah dipakai"})
			return
		}
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{
		"id":       id,
		"email":    req.Email,
		"username": req.Username,
		"role":     req.Role,
		"message":  "User berhasil dibuat",
	})
}

// ============================================================
// GLOBAL STATS
// GET /api/admin/stats
// ============================================================
func AdminStats(w http.ResponseWriter, r *http.Request) {
	stats := map[string]interface{}{}

	var totalUsers, activeUsers, adminUsers int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&totalUsers)
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM users WHERE is_active = TRUE").Scan(&activeUsers)
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminUsers)

	stats["users"] = map[string]int{
		"total":  totalUsers,
		"active": activeUsers,
		"admin":  adminUsers,
	}

	var totalNotes, trashedNotes, totalTasks, totalNotebooks int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM notes").Scan(&totalNotes)
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM notes WHERE is_trashed = TRUE").Scan(&trashedNotes)
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&totalTasks)
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM notebooks").Scan(&totalNotebooks)

	stats["content"] = map[string]int{
		"notes":     totalNotes,
		"trashed":   trashedNotes,
		"tasks":     totalTasks,
		"notebooks": totalNotebooks,
	}

	var totalAttachments int
	var totalSize sql.NullInt64
	_ = config.DB.QueryRow("SELECT COUNT(*), COALESCE(SUM(size), 0) FROM attachments").
		Scan(&totalAttachments, &totalSize)
	size := int64(0)
	if totalSize.Valid {
		size = totalSize.Int64
	}
	stats["storage"] = map[string]interface{}{
		"total_files": totalAttachments,
		"total_size":  size,
	}

	writeJSON(w, 200, stats)
}

// ============================================================
// LIST ALL NOTES (semua user)
// GET /api/admin/notes
// ============================================================
func AdminListNotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	query := `
		SELECT n.id, n.user_id, n.title,
		       COALESCE(u.username,'') AS username,
		       COALESCE(u.email,'') AS email,
		       n.word_count, n.is_trashed, n.created_at, n.updated_at
		FROM notes n
		LEFT JOIN users u ON u.id = n.user_id
		WHERE 1=1`
	args := []interface{}{}

	if uid := q.Get("user_id"); uid != "" {
		query += " AND n.user_id = ?"
		args = append(args, uid)
	}
	if kw := strings.TrimSpace(q.Get("q")); kw != "" {
		query += " AND n.title LIKE ?"
		args = append(args, "%"+kw+"%")
	}
	if q.Get("trashed") == "true" {
		query += " AND n.is_trashed = TRUE"
	} else if q.Get("trashed") == "false" {
		query += " AND n.is_trashed = FALSE"
	}

	limit := 100
	offset := 0
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v >= 0 {
		offset = v
	}

	query += " ORDER BY n.updated_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, userID int64
		var title, username, email string
		var wordCount int
		var isTrashed bool
		var createdAt, updatedAt time.Time

		_ = rows.Scan(&id, &userID, &title, &username, &email,
			&wordCount, &isTrashed, &createdAt, &updatedAt)

		result = append(result, map[string]interface{}{
			"id":         id,
			"user_id":    userID,
			"title":      title,
			"username":   username,
			"email":      email,
			"word_count": wordCount,
			"is_trashed": isTrashed,
			"created_at": createdAt,
			"updated_at": updatedAt,
		})
	}

	writeJSON(w, 200, result)
}

// ============================================================
// DELETE NOTE (oleh admin, tanpa cek owner)
// DELETE /api/admin/notes/{id}
// ============================================================
func AdminDeleteNote(w http.ResponseWriter, r *http.Request) {
	id, _ := pathInt(r, "id")

	_, err := config.DB.Exec("DELETE FROM notes WHERE id = ?", id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]string{"message": "Catatan dihapus permanen"})
}

// ============================================================
// LIST ALL TASKS (semua user)
// GET /api/admin/tasks
// ============================================================
func AdminListTasks(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(`
		SELECT t.id, t.user_id, t.title, t.status, t.priority,
		       COALESCE(u.username,''), COALESCE(u.email,''),
		       t.created_at
		FROM tasks t
		LEFT JOIN users u ON u.id = t.user_id
		ORDER BY t.created_at DESC LIMIT 200`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, userID int64
		var title, status, priority, username, email string
		var createdAt time.Time

		_ = rows.Scan(&id, &userID, &title, &status, &priority,
			&username, &email, &createdAt)

		result = append(result, map[string]interface{}{
			"id":         id,
			"user_id":    userID,
			"title":      title,
			"status":     status,
			"priority":   priority,
			"username":   username,
			"email":      email,
			"created_at": createdAt,
		})
	}

	writeJSON(w, 200, result)
}

// ============================================================
// GLOBAL ACTIVITY LOG
// GET /api/admin/activity
// ============================================================
func AdminActivity(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(`
		SELECT al.id, al.user_id, COALESCE(u.username,''), COALESCE(u.email,''),
		       al.action, COALESCE(al.item_type,''), al.item_id,
		       COALESCE(al.description,''), COALESCE(al.ip,''),
		       al.created_at
		FROM activity_log al
		LEFT JOIN users u ON u.id = al.user_id
		ORDER BY al.created_at DESC LIMIT 200`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, userID int64
		var username, email, action, itemType, description, ip string
		var itemID sql.NullInt64
		var createdAt time.Time

		_ = rows.Scan(&id, &userID, &username, &email, &action, &itemType,
			&itemID, &description, &ip, &createdAt)

		item := map[string]interface{}{
			"id":          id,
			"user_id":     userID,
			"username":    username,
			"email":       email,
			"action":      action,
			"item_type":   itemType,
			"description": description,
			"ip":          ip,
			"created_at":  createdAt,
		}
		if itemID.Valid {
			item["item_id"] = itemID.Int64
		}
		result = append(result, item)
	}

	writeJSON(w, 200, result)
}

// ============================================================
// STORAGE PER USER
// GET /api/admin/storage
// ============================================================
func AdminStorage(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(`
		SELECT u.id, u.username, u.email,
		       COALESCE(COUNT(a.id), 0) AS file_count,
		       COALESCE(SUM(a.size), 0) AS total_size
		FROM users u
		LEFT JOIN attachments a ON a.user_id = u.id
		GROUP BY u.id, u.username, u.email
		ORDER BY total_size DESC`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var username, email string
		var fileCount int
		var totalSize int64

		_ = rows.Scan(&id, &username, &email, &fileCount, &totalSize)

		result = append(result, map[string]interface{}{
			"user_id":    id,
			"username":   username,
			"email":      email,
			"file_count": fileCount,
			"total_size": totalSize,
		})
	}

	writeJSON(w, 200, result)
}
