package handlers

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ============================================================
// DATA MANAGEMENT — Section 16
// ============================================================

// BulkAction — bulk operation pada notes
//
// POST /api/bulk
// Body: { action, note_ids: [1,2,3], params: {...} }
func BulkAction(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Action  string                 `json:"action"` // trash | restore | delete | move | tag | archive | unarchive | favorite | unfavorite
		NoteIDs []int64                `json:"note_ids"`
		Params  map[string]interface{} `json:"params"`
	}
	if err := readJSON(r, &req); err != nil || len(req.NoteIDs) == 0 {
		writeJSON(w, 400, map[string]string{"error": "note_ids wajib diisi"})
		return
	}

	// Build placeholder untuk IN clause
	placeholders := make([]string, len(req.NoteIDs))
	args := []interface{}{userID}
	for i, id := range req.NoteIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	inClause := strings.Join(placeholders, ",")

	var query string
	var queryArgs []interface{}

	switch req.Action {
	case "trash":
		query = fmt.Sprintf(`
			UPDATE notes SET is_trashed=TRUE, trashed_at=NOW()
			WHERE user_id=? AND id IN (%s)`, inClause)
		queryArgs = args

	case "restore":
		query = fmt.Sprintf(`
			UPDATE notes SET is_trashed=FALSE, trashed_at=NULL
			WHERE user_id=? AND id IN (%s)`, inClause)
		queryArgs = args

	case "archive":
		query = fmt.Sprintf(`
			UPDATE notes SET is_archived=TRUE
			WHERE user_id=? AND id IN (%s)`, inClause)
		queryArgs = args

	case "unarchive":
		query = fmt.Sprintf(`
			UPDATE notes SET is_archived=FALSE
			WHERE user_id=? AND id IN (%s)`, inClause)
		queryArgs = args

	case "favorite":
		query = fmt.Sprintf(`
			UPDATE notes SET is_favorite=TRUE
			WHERE user_id=? AND id IN (%s)`, inClause)
		queryArgs = args

	case "unfavorite":
		query = fmt.Sprintf(`
			UPDATE notes SET is_favorite=FALSE
			WHERE user_id=? AND id IN (%s)`, inClause)
		queryArgs = args

	case "delete":
		// Hard delete
		query = fmt.Sprintf(`
			DELETE FROM notes WHERE user_id=? AND id IN (%s)`, inClause)
		queryArgs = args

	case "move":
		notebookID, ok := req.Params["notebook_id"]
		if !ok {
			writeJSON(w, 400, map[string]string{"error": "notebook_id wajib diisi"})
			return
		}
		var nbArg interface{} = nil
		if nbID, ok := notebookID.(float64); ok && nbID > 0 {
			nbArg = int64(nbID)
		}
		query = fmt.Sprintf(`
			UPDATE notes SET notebook_id=?
			WHERE user_id=? AND id IN (%s)`, inClause)
		queryArgs = append([]interface{}{nbArg}, args...)

	case "tag":
		tagID, ok := req.Params["tag_id"]
		if !ok {
			writeJSON(w, 400, map[string]string{"error": "tag_id wajib diisi"})
			return
		}
		tagIDInt := int64(tagID.(float64))

		// Insert tags untuk semua note
		for _, noteID := range req.NoteIDs {
			_, _ = config.DB.Exec(`
				INSERT IGNORE INTO note_tags (note_id, tag_id)
				SELECT ?, ? FROM notes
				WHERE id = ? AND user_id = ?`,
				noteID, tagIDInt, noteID, userID)
		}
		writeJSON(w, 200, map[string]interface{}{
			"message": "Tag ditambahkan",
			"count":   len(req.NoteIDs),
		})
		return

	default:
		writeJSON(w, 400, map[string]string{"error": "Action tidak dikenal"})
		return
	}

	res, err := config.DB.Exec(query, queryArgs...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	affected, _ := res.RowsAffected()
	writeJSON(w, 200, map[string]interface{}{
		"message":  "Berhasil",
		"affected": affected,
	})
}

// ============================================================
// EXPORT — Export notes ke berbagai format
// ============================================================

