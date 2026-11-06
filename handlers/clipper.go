package handlers

import (
	"database/sql"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ============================================================
// WEB CLIPPER — Section 10
// ============================================================

// ClipWebPage — save web page / article / selection
func ClipWebPage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		URL        string `json:"url"`
		Title      string `json:"title"`
		Author     string `json:"author"`
		SiteName   string `json:"site_name"`
		Excerpt    string `json:"excerpt"`
		Content    string `json:"content"`
		CoverImage string `json:"cover_image"`
		ClipType   string `json:"clip_type"`
		CreateNote bool   `json:"create_note"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	if strings.TrimSpace(req.URL) == "" {
		writeJSON(w, 400, map[string]string{"error": "URL wajib diisi"})
		return
	}

	// Validasi URL: hanya http/https (mencegah javascript:/data: tersimpan sebagai tautan).
	if _, err := url.ParseRequestURI(req.URL); err != nil || safeHTTPURL(req.URL) == "" {
		writeJSON(w, 400, map[string]string{"error": "URL tidak valid"})
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.CoverImage != "" {
		req.CoverImage = safeHTTPURL(req.CoverImage)
	}
	req.Content = sanitizeNoteHTML(req.Content)

	// Auto-fill dari URL
	if req.Title == "" {
		req.Title = extractTitleFromURL(req.URL)
	}
	if req.SiteName == "" {
		if u, err := url.Parse(req.URL); err == nil {
			req.SiteName = u.Hostname()
		}
	}
	if req.ClipType == "" {
		req.ClipType = "article"
	}

	// Hitung word count & read time
	text := stripHTML(req.Content)
	words := len(strings.Fields(text))
	readTime := (words + 199) / 200 // ~200 wpm
	if readTime < 1 {
		readTime = 1
	}

	var noteID interface{}
	if req.CreateNote {
		// Buat note baru dari clip
		content := "<h1>" + escHTML(req.Title) + "</h1>"
		if req.URL != "" {
			content += `<p><a href="` + escHTML(req.URL) + `" target="_blank">` + escHTML(req.URL) + `</a></p>`
		}
		if req.Content != "" {
			content += req.Content
		}
		content = sanitizeNoteHTML(content)

		res, err := config.DB.Exec(`
			INSERT INTO notes (user_id, title, content, content_text, word_count, char_count)
			VALUES (?, ?, ?, ?, ?, ?)`,
			userID, req.Title, content, text, words, len([]rune(text)))
		if err == nil {
			id, _ := res.LastInsertId()
			noteID = id
			SaveVersion(id, userID, req.Title, content, false)
		}
	}

	// Simpan clip
	res, err := config.DB.Exec(`
		INSERT INTO clips 
		    (user_id, note_id, url, title, author, site_name, excerpt, content, 
		     cover_image, clip_type, word_count, read_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, noteID, req.URL, req.Title, req.Author, req.SiteName,
		req.Excerpt, req.Content, req.CoverImage, req.ClipType,
		words, readTime)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	writeJSON(w, 201, map[string]interface{}{
		"id":         id,
		"note_id":    noteID,
		"title":      req.Title,
		"word_count": words,
		"read_time":  readTime,
	})
}

// ListClips — daftar clip user
func ListClips(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	clipType := q.Get("type")
	unread := q.Get("unread") == "true"
	archived := q.Get("archived") == "true"

	query := `SELECT id, url, title, COALESCE(author,''), COALESCE(site_name,''),
	          COALESCE(excerpt,''), COALESCE(cover_image,''), clip_type,
	          word_count, read_time, is_read, is_archived, note_id,
	          created_at FROM clips WHERE user_id = ?`
	args := []interface{}{userID}

	if archived {
		query += " AND is_archived = TRUE"
	} else {
		query += " AND is_archived = FALSE"
	}

	if unread {
		query += " AND is_read = FALSE"
	}

	if clipType != "" && clipType != "all" {
		query += " AND clip_type = ?"
		args = append(args, clipType)
	}

	query += " ORDER BY created_at DESC LIMIT 200"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var url, title, author, siteName, excerpt, coverImage, clipType string
		var wordCount, readTime int
		var isRead, isArchived bool
		var noteID sql.NullInt64
		var createdAt time.Time

		_ = rows.Scan(&id, &url, &title, &author, &siteName, &excerpt,
			&coverImage, &clipType, &wordCount, &readTime,
			&isRead, &isArchived, &noteID, &createdAt)

		item := map[string]interface{}{
			"id":          id,
			"url":         url,
			"title":       title,
			"author":      author,
			"site_name":   siteName,
			"excerpt":     excerpt,
			"cover_image": coverImage,
			"clip_type":   clipType,
			"word_count":  wordCount,
			"read_time":   readTime,
			"is_read":     isRead,
			"is_archived": isArchived,
			"created_at":  createdAt,
		}
		if noteID.Valid {
			item["note_id"] = noteID.Int64
		}
		result = append(result, item)
	}

	writeJSON(w, 200, result)
}

