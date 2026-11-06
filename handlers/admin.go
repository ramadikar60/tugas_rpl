package handlers

import (
	"database/sql"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"notebook-app/auth"
	"notebook-app/config"
	"notebook-app/middleware"
	"notebook-app/models"
)

// ============================================================
// RBAC MIDDLEWARE
// ============================================================

// RequireRole — middleware helper untuk check role
func RequireRole(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			userID := middleware.GetUserID(r)
			var role string
			err := config.DB.QueryRow(
				"SELECT role FROM users WHERE id=?", userID).Scan(&role)
			if err != nil {
				http.Error(w, `{"error":"User not found"}`, http.StatusUnauthorized)
				return
			}
			for _, allowed := range roles {
				if role == allowed {
					next(w, r)
					return
				}
			}
			http.Error(w, `{"error":"Forbidden - role tidak diizinkan"}`, http.StatusForbidden)
		}
	}
}

// AdminRoute — route wrapper dengan middleware auth + role check
func AdminRoute(h http.HandlerFunc) http.HandlerFunc {
	return middleware.Chain(h, middleware.Auth, RequireRole("admin"))
}

// ============================================================
// ADMIN USER MANAGEMENT
// ============================================================

func AdminListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(`
		SELECT id, email, username, COALESCE(avatar,''), role, is_active,
		       COALESCE(last_login_at, created_at), created_at
		FROM users
		ORDER BY created_at DESC`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var email, username, avatar, role string
		var isActive bool
		var lastLogin, createdAt interface{}
		_ = rows.Scan(&id, &email, &username, &avatar, &role, &isActive,
			&lastLogin, &createdAt)
		result = append(result, map[string]interface{}{
			"id":            id,
			"email":         email,
			"username":      username,
			"avatar":        avatar,
			"role":          role,
			"is_active":     isActive,
			"last_login_at": lastLogin,
			"created_at":    createdAt,
		})
	}
	writeJSON(w, 200, result)
}

func AdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, _ := pathInt(r, "id")
	adminID := middleware.GetUserID(r)

	var req struct {
		Role     *string `json:"role"`
		IsActive *bool   `json:"is_active"`
		NewPass  *string `json:"new_password"`
		Username *string `json:"username"`
		Email    *string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	// Proteksi: admin tidak boleh ubah role dirinya sendiri jadi non-admin
	if id == adminID && req.Role != nil && *req.Role != "admin" {
		writeJSON(w, 400, map[string]string{"error": "Tidak bisa menurunkan role diri sendiri"})
		return
	}
	if id == adminID && req.IsActive != nil && !*req.IsActive {
		writeJSON(w, 400, map[string]string{"error": "Tidak bisa menonaktifkan diri sendiri"})
		return
	}

	query := "UPDATE users SET "
	args := []interface{}{}
	add := func(frag string, v interface{}) {
		query += frag
		args = append(args, v)
	}

	if req.Role != nil {
		if *req.Role != "admin" && *req.Role != "user" && *req.Role != "viewer" {
			writeJSON(w, 400, map[string]string{"error": "Role tidak valid"})
			return
		}
		add("role=?, ", *req.Role)
	}
	if req.IsActive != nil {
		add("is_active=?, ", *req.IsActive)
	}
	if req.Username != nil {
		add("username=?, ", *req.Username)
	}
	if req.Email != nil {
		add("email=?, ", *req.Email)
	}
	passwordChanged := false
	if req.NewPass != nil && *req.NewPass != "" {
		// Kebijakan sandi akun; email/username memakai nilai baru bila ikut diubah.
		var curEmail, curUser string
		_ = config.DB.QueryRow("SELECT email, username FROM users WHERE id=?", id).Scan(&curEmail, &curUser)
		if req.Email != nil {
			curEmail = *req.Email
		}
		if req.Username != nil {
			curUser = *req.Username
		}
		if err := auth.ValidateAccountPassword(*req.NewPass, curEmail, curUser); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		hash, err := auth.HashPassword(*req.NewPass)
		if err == nil {
			add("password_hash=?, ", hash)
			passwordChanged = true
		}
	}

	if len(args) == 0 {
		writeJSON(w, 400, map[string]string{"error": "No changes"})
		return
	}
	query = query[:len(query)-2] + " WHERE id=?"
	args = append(args, id)

	_, err := config.DB.Exec(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Sandi diganti atau akun dinonaktifkan: semua sesi pengguna itu berakhir.
	if passwordChanged || (req.IsActive != nil && !*req.IsActive) {
		revokeAllRefreshTokens(id)
	}

	// Log aktivitas admin
	LogActivity(adminID, "admin_update_user", "user", id,
		"Admin update user #"+strconv.FormatInt(id, 10),
		clientIP(r), r.UserAgent())

	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

func AdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, _ := pathInt(r, "id")
	adminID := middleware.GetUserID(r)

	// Cegah hapus diri sendiri
	if id == adminID {
		writeJSON(w, 400, map[string]string{"error": "Tidak bisa menghapus akun sendiri"})
		return
	}

	// Ambil info user untuk log
	var username, email string
	_ = config.DB.QueryRow("SELECT username, email FROM users WHERE id=?", id).
		Scan(&username, &email)

	_, err := config.DB.Exec("DELETE FROM users WHERE id=?", id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	LogActivity(adminID, "admin_delete_user", "user", id,
		"Admin hapus user: "+username+" ("+email+")",
		clientIP(r), r.UserAgent())

	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

func AdminGetUserDetail(w http.ResponseWriter, r *http.Request) {
	id, _ := pathInt(r, "id")

	var user models.User
	var role string
	var isActive bool
	err := config.DB.QueryRow(`
		SELECT id, email, username, COALESCE(avatar,''), role, is_active, created_at
		FROM users WHERE id = ?`, id).
		Scan(&user.ID, &user.Email, &user.Username, &user.Avatar, &role, &isActive, &user.CreatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "User tidak ditemukan"})
		return
	}

	var noteCount, notebookCount, taskCount int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM notes WHERE user_id=?", id).Scan(&noteCount)
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM notebooks WHERE user_id=?", id).Scan(&notebookCount)
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM tasks WHERE user_id=?", id).Scan(&taskCount)

	writeJSON(w, 200, map[string]interface{}{
		"user":           user,
		"role":           role,
		"is_active":      isActive,
		"note_count":     noteCount,
		"notebook_count": notebookCount,
		"task_count":     taskCount,
	})
}

// ============================================================
// ACTIVITY LOG
// ============================================================

// LogActivity — catat aktivitas user
func LogActivity(userID int64, action, itemType string, itemID int64, description, ip, ua string) {
	var itemArg interface{}
	if itemID != 0 {
		itemArg = itemID
	}
	_, _ = config.DB.Exec(`
		INSERT INTO activity_log (user_id, action, item_type, item_id, description, ip, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, action, itemType, itemArg, description, ip, ua)
}

var activityActionRe = regexp.MustCompile(`^[a-z_]+$`)

// ListActivityLog — audit log untuk user
func ListActivityLog(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	// Filter opsional ?action=login,logout (hanya huruf kecil dan garis bawah).
	where := "user_id = ?"
	args := []interface{}{userID}
	if af := r.URL.Query().Get("action"); af != "" {
		ph := ""
		for _, a := range strings.Split(af, ",") {
			if a != "" && len(a) <= 50 && activityActionRe.MatchString(a) {
				if ph != "" {
					ph += ","
				}
				ph += "?"
				args = append(args, a)
			}
		}
		if ph != "" {
			where += " AND action IN (" + ph + ")"
		}
	}
	args = append(args, limit)

	rows, err := config.DB.Query(`
		SELECT id, action, COALESCE(item_type,''), item_id,
		       COALESCE(description,''), COALESCE(ip,''), created_at
		FROM activity_log
		WHERE `+where+`
		ORDER BY created_at DESC LIMIT ?`, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var action, itemType, desc, ip string
		var itemID sql.NullInt64
		var t sql.NullTime
		_ = rows.Scan(&id, &action, &itemType, &itemID, &desc, &ip, &t)
		item := map[string]interface{}{
			"id":          id,
			"action":      action,
			"item_type":   itemType,
			"description": desc,
			"ip":          ip,
			"created_at":  t.Time,
		}
		if itemID.Valid {
			item["item_id"] = itemID.Int64
		}
		result = append(result, item)
	}
	writeJSON(w, 200, result)
}