// ExportNotes — export ke JSON / Markdown / CSV
//
// Query params:
//
//	format = json | md | csv
//	filter = all | favorites | ...
func ExportNotes(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	format := q.Get("format")
	if format == "" {
		format = "json"
	}
	filter := q.Get("filter")
	if filter == "" {
		filter = "all"
	}

	// Query notes
	query := `SELECT id, title, content, content_text, is_pinned, is_favorite, is_archived,
	          created_at, updated_at FROM notes WHERE user_id = ?`
	args := []interface{}{userID}

	switch filter {
	case "favorites":
		query += " AND is_favorite = TRUE"
	case "archived":
		query += " AND is_archived = TRUE"
	case "pinned":
		query += " AND is_pinned = TRUE"
	case "trash":
		query += " AND is_trashed = TRUE"
	default:
		query += " AND is_trashed = FALSE"
	}

	query += " ORDER BY updated_at DESC"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type ExportNote struct {
		ID          int64     `json:"id"`
		Title       string    `json:"title"`
		Content     string    `json:"content"`
		ContentText string    `json:"content_text"`
		IsPinned    bool      `json:"is_pinned"`
		IsFavorite  bool      `json:"is_favorite"`
		IsArchived  bool      `json:"is_archived"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
	}

	notes := []ExportNote{}
	for rows.Next() {
		var n ExportNote
		_ = rows.Scan(&n.ID, &n.Title, &n.Content, &n.ContentText,
			&n.IsPinned, &n.IsFavorite, &n.IsArchived, &n.CreatedAt, &n.UpdatedAt)
		notes = append(notes, n)
	}

	timestamp := time.Now().Format("20060102-150405")

	switch format {
	case "md":
		w.Header().Set("Content-Type", "text/markdown")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="notes-%s.md"`, timestamp))

		fmt.Fprintf(w, "# Notes Export\n\n")
		fmt.Fprintf(w, "Exported: %s\n\n---\n\n", time.Now().Format("2006-01-02 15:04:05"))

		for _, n := range notes {
			fmt.Fprintf(w, "## %s\n\n", n.Title)
			fmt.Fprintf(w, "*Created: %s | Updated: %s*\n\n",
				n.CreatedAt.Format("2006-01-02"), n.UpdatedAt.Format("2006-01-02"))
			fmt.Fprintf(w, "%s\n\n---\n\n", n.ContentText)
		}

	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="notes-%s.csv"`, timestamp))

		writer := csv.NewWriter(w)
		defer writer.Flush()

		writer.Write([]string{"ID", "Title", "Content", "Created", "Updated"})
		for _, n := range notes {
			writer.Write([]string{
				fmt.Sprintf("%d", n.ID),
				n.Title,
				n.ContentText,
				n.CreatedAt.Format("2006-01-02 15:04:05"),
				n.UpdatedAt.Format("2006-01-02 15:04:05"),
			})
		}

	default: // json
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="notes-%s.json"`, timestamp))

		json.NewEncoder(w).Encode(map[string]interface{}{
			"exported_at": time.Now(),
			"count":       len(notes),
			"notes":       notes,
		})
	}
}

// ============================================================
// IMPORT — Import notes dari JSON
// ============================================================

// ImportNotes — import notes dari JSON
func ImportNotes(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Notes []struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		} `json:"notes"`
	}
	if err := readJSONMax(r, &req, importJSONLimit); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid JSON"})
		return
	}

	imported := 0
	failed := 0

	for _, n := range req.Notes {
		if n.Title == "" {
			n.Title = "Imported Note"
		}

		n.Content = sanitizeNoteHTML(n.Content)
		text := stripHTML(n.Content)
		words := len(strings.Fields(text))
		chars := len([]rune(text))

		res, err := config.DB.Exec(`
			INSERT INTO notes (user_id, title, content, content_text, word_count, char_count)
			VALUES (?, ?, ?, ?, ?, ?)`,
			userID, n.Title, n.Content, text, words, chars)

		if err != nil {
			failed++
			continue
		}

		id, _ := res.LastInsertId()
		SaveVersion(id, userID, n.Title, n.Content, false)
		imported++
	}

	writeJSON(w, 200, map[string]interface{}{
		"imported": imported,
		"failed":   failed,
		"total":    len(req.Notes),
	})
}

// ============================================================
// DUPLICATE DETECTION
// ============================================================