// GetClip — detail 1 clip
func GetClip(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var url, title, author, siteName, excerpt, content, coverImage, clipType string
	var wordCount, readTime int
	var isRead, isArchived bool
	var noteID sql.NullInt64
	var createdAt, updatedAt time.Time

	err := config.DB.QueryRow(`
		SELECT url, title, COALESCE(author,''), COALESCE(site_name,''),
		       COALESCE(excerpt,''), COALESCE(content,''), COALESCE(cover_image,''),
		       clip_type, word_count, read_time, is_read, is_archived, note_id,
		       created_at, updated_at
		FROM clips WHERE id = ? AND user_id = ?`,
		id, userID).
		Scan(&url, &title, &author, &siteName, &excerpt, &content, &coverImage,
			&clipType, &wordCount, &readTime, &isRead, &isArchived, &noteID,
			&createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		writeJSON(w, 404, map[string]string{"error": "Clip tidak ditemukan"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	result := map[string]interface{}{
		"id":          id,
		"url":         url,
		"title":       title,
		"author":      author,
		"site_name":   siteName,
		"excerpt":     excerpt,
		"content":     content,
		"cover_image": coverImage,
		"clip_type":   clipType,
		"word_count":  wordCount,
		"read_time":   readTime,
		"is_read":     isRead,
		"is_archived": isArchived,
		"created_at":  createdAt,
		"updated_at":  updatedAt,
	}
	if noteID.Valid {
		result["note_id"] = noteID.Int64
	}
	writeJSON(w, 200, result)
}

// UpdateClip
func UpdateClip(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Title      *string `json:"title"`
		IsRead     *bool   `json:"is_read"`
		IsArchived *bool   `json:"is_archived"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE clips SET "
	args := []interface{}{}
	if req.Title != nil {
		query += "title=?, "
		args = append(args, *req.Title)
	}
	if req.IsRead != nil {
		query += "is_read=?, "
		args = append(args, *req.IsRead)
	}
	if req.IsArchived != nil {
		query += "is_archived=?, "
		args = append(args, *req.IsArchived)
	}

	if len(args) == 0 {
		writeJSON(w, 400, map[string]string{"error": "Tidak ada field"})
		return
	}
	query = query[:len(query)-2] + " WHERE id=? AND user_id=?"
	args = append(args, id, userID)

	_, _ = config.DB.Exec(query, args...)
	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

// DeleteClip
func DeleteClip(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	_, _ = config.DB.Exec("DELETE FROM clips WHERE id=? AND user_id=?", id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// ============================================================
// READING LIST
// ============================================================

// ListReadingList
func ListReadingList(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	status := q.Get("status")

	query := `SELECT id, COALESCE(clip_id, 0), url, COALESCE(title,''),
	          COALESCE(description,''), COALESCE(cover_image,''),
	          priority, status, progress, created_at, completed_at
	          FROM reading_list WHERE user_id = ?`
	args := []interface{}{userID}

	if status != "" && status != "all" {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY FIELD(priority, 'high', 'medium', 'low'), created_at DESC"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, clipID int64
		var url, title, description, coverImage, priority, status string
		var progress int
		var createdAt time.Time
		var completedAt sql.NullTime

		_ = rows.Scan(&id, &clipID, &url, &title, &description, &coverImage,
			&priority, &status, &progress, &createdAt, &completedAt)

		item := map[string]interface{}{
			"id":          id,
			"url":         url,
			"title":       title,
			"description": description,
			"cover_image": coverImage,
			"priority":    priority,
			"status":      status,
			"progress":    progress,
			"created_at":  createdAt,
		}
		if clipID > 0 {
			item["clip_id"] = clipID
		}
		if completedAt.Valid {
			item["completed_at"] = completedAt.Time
		}
		result = append(result, item)
	}

	writeJSON(w, 200, result)
}

// AddToReadingList
func AddToReadingList(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		URL         string `json:"url"`
		Title       string `json:"title"`
		Description string `json:"description"`
		CoverImage  string `json:"cover_image"`
		Priority    string `json:"priority"`
		ClipID      *int64 `json:"clip_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	if req.URL == "" && req.ClipID == nil {
		writeJSON(w, 400, map[string]string{"error": "URL atau clip_id wajib diisi"})
		return
	}
	if req.Priority == "" {
		req.Priority = "medium"
	}

	res, err := config.DB.Exec(`
		INSERT INTO reading_list (user_id, clip_id, url, title, description, cover_image, priority)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, req.ClipID, req.URL, req.Title, req.Description, req.CoverImage, req.Priority)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": id, "title": req.Title})
}

// UpdateReadingList
func UpdateReadingList(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Status   *string `json:"status"`
		Progress *int    `json:"progress"`
		Priority *string `json:"priority"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE reading_list SET "
	args := []interface{}{}

	if req.Status != nil {
		query += "status=?, "
		args = append(args, *req.Status)

		if *req.Status == "finished" {
			query += "completed_at=NOW(), progress=100, "
		}
	}
	if req.Progress != nil {
		query += "progress=?, "
		args = append(args, *req.Progress)
	}
	if req.Priority != nil {
		query += "priority=?, "
		args = append(args, *req.Priority)
	}

	query = strings.TrimSuffix(query, ", ")
	query += " WHERE id=? AND user_id=?"
	args = append(args, id, userID)

	_, _ = config.DB.Exec(query, args...)
	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

// DeleteReadingList
func DeleteReadingList(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	_, _ = config.DB.Exec("DELETE FROM reading_list WHERE id=? AND user_id=?", id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// ============================================================
// HELPER
// ============================================================

func extractTitleFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// Ambil dari path terakhir
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		last = strings.TrimSuffix(last, ".html")
		last = strings.TrimSuffix(last, ".htm")
		last = strings.ReplaceAll(last, "-", " ")
		last = strings.ReplaceAll(last, "_", " ")
		if last != "" {
			return strings.Title(last)
		}
	}

	return u.Hostname()
}

var _ = regexp.MustCompile
