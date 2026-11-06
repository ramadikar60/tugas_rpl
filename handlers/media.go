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
// MEDIA LIBRARY — Section 12
// ============================================================

// detectMediaType — dari mime type
func detectMediaType(mime string) string {
	if strings.HasPrefix(mime, "image/") {
		return "image"
	}
	if strings.HasPrefix(mime, "video/") {
		return "video"
	}
	if strings.HasPrefix(mime, "audio/") {
		return "audio"
	}
	if strings.Contains(mime, "pdf") {
		return "pdf"
	}
	if strings.Contains(mime, "word") || strings.Contains(mime, "excel") ||
		strings.Contains(mime, "spreadsheet") || strings.Contains(mime, "document") ||
		strings.Contains(mime, "presentation") || strings.Contains(mime, "powerpoint") {
		return "document"
	}
	return "other"
}

// ListMedia — daftar media library
// Sekarang menyertakan info catatan asal (note_id, note_title)
func ListMedia(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	mediaType := q.Get("type")
	keyword := strings.TrimSpace(q.Get("q"))
	favorite := q.Get("favorite") == "true"
	tag := q.Get("tag")

	query := `
		SELECT ma.id, ma.attachment_id, ma.media_type, COALESCE(ma.title,''),
		       COALESCE(ma.description,''), COALESCE(ma.tags,''), 
		       ma.width, ma.height, ma.duration, ma.is_favorite,
		       a.filename, a.stored_name, a.mime_type, a.size, a.created_at,
		       a.note_id,
		       COALESCE(n.title, '') AS note_title,
		       n.is_trashed AS note_trashed,
		       COALESCE(nb.name, '') AS notebook_name
		FROM media_attachments ma
		JOIN attachments a ON a.id = ma.attachment_id
		LEFT JOIN notes n ON n.id = a.note_id
		LEFT JOIN notebooks nb ON nb.id = n.notebook_id
		WHERE ma.user_id = ?`
	args := []interface{}{userID}

	if mediaType != "" && mediaType != "all" {
		query += " AND ma.media_type = ?"
		args = append(args, mediaType)
	}

	if favorite {
		query += " AND ma.is_favorite = TRUE"
	}

	if keyword != "" {
		kw := "%" + keyword + "%"
		query += " AND (ma.title LIKE ? OR ma.description LIKE ? OR a.filename LIKE ? OR n.title LIKE ?)"
		args = append(args, kw, kw, kw, kw)
	}

	if tag != "" {
		query += " AND ma.tags LIKE ?"
		args = append(args, "%"+tag+"%")
	}

	query += " ORDER BY ma.created_at DESC LIMIT 500"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, attachmentID, noteID int64
		var mediaType, title, description, tags, filename, storedName, mimeType string
		var noteTitle, notebookName string
		var width, height, duration int
		var isFavorite bool
		var noteTrashed sql.NullBool
		var size int64
		var createdAt time.Time

		if err := rows.Scan(&id, &attachmentID, &mediaType, &title, &description, &tags,
			&width, &height, &duration, &isFavorite, &filename, &storedName,
			&mimeType, &size, &createdAt, &noteID, &noteTitle, &noteTrashed, &notebookName); err != nil {
			continue
		}

		item := map[string]interface{}{
			"id":            id,
			"attachment_id": attachmentID,
			"media_type":    mediaType,
			"title":         title,
			"description":   description,
			"tags":          tags,
			"width":         width,
			"height":        height,
			"duration":      duration,
			"is_favorite":   isFavorite,
			"filename":      filename,
			"stored_name":   storedName,
			"mime_type":     mimeType,
			"size":          size,
			"url":           "/uploads/" + storedName,
			"created_at":    createdAt,
			"note_id":       noteID,
			"note_title":    noteTitle,
			"notebook_name": notebookName,
		}

		if noteTrashed.Valid && noteTrashed.Bool {
			item["note_trashed"] = true
		}

		result = append(result, item)
	}

	writeJSON(w, 200, result)
}

// RegisterMedia — register attachment sebagai media item (manual)
func RegisterMedia(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		AttachmentID int64  `json:"attachment_id"`
		MediaType    string `json:"media_type"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		Tags         string `json:"tags"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		Duration     int    `json:"duration"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	var mimeType, filename string
	err := config.DB.QueryRow(`
		SELECT mime_type, filename FROM attachments 
		WHERE id = ? AND user_id = ?`,
		req.AttachmentID, userID).Scan(&mimeType, &filename)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Attachment tidak ditemukan"})
		return
	}

	if req.MediaType == "" {
		req.MediaType = detectMediaType(mimeType)
	}
	if req.Title == "" {
		req.Title = filename
	}

	res, err := config.DB.Exec(`
		INSERT INTO media_attachments 
		    (attachment_id, user_id, media_type, title, description, tags,
		     width, height, duration)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.AttachmentID, userID, req.MediaType, req.Title, req.Description,
		req.Tags, req.Width, req.Height, req.Duration)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": id, "title": req.Title})
}