// FindDuplicates — cari catatan dengan judul mirip
func FindDuplicates(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	// Cari notes dengan title yang sama (case-insensitive)
	rows, err := config.DB.Query(`
		SELECT LOWER(title) AS lower_title, COUNT(*) AS cnt
		FROM notes WHERE user_id = ? AND is_trashed = FALSE
		GROUP BY LOWER(title)
		HAVING cnt > 1
		ORDER BY cnt DESC
		LIMIT 50`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var lowerTitle string
		var count int
		_ = rows.Scan(&lowerTitle, &count)

		// Ambil detail notes dengan title ini
		noteRows, _ := config.DB.Query(`
			SELECT id, title, created_at, updated_at, word_count
			FROM notes WHERE user_id = ? AND LOWER(title) = ? AND is_trashed = FALSE
			ORDER BY updated_at DESC`,
			userID, lowerTitle)
		notes := []map[string]interface{}{}
		if noteRows != nil {
			for noteRows.Next() {
				var id int64
				var title string
				var createdAt, updatedAt time.Time
				var wordCount int
				_ = noteRows.Scan(&id, &title, &createdAt, &updatedAt, &wordCount)
				notes = append(notes, map[string]interface{}{
					"id": id, "title": title,
					"created_at": createdAt, "updated_at": updatedAt,
					"word_count": wordCount,
				})
			}
			noteRows.Close()
		}

		result = append(result, map[string]interface{}{
			"title": lowerTitle,
			"count": count,
			"notes": notes,
		})
	}

	writeJSON(w, 200, result)
}

// ============================================================
// STORAGE MANAGEMENT
// ============================================================

// StorageBreakdown — detail penggunaan storage
func StorageBreakdown(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	result := map[string]interface{}{}

	// Total size
	var totalSize sql.NullInt64
	_ = config.DB.QueryRow(`
		SELECT COALESCE(SUM(size), 0) FROM attachments WHERE user_id = ?`,
		userID).Scan(&totalSize)

	// By mime type
	rows, _ := config.DB.Query(`
		SELECT 
		    CASE 
		        WHEN mime_type LIKE 'image/%' THEN 'image'
		        WHEN mime_type LIKE 'video/%' THEN 'video'
		        WHEN mime_type LIKE 'audio/%' THEN 'audio'
		        WHEN mime_type LIKE '%pdf%' THEN 'pdf'
		        WHEN mime_type LIKE '%word%' OR mime_type LIKE '%document%' THEN 'document'
		        WHEN mime_type LIKE '%excel%' OR mime_type LIKE '%spreadsheet%' THEN 'spreadsheet'
		        ELSE 'other'
		    END AS type,
		    COUNT(*) AS count,
		    COALESCE(SUM(size), 0) AS total_size
		FROM attachments WHERE user_id = ?
		GROUP BY type
		ORDER BY total_size DESC`, userID)

	byType := []map[string]interface{}{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var typ string
			var count int
			var size int64
			_ = rows.Scan(&typ, &count, &size)
			byType = append(byType, map[string]interface{}{
				"type": typ, "count": count, "size": size,
			})
		}
	}

	// Quota
	var quota int64
	_ = config.DB.QueryRow("SELECT storage_quota FROM users WHERE id=?", userID).Scan(&quota)

	usedSize := int64(0)
	if totalSize.Valid {
		usedSize = totalSize.Int64
	}

	result["total_size"] = usedSize
	result["quota"] = quota
	result["by_type"] = byType

	if quota > 0 {
		result["percent"] = (usedSize * 100) / quota
	} else {
		result["percent"] = 0
	}

	writeJSON(w, 200, result)
}

// LargestAttachments — file terbesar
func LargestAttachments(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	rows, err := config.DB.Query(`
		SELECT id, filename, stored_name, mime_type, size, created_at
		FROM attachments WHERE user_id = ?
		ORDER BY size DESC LIMIT 20`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var filename, storedName, mimeType string
		var size int64
		var createdAt time.Time
		_ = rows.Scan(&id, &filename, &storedName, &mimeType, &size, &createdAt)
		result = append(result, map[string]interface{}{
			"id": id, "filename": filename, "mime_type": mimeType,
			"size": size, "url": "/uploads/" + storedName,
			"created_at": createdAt,
		})
	}

	writeJSON(w, 200, result)
}
