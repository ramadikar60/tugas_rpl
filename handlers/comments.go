package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"notebook-app/config"
	"notebook-app/middleware"
	"notebook-app/models"
)

// ============================================================
// LIST COMMENTS (nested)
// ============================================================

func ListComments(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "id")

	// Cek ownership ATAU shared access
	var ownerID int64
	err := config.DB.QueryRow(
		"SELECT user_id FROM notes WHERE id=?", noteID).Scan(&ownerID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Note tidak ditemukan"})
		return
	}

	if ownerID != userID {
		var count int
		_ = config.DB.QueryRow(`
			SELECT COUNT(*) FROM shares 
			WHERE item_type='note' AND item_id=? AND shared_with_user_id=? 
			  AND is_active=TRUE`,
			noteID, userID).Scan(&count)
		if count == 0 {
			writeJSON(w, 403, map[string]string{"error": "Forbidden"})
			return
		}
	}

	rows, err := config.DB.Query(`
		SELECT c.id, c.note_id, c.user_id, c.parent_id, c.content,
		       c.is_resolved, c.created_at, c.updated_at,
		       COALESCE(u.username,''), COALESCE(u.avatar,'')
		FROM comments c
		JOIN users u ON u.id = c.user_id
		WHERE c.note_id = ?
		ORDER BY c.created_at ASC`, noteID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	all := map[int64]*models.Comment{}
	roots := []*models.Comment{}

	for rows.Next() {
		c := &models.Comment{}
		var parentID sql.NullInt64
		_ = rows.Scan(&c.ID, &c.NoteID, &c.UserID, &parentID, &c.Content,
			&c.IsResolved, &c.CreatedAt, &c.UpdatedAt, &c.Username, &c.Avatar)
		if parentID.Valid {
			v := parentID.Int64
			c.ParentID = &v
		}
		all[c.ID] = c
	}

	// Build tree
	for _, c := range all {
		if c.ParentID == nil {
			roots = append(roots, c)
		} else if parent, ok := all[*c.ParentID]; ok {
			parent.Replies = append(parent.Replies, c)
		}
	}

	writeJSON(w, 200, roots)
}

// ============================================================
// CREATE COMMENT
// ============================================================

func CreateComment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "id")

	var req struct {
		Content  string `json:"content"`
		ParentID *int64 `json:"parent_id"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Content) == "" {
		writeJSON(w, 400, map[string]string{"error": "Konten komentar wajib diisi"})
		return
	}

	// Verifikasi akses
	var noteOwnerID int64
	err := config.DB.QueryRow(
		"SELECT user_id FROM notes WHERE id=?", noteID).Scan(&noteOwnerID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Note tidak ditemukan"})
		return
	}

	if noteOwnerID != userID {
		var count int
		_ = config.DB.QueryRow(`
			SELECT COUNT(*) FROM shares 
			WHERE item_type='note' AND item_id=? AND shared_with_user_id=? 
			  AND is_active=TRUE`,
			noteID, userID).Scan(&count)
		if count == 0 {
			writeJSON(w, 403, map[string]string{"error": "Forbidden"})
			return
		}
	}

	res, err := config.DB.Exec(`
		INSERT INTO comments (note_id, user_id, parent_id, content)
		VALUES (?, ?, ?, ?)`,
		noteID, userID, req.ParentID, req.Content)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	// Notif ke owner note (kalau bukan diri sendiri)
	if noteOwnerID != userID {
		var username string
		_ = config.DB.QueryRow("SELECT username FROM users WHERE id=?", userID).Scan(&username)

		_, _ = config.DB.Exec(`
			INSERT INTO notifications (user_id, type, title, message, link)
			VALUES (?, 'comment', 'Komentar baru', ?, ?)`,
			noteOwnerID,
			"@"+username+" mengomentari catatan Anda",
			"/notes?id="+intToStr(noteID))
	}

	writeJSON(w, 201, map[string]interface{}{"id": id})
}

// ============================================================
// UPDATE / DELETE / RESOLVE COMMENT
// ============================================================

func UpdateComment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	commentID, _ := pathInt(r, "id")

	var req struct {
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	_, err := config.DB.Exec(
		"UPDATE comments SET content=? WHERE id=? AND user_id=?",
		req.Content, commentID, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

func DeleteComment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	commentID, _ := pathInt(r, "id")

	_, _ = config.DB.Exec(
		"DELETE FROM comments WHERE id=? AND user_id=?", commentID, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

func ResolveComment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	commentID, _ := pathInt(r, "id")

	var req struct {
		IsResolved bool `json:"is_resolved"`
	}
	_ = readJSON(r, &req)

	if req.IsResolved {
		_, _ = config.DB.Exec(`
			UPDATE comments SET is_resolved=TRUE, 
			       resolved_by=?, resolved_at=NOW()
			WHERE id=?`, userID, commentID)
	} else {
		_, _ = config.DB.Exec(`
			UPDATE comments SET is_resolved=FALSE, 
			       resolved_by=NULL, resolved_at=NULL
			WHERE id=?`, commentID)
	}

	writeJSON(w, 200, map[string]string{"message": "Updated"})
}