// UpdateMedia
func UpdateMedia(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Tags        *string `json:"tags"`
		IsFavorite  *bool   `json:"is_favorite"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE media_attachments SET "
	args := []interface{}{}

	if req.Title != nil {
		query += "title=?, "
		args = append(args, *req.Title)
	}
	if req.Description != nil {
		query += "description=?, "
		args = append(args, *req.Description)
	}
	if req.Tags != nil {
		query += "tags=?, "
		args = append(args, *req.Tags)
	}
	if req.IsFavorite != nil {
		query += "is_favorite=?, "
		args = append(args, *req.IsFavorite)
	}

	if len(args) == 0 {
		writeJSON(w, 400, map[string]string{"error": "Tidak ada field"})
		return
	}
	query = query[:len(query)-2] + " WHERE id=? AND user_id=?"
	args = append(args, id, userID)

	if _, err := config.DB.Exec(query, args...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

// DeleteMedia — hanya hapus record, file tetap
func DeleteMedia(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	if _, err := config.DB.Exec(
		"DELETE FROM media_attachments WHERE id=? AND user_id=?", id, userID); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// MediaStats
func MediaStats(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	stats := map[string]int{
		"image": 0, "video": 0, "audio": 0, "pdf": 0, "document": 0, "other": 0,
		"total": 0, "favorite": 0,
	}

	rows, err := config.DB.Query(`
		SELECT media_type, COUNT(*) FROM media_attachments
		WHERE user_id = ? GROUP BY media_type`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var mt string
			var c int
			if err := rows.Scan(&mt, &c); err == nil {
				stats[mt] = c
				stats["total"] += c
			}
		}
	}

	var favoriteCount int
	err = config.DB.QueryRow(`
		SELECT COUNT(*) FROM media_attachments
		WHERE user_id = ? AND is_favorite = TRUE`, userID).Scan(&favoriteCount)
	if err == nil {
		stats["favorite"] = favoriteCount
	}

	var totalSize sql.NullInt64
	_ = config.DB.QueryRow(`
		SELECT COALESCE(SUM(size), 0) FROM attachments WHERE user_id = ?`,
		userID).Scan(&totalSize)

	totalSizeVal := int64(0)
	if totalSize.Valid {
		totalSizeVal = totalSize.Int64
	}

	response := map[string]interface{}{
		"image":      stats["image"],
		"video":      stats["video"],
		"audio":      stats["audio"],
		"pdf":        stats["pdf"],
		"document":   stats["document"],
		"other":      stats["other"],
		"total":      stats["total"],
		"favorite":   stats["favorite"],
		"total_size": totalSizeVal,
	}

	writeJSON(w, 200, response)
}

// ListMediaTags
func ListMediaTags(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	rows, err := config.DB.Query(`
		SELECT tags FROM media_attachments
		WHERE user_id = ? AND tags IS NOT NULL AND tags != ''`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	tagSet := map[string]bool{}
	for rows.Next() {
		var tags string
		if err := rows.Scan(&tags); err != nil {
			continue
		}
		for _, t := range strings.Split(tags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tagSet[t] = true
			}
		}
	}

	result := []string{}
	for t := range tagSet {
		result = append(result, t)
	}

	writeJSON(w, 200, result)
}

// ============================================================
// BACKFILL MEDIA
// ============================================================
func BackfillMedia(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	rows, err := config.DB.Query(`
		SELECT a.id, a.user_id, a.filename, a.mime_type
		FROM attachments a
		LEFT JOIN media_attachments ma ON ma.attachment_id = a.id
		WHERE a.user_id = ? AND ma.id IS NULL`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type pending struct {
		ID       int64
		UserID   int64
		Filename string
		MimeType string
	}
	list := []pending{}

	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.ID, &p.UserID, &p.Filename, &p.MimeType); err == nil {
			list = append(list, p)
		}
	}

	inserted := 0
	for _, p := range list {
		mediaType := detectMediaType(p.MimeType)
		_, err := config.DB.Exec(`
			INSERT INTO media_attachments 
			    (attachment_id, user_id, media_type, title)
			VALUES (?, ?, ?, ?)`,
			p.ID, p.UserID, mediaType, p.Filename)
		if err == nil {
			inserted++
		}
	}

	writeJSON(w, 200, map[string]interface{}{
		"found":    len(list),
		"inserted": inserted,
		"message":  "Backfill selesai",
	})
}
