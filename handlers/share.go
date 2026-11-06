package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"time"

	"notebook-app/auth"
	"notebook-app/config"
	"notebook-app/middleware"
)

// generateToken — random 32-byte hex
func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ============================================================
// CREATE SHARE
// ============================================================
func CreateShare(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		ItemType         string  `json:"item_type"`
		ItemID           int64   `json:"item_id"`
		ShareMode        string  `json:"share_mode"`
		Permission       string  `json:"permission"`
		SharedWithUserID *int64  `json:"shared_with_user_id"`
		Password         string  `json:"password"`
		ExpiresAt        *string `json:"expires_at"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	if req.ItemType != "note" && req.ItemType != "notebook" {
		writeJSON(w, 400, map[string]string{"error": "Item type harus note atau notebook"})
		return
	}
	if req.ShareMode == "" {
		req.ShareMode = "private"
	}
	if req.Permission == "" {
		req.Permission = "viewer"
	}

	// Verifikasi ownership
	var table string
	if req.ItemType == "note" {
		table = "notes"
	} else {
		table = "notebooks"
	}
	var count int
	_ = config.DB.QueryRow(
		"SELECT COUNT(*) FROM "+table+" WHERE id=? AND user_id=?",
		req.ItemID, userID).Scan(&count)
	if count == 0 {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}

	// Ambil judul
	var title string
	_ = config.DB.QueryRow(
		"SELECT title FROM notes WHERE id=?",
		req.ItemID).Scan(&title)
	if title == "" {
		_ = config.DB.QueryRow(
			"SELECT name FROM notebooks WHERE id=?",
			req.ItemID).Scan(&title)
	}

	// Generate token kalau mode public
	var tokenArg interface{}
	if req.ShareMode == "public" {
		tokenArg = generateToken()
	}

	// Hash password kalau ada
	var pwArg interface{}
	if req.Password != "" {
		h, err := auth.HashPassword(req.Password)
		if err == nil {
			pwArg = h
		}
	}

	// Expires at
	var expiresArg interface{}
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		if t, err := time.Parse("2006-01-02", *req.ExpiresAt); err == nil {
			expiresArg = t
		}
	}

	res, err := config.DB.Exec(`
		INSERT INTO shares 
		    (user_id, item_type, item_id, share_mode, permission, token, 
		     shared_with_user_id, title, expires_at, password_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, req.ItemType, req.ItemID, req.ShareMode, req.Permission,
		tokenArg, req.SharedWithUserID, title, expiresArg, pwArg)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	// Kalau share ke user lain, buat notifikasi dengan info lengkap
	if req.SharedWithUserID != nil {
		var ownerName string
		_ = config.DB.QueryRow("SELECT username FROM users WHERE id=?", userID).Scan(&ownerName)

		permText := "hanya lihat"
		if req.Permission == "editor" {
			permText = "bisa edit"
		}

		_, _ = config.DB.Exec(`
			INSERT INTO notifications (user_id, type, title, message, link)
			VALUES (?, 'share', 'Catatan dibagikan', ?, ?)`,
			*req.SharedWithUserID,
			"@"+ownerName+" membagikan catatan \""+title+"\" kepada Anda ("+permText+")",
			"/notes?id="+intToStr(req.ItemID))
	}

	writeJSON(w, 201, map[string]interface{}{"id": id, "token": tokenArg})
}

// ============================================================
// LIST SHARES
// ============================================================
func ListShares(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	itemType := r.URL.Query().Get("item_type")
	itemID := r.URL.Query().Get("item_id")

	query := `
		SELECT s.id, s.item_type, s.item_id, s.share_mode, s.permission, 
		       COALESCE(s.token,''), s.shared_with_user_id, COALESCE(s.title,''),
		       s.expires_at, s.is_active, s.view_count, s.created_at,
		       COALESCE(u.username,''), COALESCE(u.email,'')
		FROM shares s
		LEFT JOIN users u ON u.id = s.shared_with_user_id
		WHERE s.user_id = ?`
	args := []interface{}{userID}

	if itemType != "" {
		query += " AND s.item_type = ?"
		args = append(args, itemType)
	}
	if itemID != "" {
		query += " AND s.item_id = ?"
		args = append(args, itemID)
	}
	query += " ORDER BY s.created_at DESC"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, itemID2 int64
		var itemType, shareMode, permission, token, title, username, email string
		var sharedWith sql.NullInt64
		var expiresAt sql.NullTime
		var isActive bool
		var viewCount int
		var createdAt sql.NullTime

		_ = rows.Scan(&id, &itemType, &itemID2, &shareMode, &permission, &token,
			&sharedWith, &title, &expiresAt, &isActive, &viewCount, &createdAt,
			&username, &email)

		item := map[string]interface{}{
			"id":         id,
			"item_type":  itemType,
			"item_id":    itemID2,
			"share_mode": shareMode,
			"permission": permission,
			"token":      token,
			"title":      title,
			"is_active":  isActive,
			"view_count": viewCount,
		}
		if sharedWith.Valid {
			item["shared_with_user_id"] = sharedWith.Int64
			item["shared_with_name"] = username
			item["shared_with_email"] = email
		}
		if expiresAt.Valid {
			item["expires_at"] = expiresAt.Time
		}
		if createdAt.Valid {
			item["created_at"] = createdAt.Time
		}
		result = append(result, item)
	}
	writeJSON(w, 200, result)
}

