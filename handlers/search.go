package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ============================================================
// SEARCH ADVANCED — Include shared notes
// ============================================================
func SearchNotes(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	keyword := strings.TrimSpace(q.Get("q"))
	tagID := q.Get("tag_id")
	notebookID := q.Get("notebook_id")
	dateFrom := q.Get("date_from")
	dateTo := q.Get("date_to")
	filter := q.Get("filter")
	hasChecklist := q.Get("has_checklist")
	hasAttachment := q.Get("has_attachment")
	sortBy := q.Get("sort")

	limit := 100
	offset := 0
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v >= 0 {
		offset = v
	}

	query := `
		SELECT DISTINCT
		    n.id, n.user_id AS owner_id, n.notebook_id, n.title, n.content,
		    n.is_pinned, n.is_favorite, n.is_archived, n.is_trashed, n.trashed_at,
		    n.is_locked, n.is_readonly, n.has_checklist, n.checklist_total, n.checklist_done,
		    n.word_count, n.char_count, n.sort_order,
		    n.created_at, n.updated_at,
		    CASE WHEN n.user_id = ? THEN 'owner' ELSE s.permission END AS access_permission,
		    COALESCE(u.username, '') AS owner_username
		FROM notes n
		LEFT JOIN users u ON u.id = n.user_id
		LEFT JOIN shares s ON s.item_type = 'note'
		    AND s.item_id = n.id
		    AND s.shared_with_user_id = ?
		    AND s.is_active = TRUE
		    AND (s.expires_at IS NULL OR s.expires_at > NOW())
		WHERE (n.user_id = ? OR s.id IS NOT NULL)
	`
	args := []interface{}{userID, userID, userID}

	useFulltext := false
	if keyword != "" && len(keyword) > 3 {
		useFulltext = true
	}

	switch filter {
	case "trash":
		query += " AND n.is_trashed = TRUE AND n.user_id = ?"
		args = append(args, userID)
	case "archived":
		query += ` AND n.is_archived = TRUE AND n.is_trashed = FALSE
		           AND (n.user_id = ? OR (n.user_id != ? AND s.permission = 'editor'))`
		args = append(args, userID, userID)
	case "favorites":
		query += " AND n.is_favorite = TRUE AND n.is_trashed = FALSE AND n.user_id = ?"
		args = append(args, userID)
	case "pinned":
		query += " AND n.is_pinned = TRUE AND n.is_trashed = FALSE AND n.user_id = ?"
		args = append(args, userID)
	default:
		query += " AND n.is_trashed = FALSE"
	}

	if tagID != "" {
		query += " AND n.id IN (SELECT note_id FROM note_tags WHERE tag_id = ?)"
		args = append(args, tagID)
	}
	if notebookID != "" {
		query += " AND n.notebook_id = ?"
		args = append(args, notebookID)
	}
	if keyword != "" {
		if useFulltext {
			query += ` AND (MATCH(n.title, n.content_text) AGAINST (? IN NATURAL LANGUAGE MODE)
			  OR n.id IN (SELECT note_id FROM attachments WHERE user_id = ? AND filename LIKE ?))`
			args = append(args, keyword, userID, "%"+keyword+"%")
		} else {
			kw := "%" + keyword + "%"
			query += ` AND (n.title LIKE ? OR n.content_text LIKE ?
			  OR n.id IN (SELECT note_id FROM attachments WHERE user_id = ? AND filename LIKE ?))`
			args = append(args, kw, kw, userID, kw)
		}
	}
	if dateFrom != "" {
		if _, err := time.Parse("2006-01-02", dateFrom); err == nil {
			query += " AND n.created_at >= ?"
			args = append(args, dateFrom+" 00:00:00")
		}
	}
	if dateTo != "" {
		if _, err := time.Parse("2006-01-02", dateTo); err == nil {
			query += " AND n.created_at <= ?"
			args = append(args, dateTo+" 23:59:59")
		}
	}
	if hasChecklist == "true" {
		query += " AND n.has_checklist = TRUE"
	} else if hasChecklist == "false" {
		query += " AND n.has_checklist = FALSE"
	}
	if hasAttachment == "true" {
		query += " AND n.id IN (SELECT note_id FROM attachments WHERE user_id = ?)"
		args = append(args, userID)
	} else if hasAttachment == "false" {
		query += " AND n.id NOT IN (SELECT note_id FROM attachments WHERE user_id = ?)"
		args = append(args, userID)
	}

	switch sortBy {
	case "created":
		query += " ORDER BY n.created_at DESC"
	case "title":
		query += " ORDER BY n.title ASC"
	default:
		query += ` ORDER BY 
		    CASE WHEN n.user_id = ? THEN 0 ELSE 1 END,
		    n.is_pinned DESC, 
		    n.updated_at DESC`
		args = append(args, userID)
	}

	query += " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	results := []map[string]interface{}{}
	for rows.Next() {
		var id, ownerID int64
		var nbID sql.NullInt64
		var title, content, ownerUsername, accessPermission string
		var isPinned, isFavorite, isArchived, isTrashed, isLocked, isReadonly, hasChecklist bool
		var checklistTotal, checklistDone, wordCount, charCount, sortOrder int
		var trashedAt sql.NullTime
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &ownerID, &nbID, &title, &content,
			&isPinned, &isFavorite, &isArchived, &isTrashed, &trashedAt,
			&isLocked, &isReadonly, &hasChecklist, &checklistTotal, &checklistDone,
			&wordCount, &charCount, &sortOrder,
			&createdAt, &updatedAt,
			&accessPermission, &ownerUsername)

		if err != nil {
			continue
		}

		note := map[string]interface{}{
			"id":                id,
			"user_id":           ownerID,
			"title":             title,
			"content":           content,
			"is_pinned":         isPinned,
			"is_favorite":       isFavorite,
			"is_archived":       isArchived,
			"is_trashed":        isTrashed,
			"is_locked":         isLocked,
			"is_readonly":       isReadonly,
			"has_checklist":     hasChecklist,
			"checklist_total":   checklistTotal,
			"checklist_done":    checklistDone,
			"word_count":        wordCount,
			"char_count":        charCount,
			"sort_order":        sortOrder,
			"created_at":        createdAt,
			"updated_at":        updatedAt,
			"access_permission": accessPermission,
			"owner_username":    ownerUsername,
		}

		if nbID.Valid {
			note["notebook_id"] = nbID.Int64
		}
		if trashedAt.Valid {
			note["trashed_at"] = trashedAt.Time
		}

		results = append(results, note)
	}

	writeJSON(w, 200, results)
}

