package handlers

import (
	"net/http"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ============ LIST TAGS ============
func ListTags(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	rows, err := config.DB.Query(
		"SELECT id, name, color FROM tags WHERE user_id=? ORDER BY name", userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	tags := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name, color string
		_ = rows.Scan(&id, &name, &color)
		tags = append(tags, map[string]interface{}{
			"id":    id,
			"name":  name,
			"color": color,
		})
	}
	writeJSON(w, 200, tags)
}

// ============ CREATE TAG ============
func CreateTag(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}
	if req.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "Nama tag wajib diisi"})
		return
	}
	if req.Color == "" {
		req.Color = "#6366f1"
	}

	res, err := config.DB.Exec(
		"INSERT INTO tags (user_id, name, color) VALUES (?, ?, ?)",
		userID, req.Name, req.Color)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Tag sudah ada"})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{
		"id":    id,
		"name":  req.Name,
		"color": req.Color,
	})
}

// ============ UPDATE TAG ============
func UpdateTag(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE tags SET "
	args := []interface{}{}
	if req.Name != nil {
		query += "name=?, "
		args = append(args, *req.Name)
	}
	if req.Color != nil {
		query += "color=?, "
		args = append(args, *req.Color)
	}

	if len(args) == 0 {
		writeJSON(w, 400, map[string]string{"error": "Tidak ada field"})
		return
	}
	query = query[:len(query)-2] + " WHERE id=? AND user_id=?"
	args = append(args, id, userID)

	_, err := config.DB.Exec(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

// ============ DELETE TAG ============
func DeleteTag(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	_, _ = config.DB.Exec("DELETE FROM tags WHERE id=? AND user_id=?", id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// ============ ATTACH TAG TO NOTE ============
func AttachTagToNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "note_id")
	tagID, _ := pathInt(r, "tag_id")

	// Verifikasi kepemilikan note
	var count int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM notes WHERE id=? AND user_id=?",
		noteID, userID).Scan(&count)
	if count == 0 {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}

	// Verifikasi kepemilikan tag
	var tagCount int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM tags WHERE id=? AND user_id=?",
		tagID, userID).Scan(&tagCount)
	if tagCount == 0 {
		writeJSON(w, 403, map[string]string{"error": "Tag tidak ditemukan"})
		return
	}

	_, _ = config.DB.Exec("INSERT IGNORE INTO note_tags (note_id, tag_id) VALUES (?, ?)",
		noteID, tagID)
	writeJSON(w, 200, map[string]string{"message": "Tag ditambahkan"})
}

// ============ DETACH TAG FROM NOTE ============
func DetachTagFromNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "note_id")
	tagID, _ := pathInt(r, "tag_id")

	// Verifikasi kepemilikan note
	var count int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM notes WHERE id=? AND user_id=?",
		noteID, userID).Scan(&count)
	if count == 0 {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}

	_, _ = config.DB.Exec("DELETE FROM note_tags WHERE note_id=? AND tag_id=?",
		noteID, tagID)
	writeJSON(w, 200, map[string]string{"message": "Tag dihapus"})
}

// ============ MOST USED TAGS ============
func MostUsedTags(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	rows, err := config.DB.Query(`
		SELECT t.id, t.name, t.color, COUNT(nt.note_id) AS usage_count
		FROM tags t
		LEFT JOIN note_tags nt ON nt.tag_id = t.id
		WHERE t.user_id = ?
		GROUP BY t.id, t.name, t.color
		ORDER BY usage_count DESC, t.name ASC
		LIMIT 20`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name, color string
		var count int
		_ = rows.Scan(&id, &name, &color, &count)
		result = append(result, map[string]interface{}{
			"id":          id,
			"name":        name,
			"color":       color,
			"usage_count": count,
		})
	}
	writeJSON(w, 200, result)
}