// ============================================================
// UPDATE / REVOKE SHARE
// ============================================================
func UpdateSharePermission(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	if !ownsShare(userID, id) {
		writeJSON(w, 404, map[string]string{"error": "Berbagi tidak ditemukan"})
		return
	}

	var req struct {
		Permission string `json:"permission"`
		IsActive   *bool  `json:"is_active"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE shares SET "
	args := []interface{}{}
	if req.Permission != "" {
		query += "permission=?, "
		args = append(args, req.Permission)
	}
	if req.IsActive != nil {
		query += "is_active=?, "
		args = append(args, *req.IsActive)
	}
	if len(args) == 0 {
		writeJSON(w, 400, map[string]string{"error": "No changes"})
		return
	}
	query = query[:len(query)-2] + " WHERE id=? AND user_id=?"
	args = append(args, id, userID)

	_, _ = config.DB.Exec(query, args...)
	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

func RevokeShare(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	if !ownsShare(userID, id) {
		writeJSON(w, 404, map[string]string{"error": "Berbagi tidak ditemukan"})
		return
	}

	_, err := config.DB.Exec(
		"DELETE FROM shares WHERE id=? AND user_id=?", id, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Share dicabut"})
}

// ============================================================
// ACCESS SHARED CONTENT (public, no auth)
// ============================================================
func AccessSharedNote(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeJSON(w, 400, map[string]string{"error": "Token wajib diisi"})
		return
	}

	password := r.URL.Query().Get("password")

	var id, userID, itemID int64
	var itemType, shareMode, permission, title string
	var passwordHash sql.NullString
	var expiresAt sql.NullTime
	var isActive bool

	err := config.DB.QueryRow(`
		SELECT id, user_id, item_type, item_id, share_mode, permission,
		       COALESCE(title,''), password_hash, expires_at, is_active
		FROM shares WHERE token = ?`, token).
		Scan(&id, &userID, &itemType, &itemID, &shareMode, &permission,
			&title, &passwordHash, &expiresAt, &isActive)

	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Link tidak ditemukan"})
		return
	}

	if !isActive {
		writeJSON(w, 403, map[string]string{"error": "Link sudah dinonaktifkan"})
		return
	}

	if expiresAt.Valid && time.Now().After(expiresAt.Time) {
		writeJSON(w, 403, map[string]string{"error": "Link sudah kadaluarsa"})
		return
	}

	if passwordHash.Valid && passwordHash.String != "" {
		if !auth.VerifyPassword(passwordHash.String, password) {
			writeJSON(w, 401, map[string]string{"error": "Password salah atau kosong"})
			return
		}
	}

	_, _ = config.DB.Exec("UPDATE shares SET view_count = view_count + 1 WHERE id=?", id)

	if itemType == "note" {
		var noteTitle, content string
		var nbID sql.NullInt64
		err := config.DB.QueryRow(`
			SELECT title, content, notebook_id FROM notes
			WHERE id = ? AND is_trashed = FALSE`, itemID).
			Scan(&noteTitle, &content, &nbID)
		if err != nil {
			writeJSON(w, 404, map[string]string{"error": "Catatan tidak ditemukan"})
			return
		}
		result := map[string]interface{}{
			"type":       "note",
			"id":         itemID,
			"title":      noteTitle,
			"content":    sanitizeNoteHTML(content),
			"permission": permission,
		}
		if nbID.Valid {
			result["notebook_id"] = nbID.Int64
		}
		writeJSON(w, 200, result)
	} else {
		var nbName string
		_ = config.DB.QueryRow(
			"SELECT name FROM notebooks WHERE id=?", itemID).Scan(&nbName)

		rows, _ := config.DB.Query(`
			SELECT id, title, updated_at FROM notes
			WHERE notebook_id = ? AND is_trashed = FALSE
			ORDER BY updated_at DESC LIMIT 100`, itemID)
		notes := []map[string]interface{}{}
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var nID int64
				var nTitle string
				var updatedAt time.Time
				_ = rows.Scan(&nID, &nTitle, &updatedAt)
				notes = append(notes, map[string]interface{}{
					"id":         nID,
					"title":      nTitle,
					"updated_at": updatedAt,
				})
			}
		}

		writeJSON(w, 200, map[string]interface{}{
			"type":       "notebook",
			"id":         itemID,
			"title":      nbName,
			"notes":      notes,
			"permission": permission,
		})
	}
}

// ============================================================
// LIST SHARED WITH ME — Info lengkap
// ============================================================
func ListSharedWithMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	rows, err := config.DB.Query(`
		SELECT s.id, s.item_type, s.item_id, s.share_mode, s.permission,
		       COALESCE(s.title,''), s.created_at,
		       COALESCE(u.username,'') AS owner_name,
		       COALESCE(u.email,'') AS owner_email,
		       s.view_count, s.expires_at
		FROM shares s
		JOIN users u ON u.id = s.user_id
		WHERE s.shared_with_user_id = ? 
		  AND s.is_active = TRUE
		  AND (s.expires_at IS NULL OR s.expires_at > NOW())
		ORDER BY s.created_at DESC`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, itemID int64
		var itemType, shareMode, permission, title, ownerName, ownerEmail string
		var t sql.NullTime
		var expiresAt sql.NullTime
		var viewCount int

		_ = rows.Scan(&id, &itemType, &itemID, &shareMode, &permission,
			&title, &t, &ownerName, &ownerEmail, &viewCount, &expiresAt)

		item := map[string]interface{}{
			"id":          id,
			"item_type":   itemType,
			"item_id":     itemID,
			"share_mode":  shareMode,
			"permission":  permission,
			"title":       title,
			"owner_name":  ownerName,
			"owner_email": ownerEmail,
			"view_count":  viewCount,
			"created_at":  t.Time,
		}
		if expiresAt.Valid {
			item["expires_at"] = expiresAt.Time
		}
		result = append(result, item)
	}
	writeJSON(w, 200, result)
}
