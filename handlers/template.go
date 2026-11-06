package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ============ LIST TEMPLATES ============
func ListTemplates(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	query := `SELECT id, user_id, name, COALESCE(description,''), category, icon,
	          COALESCE(icon_color,'#18181b'), content, is_system, is_favorite,
	          usage_count, COALESCE(tags,''), sort_order, created_at, updated_at
	          FROM templates
	          WHERE (user_id = ? OR is_system = TRUE)`
	args := []interface{}{userID}

	if cat := q.Get("category"); cat != "" && cat != "all" {
		query += " AND category = ?"
		args = append(args, cat)
	}
	if fav := q.Get("favorite"); fav == "true" {
		query += " AND is_favorite = TRUE"
	}
	if kw := strings.TrimSpace(q.Get("q")); kw != "" {
		query += " AND (name LIKE ? OR description LIKE ?)"
		like := "%" + kw + "%"
		args = append(args, like, like)
	}

	query += " ORDER BY is_system DESC, usage_count DESC, sort_order ASC, name ASC"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var uid sql.NullInt64
		var name, desc, cat, icon, iconColor, content, tags string
		var isSystem, isFavorite bool
		var usageCount, sortOrder int
		var createdAt, updatedAt interface{}

		_ = rows.Scan(&id, &uid, &name, &desc, &cat, &icon, &iconColor,
			&content, &isSystem, &isFavorite, &usageCount, &tags,
			&sortOrder, &createdAt, &updatedAt)

		result = append(result, map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": desc,
			"category":    cat,
			"icon":        icon,
			"icon_color":  iconColor,
			"content":     content,
			"is_system":   isSystem,
			"is_favorite": isFavorite,
			"usage_count": usageCount,
			"tags":        tags,
			"sort_order":  sortOrder,
		})
	}
	writeJSON(w, 200, result)
}

// ============ CREATE TEMPLATE ============
func CreateTemplate(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Category    string `json:"category"`
		Icon        string `json:"icon"`
		IconColor   string `json:"icon_color"`
		Content     string `json:"content"`
		Tags        string `json:"tags"`
		NoteID      *int64 `json:"note_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "Nama template wajib diisi"})
		return
	}

	// Kalau dari note, ambil kontennya
	if req.NoteID != nil {
		var title, content string
		err := config.DB.QueryRow(
			"SELECT title, content FROM notes WHERE id=? AND user_id=?",
			*req.NoteID, userID).Scan(&title, &content)
		if err == nil {
			req.Content = content
		}
	}

	req.Content = sanitizeNoteHTML(req.Content)

	if req.Category == "" {
		req.Category = "general"
	}
	if req.IconColor == "" {
		req.IconColor = "#18181b"
	}
	if req.Icon == "" {
		req.Icon = "doc"
	}

	res, err := config.DB.Exec(`
		INSERT INTO templates (user_id, name, description, category, icon, icon_color, content, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, req.Name, req.Description, req.Category, req.Icon, req.IconColor,
		req.Content, req.Tags)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": id, "name": req.Name})
}

// ============ UPDATE TEMPLATE ============
func UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Category    *string `json:"category"`
		Icon        *string `json:"icon"`
		IconColor   *string `json:"icon_color"`
		Content     *string `json:"content"`
		Tags        *string `json:"tags"`
		IsFavorite  *bool   `json:"is_favorite"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE templates SET "
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
	if req.Category != nil {
		add("category=?, ", *req.Category)
	}
	if req.Icon != nil {
		add("icon=?, ", *req.Icon)
	}
	if req.IconColor != nil {
		add("icon_color=?, ", *req.IconColor)
	}
	if req.Content != nil {
		add("content=?, ", sanitizeNoteHTML(*req.Content))
	}
	if req.Tags != nil {
		add("tags=?, ", *req.Tags)
	}
	if req.IsFavorite != nil {
		add("is_favorite=?, ", *req.IsFavorite)
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

// ============ DELETE TEMPLATE ============
func DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	_, err := config.DB.Exec(
		"DELETE FROM templates WHERE id=? AND user_id=? AND is_system=FALSE",
		id, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// ============ DUPLICATE TEMPLATE ============
func DuplicateTemplate(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var name, desc, cat, icon, iconColor, content, tags string
	err := config.DB.QueryRow(`
		SELECT name, COALESCE(description,''), category, icon,
		       COALESCE(icon_color,'#18181b'), content, COALESCE(tags,'')
		FROM templates WHERE id = ? AND (user_id = ? OR is_system = TRUE)`,
		id, userID).Scan(&name, &desc, &cat, &icon, &iconColor, &content, &tags)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Template tidak ditemukan"})
		return
	}

	res, err := config.DB.Exec(`
		INSERT INTO templates (user_id, name, description, category, icon, icon_color, content, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, name+" (Copy)", desc, cat, icon, iconColor, sanitizeNoteHTML(content), tags)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	newID, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": newID})
}

// ============ CREATE NOTE FROM TEMPLATE ============
func CreateNoteFromTemplate(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		TemplateID int64  `json:"template_id"`
		NotebookID *int64 `json:"notebook_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	var name, content string
	err := config.DB.QueryRow(`
		SELECT name, content FROM templates
		WHERE id = ? AND (user_id = ? OR is_system = TRUE)`,
		req.TemplateID, userID).Scan(&name, &content)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Template tidak ditemukan"})
		return
	}

	content = sanitizeNoteHTML(content)
	text := stripHTML(content)
	words := len(strings.Fields(text))
	chars := len([]rune(text))

	res, err := config.DB.Exec(`
		INSERT INTO notes (user_id, notebook_id, title, content, content_text, word_count, char_count)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, req.NotebookID, name, content, text, words, chars)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	// Increment usage
	_, _ = config.DB.Exec(
		"UPDATE templates SET usage_count = usage_count + 1 WHERE id = ?",
		req.TemplateID)

	writeJSON(w, 201, map[string]interface{}{"id": id, "title": name})
}

// ============ LIST CATEGORIES ============
func ListTemplateCategories(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	rows, err := config.DB.Query(`
		SELECT DISTINCT category FROM templates
		WHERE user_id = ? OR is_system = TRUE
		ORDER BY category`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []string{}
	for rows.Next() {
		var cat string
		_ = rows.Scan(&cat)
		result = append(result, cat)
	}
	writeJSON(w, 200, result)
}
