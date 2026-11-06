package handlers

import (
	"database/sql"
	"net/http"

	"notebook-app/config"
	"notebook-app/middleware"
	"notebook-app/models"
)

// ============ LIST ============
func ListNotebooks(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	rows, err := config.DB.Query(`
		SELECT id, user_id, parent_id, name, icon, color,
		       is_pinned, is_favorite, is_collapsed, position,
		       created_at, updated_at
		FROM notebooks WHERE user_id = ?
		ORDER BY is_pinned DESC, position ASC, name ASC`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	flat := []*models.Notebook{}
	for rows.Next() {
		n := &models.Notebook{}
		var parentID sql.NullInt64
		if err := rows.Scan(&n.ID, &n.UserID, &parentID, &n.Name, &n.Icon, &n.Color,
			&n.IsPinned, &n.IsFavorite, &n.IsCollapsed, &n.Position,
			&n.CreatedAt, &n.UpdatedAt); err != nil {
			continue
		}
		if parentID.Valid {
			p := parentID.Int64
			n.ParentID = &p
		}
		flat = append(flat, n)
	}

	tree := buildTree(flat, nil)
	writeJSON(w, 200, tree)
}

func buildTree(items []*models.Notebook, parentID *int64) []*models.Notebook {
	result := []*models.Notebook{}
	for _, item := range items {
		match := false
		if parentID == nil && item.ParentID == nil {
			match = true
		} else if parentID != nil && item.ParentID != nil && *item.ParentID == *parentID {
			match = true
		}
		if match {
			item.Children = buildTree(items, &item.ID)
			result = append(result, item)
		}
	}
	return result
}

// ============ CREATE ============
func CreateNotebook(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	var req struct {
		Name     string `json:"name"`
		ParentID *int64 `json:"parent_id"`
		Icon     string `json:"icon"`
		Color    string `json:"color"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}
	if req.Name == "" {
		req.Name = "Folder Baru"
	}
	if req.Icon == "" {
		req.Icon = "folder"
	}
	if req.Color == "" {
		req.Color = "#6366f1"
	}

	res, err := config.DB.Exec(
		"INSERT INTO notebooks (user_id, parent_id, name, icon, color) VALUES (?, ?, ?, ?, ?)",
		userID, req.ParentID, req.Name, req.Icon, req.Color,
	)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": id, "name": req.Name})
}

// ============ UPDATE ============
func UpdateNotebook(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, err := pathInt(r, "id")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid id"})
		return
	}

	var req struct {
		Name       *string `json:"name"`
		Icon       *string `json:"icon"`
		Color      *string `json:"color"`
		ParentID   *int64  `json:"parent_id"`
		IsPinned   *bool   `json:"is_pinned"`
		IsFavorite *bool   `json:"is_favorite"`
		Position   *int    `json:"position"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE notebooks SET "
	args := []interface{}{}
	add := func(frag string, v interface{}) {
		query += frag
		args = append(args, v)
	}
	if req.Name != nil {
		add("name=?, ", *req.Name)
	}
	if req.Icon != nil {
		add("icon=?, ", *req.Icon)
	}
	if req.Color != nil {
		add("color=?, ", *req.Color)
	}
	if req.ParentID != nil {
		add("parent_id=?, ", *req.ParentID)
	}
	if req.IsPinned != nil {
		add("is_pinned=?, ", *req.IsPinned)
	}
	if req.IsFavorite != nil {
		add("is_favorite=?, ", *req.IsFavorite)
	}
	if req.Position != nil {
		add("position=?, ", *req.Position)
	}

	if len(args) == 0 {
		writeJSON(w, 400, map[string]string{"error": "Tidak ada field untuk diupdate"})
		return
	}
	query = query[:len(query)-2] + " WHERE id=? AND user_id=?"
	args = append(args, id, userID)

	if _, err := config.DB.Exec(query, args...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "OK"})
}

// ============ DELETE ============
func DeleteNotebook(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, err := pathInt(r, "id")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid id"})
		return
	}

	_, err = config.DB.Exec("DELETE FROM notebooks WHERE id=? AND user_id=?", id, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// ============ FASE 1: REORDER ============
func ReorderNotebooks(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Orders []struct {
			ID       int64  `json:"id"`
			Position int    `json:"position"`
			ParentID *int64 `json:"parent_id"`
		} `json:"orders"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	tx, err := config.DB.Begin()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	for _, o := range req.Orders {
		_, err := tx.Exec(
			"UPDATE notebooks SET position=?, parent_id=? WHERE id=? AND user_id=?",
			o.Position, o.ParentID, o.ID, userID)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
	}

	tx.Commit()
	writeJSON(w, 200, map[string]string{"message": "Reordered"})
}

// ============ FASE 1: COLLAPSE ============
func ToggleCollapseNotebook(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		IsCollapsed bool `json:"is_collapsed"`
	}
	_ = readJSON(r, &req)

	_, _ = config.DB.Exec(
		"UPDATE notebooks SET is_collapsed=? WHERE id=? AND user_id=?",
		req.IsCollapsed, id, userID)
	writeJSON(w, 200, map[string]string{"message": "OK"})
}