// ============================================================
// SEARCH HISTORY
// ============================================================
func SaveSearchHistory(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Keyword string `json:"keyword"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Keyword) == "" {
		writeJSON(w, 400, map[string]string{"error": "Invalid keyword"})
		return
	}

	_, _ = config.DB.Exec(
		"DELETE FROM search_history WHERE user_id=? AND keyword=?",
		userID, req.Keyword)

	_, _ = config.DB.Exec(
		"INSERT INTO search_history (user_id, keyword) VALUES (?, ?)",
		userID, req.Keyword)

	_, _ = config.DB.Exec(`
		DELETE FROM search_history 
		WHERE user_id = ? AND id NOT IN (
			SELECT id FROM (
				SELECT id FROM search_history 
				WHERE user_id = ? 
				ORDER BY created_at DESC 
				LIMIT 20
			) AS t
		)`, userID, userID)

	writeJSON(w, 200, map[string]string{"message": "Saved"})
}

func ListSearchHistory(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	rows, err := config.DB.Query(`
		SELECT keyword, created_at FROM search_history
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT 20`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	results := []map[string]interface{}{}
	for rows.Next() {
		var kw string
		var t time.Time
		_ = rows.Scan(&kw, &t)
		results = append(results, map[string]interface{}{
			"keyword": kw,
			"time":    t,
		})
	}
	writeJSON(w, 200, results)
}

func ClearSearchHistory(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	_, _ = config.DB.Exec("DELETE FROM search_history WHERE user_id=?", userID)
	writeJSON(w, 200, map[string]string{"message": "Cleared"})
}

// ============================================================
// SAVED SEARCHES (PRESETS)
// ============================================================
func SaveSearchPreset(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Name    string                 `json:"name"`
		Filters map[string]interface{} `json:"filters"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "Nama preset wajib diisi"})
		return
	}

	filtersJSON, _ := json.Marshal(req.Filters)

	res, err := config.DB.Exec(
		"INSERT INTO saved_searches (user_id, name, filters) VALUES (?, ?, ?)",
		userID, req.Name, string(filtersJSON))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": id, "name": req.Name})
}

func ListSearchPresets(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	rows, err := config.DB.Query(`
		SELECT id, name, filters, created_at FROM saved_searches
		WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name, filtersJSON string
		var t time.Time
		_ = rows.Scan(&id, &name, &filtersJSON, &t)

		var filters map[string]interface{}
		_ = json.Unmarshal([]byte(filtersJSON), &filters)

		result = append(result, map[string]interface{}{
			"id":      id,
			"name":    name,
			"filters": filters,
			"time":    t,
		})
	}
	writeJSON(w, 200, result)
}

func DeleteSearchPreset(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	_, _ = config.DB.Exec("DELETE FROM saved_searches WHERE id=? AND user_id=?", id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}
