package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ============================================================
// RESEARCH & READING — Section 11
// ============================================================

// ListSources
func ListSources(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	sourceType := q.Get("type")
	tag := q.Get("tag")

	query := `SELECT id, note_id, title, COALESCE(url,''), COALESCE(author,''),
	          COALESCE(publication,''), COALESCE(year,''), COALESCE(publisher,''),
	          COALESCE(doi,''), COALESCE(isbn,''), source_type, citation_format,
	          COALESCE(tags,''), COALESCE(notes,''), created_at
	          FROM research_sources WHERE user_id = ?`
	args := []interface{}{userID}

	if sourceType != "" && sourceType != "all" {
		query += " AND source_type = ?"
		args = append(args, sourceType)
	}
	if tag != "" {
		query += " AND tags LIKE ?"
		args = append(args, "%"+tag+"%")
	}

	query += " ORDER BY created_at DESC LIMIT 500"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var noteID sql.NullInt64
		var title, url, author, publication, year, publisher, doi, isbn string
		var sourceType, citationFormat, tags, notes string
		var createdAt time.Time

		_ = rows.Scan(&id, &noteID, &title, &url, &author, &publication, &year,
			&publisher, &doi, &isbn, &sourceType, &citationFormat, &tags, &notes, &createdAt)

		item := map[string]interface{}{
			"id":              id,
			"title":           title,
			"url":             url,
			"author":          author,
			"publication":     publication,
			"year":            year,
			"publisher":       publisher,
			"doi":             doi,
			"isbn":            isbn,
			"source_type":     sourceType,
			"citation_format": citationFormat,
			"tags":            tags,
			"notes":           notes,
			"created_at":      createdAt,
		}
		if noteID.Valid {
			item["note_id"] = noteID.Int64
		}
		result = append(result, item)
	}

	writeJSON(w, 200, result)
}

// CreateSource
func CreateSource(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Title          string `json:"title"`
		URL            string `json:"url"`
		Author         string `json:"author"`
		Publication    string `json:"publication"`
		Year           string `json:"year"`
		Publisher      string `json:"publisher"`
		DOI            string `json:"doi"`
		ISBN           string `json:"isbn"`
		SourceType     string `json:"source_type"`
		CitationFormat string `json:"citation_format"`
		Tags           string `json:"tags"`
		Notes          string `json:"notes"`
		NoteID         *int64 `json:"note_id"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Title) == "" {
		writeJSON(w, 400, map[string]string{"error": "Judul wajib diisi"})
		return
	}

	if req.SourceType == "" {
		req.SourceType = "website"
	}
	if req.CitationFormat == "" {
		req.CitationFormat = "apa"
	}

	res, err := config.DB.Exec(`
		INSERT INTO research_sources 
		    (user_id, note_id, title, url, author, publication, year, publisher,
		     doi, isbn, source_type, citation_format, tags, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, req.NoteID, req.Title, req.URL, req.Author, req.Publication,
		req.Year, req.Publisher, req.DOI, req.ISBN, req.SourceType,
		req.CitationFormat, req.Tags, req.Notes)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": id, "title": req.Title})
}

// GetSource — detail + citation formatted
func GetSource(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var noteID sql.NullInt64
	var title, url, author, publication, year, publisher, doi, isbn string
	var sourceType, citationFormat, tags, notes string
	var createdAt time.Time

	err := config.DB.QueryRow(`
		SELECT note_id, title, COALESCE(url,''), COALESCE(author,''),
		       COALESCE(publication,''), COALESCE(year,''), COALESCE(publisher,''),
		       COALESCE(doi,''), COALESCE(isbn,''), source_type, citation_format,
		       COALESCE(tags,''), COALESCE(notes,''), created_at
		FROM research_sources WHERE id = ? AND user_id = ?`,
		id, userID).
		Scan(&noteID, &title, &url, &author, &publication, &year, &publisher,
			&doi, &isbn, &sourceType, &citationFormat, &tags, &notes, &createdAt)

	if err == sql.ErrNoRows {
		writeJSON(w, 404, map[string]string{"error": "Sumber tidak ditemukan"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	citation := generateCitation(citationFormat, author, year, title, publisher, url, publication, doi)

	result := map[string]interface{}{
		"id":              id,
		"title":           title,
		"url":             url,
		"author":          author,
		"publication":     publication,
		"year":            year,
		"publisher":       publisher,
		"doi":             doi,
		"isbn":            isbn,
		"source_type":     sourceType,
		"citation_format": citationFormat,
		"tags":            tags,
		"notes":           notes,
		"citation":        citation,
		"created_at":      createdAt,
	}
	if noteID.Valid {
		result["note_id"] = noteID.Int64
	}

	// Ambil highlights
	hlRows, _ := config.DB.Query(`
		SELECT id, text, COALESCE(note,''), color, created_at
		FROM highlights WHERE source_id = ? AND user_id = ?
		ORDER BY created_at DESC`, id, userID)
	highlights := []map[string]interface{}{}
	if hlRows != nil {
		defer hlRows.Close()
		for hlRows.Next() {
			var hid int64
			var text, note, color string
			var t time.Time
			_ = hlRows.Scan(&hid, &text, &note, &color, &t)
			highlights = append(highlights, map[string]interface{}{
				"id":         hid,
				"text":       text,
				"note":       note,
				"color":      color,
				"created_at": t,
			})
		}
	}
	result["highlights"] = highlights

	writeJSON(w, 200, result)
}

// UpdateSource
func UpdateSource(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Title          *string `json:"title"`
		URL            *string `json:"url"`
		Author         *string `json:"author"`
		Publication    *string `json:"publication"`
		Year           *string `json:"year"`
		Publisher      *string `json:"publisher"`
		DOI            *string `json:"doi"`
		ISBN           *string `json:"isbn"`
		SourceType     *string `json:"source_type"`
		CitationFormat *string `json:"citation_format"`
		Tags           *string `json:"tags"`
		Notes          *string `json:"notes"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE research_sources SET "
	args := []interface{}{}

	add := func(f string, v interface{}) {
		query += f + "=?, "
		args = append(args, v)
	}

	if req.Title != nil {
		add("title", *req.Title)
	}
	if req.URL != nil {
		add("url", *req.URL)
	}
	if req.Author != nil {
		add("author", *req.Author)
	}
	if req.Publication != nil {
		add("publication", *req.Publication)
	}
	if req.Year != nil {
		add("year", *req.Year)
	}
	if req.Publisher != nil {
		add("publisher", *req.Publisher)
	}
	if req.DOI != nil {
		add("doi", *req.DOI)
	}
	if req.ISBN != nil {
		add("isbn", *req.ISBN)
	}
	if req.SourceType != nil {
		add("source_type", *req.SourceType)
	}
	if req.CitationFormat != nil {
		add("citation_format", *req.CitationFormat)
	}
	if req.Tags != nil {
		add("tags", *req.Tags)
	}
	if req.Notes != nil {
		add("notes", *req.Notes)
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

// DeleteSource
func DeleteSource(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	_, _ = config.DB.Exec("DELETE FROM research_sources WHERE id=? AND user_id=?", id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// ============================================================
// HIGHLIGHTS
// ============================================================

// ListHighlights — semua highlight user
func ListHighlights(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	rows, err := config.DB.Query(`
		SELECT h.id, COALESCE(h.clip_id, 0), COALESCE(h.source_id, 0),
		       COALESCE(h.note_id, 0), h.text, COALESCE(h.note,''), h.color,
		       COALESCE(c.title, s.title, n.title, '') AS source_title,
		       h.created_at
		FROM highlights h
		LEFT JOIN clips c ON c.id = h.clip_id
		LEFT JOIN research_sources s ON s.id = h.source_id
		LEFT JOIN notes n ON n.id = h.note_id
		WHERE h.user_id = ?
		ORDER BY h.created_at DESC LIMIT 500`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, clipID, sourceID, noteID int64
		var text, note, color, sourceTitle string
		var createdAt time.Time

		_ = rows.Scan(&id, &clipID, &sourceID, &noteID, &text, &note, &color,
			&sourceTitle, &createdAt)

		item := map[string]interface{}{
			"id":           id,
			"text":         text,
			"note":         note,
			"color":        color,
			"source_title": sourceTitle,
			"created_at":   createdAt,
		}
		if clipID > 0 {
			item["clip_id"] = clipID
		}
		if sourceID > 0 {
			item["source_id"] = sourceID
		}
		if noteID > 0 {
			item["note_id"] = noteID
		}
		result = append(result, item)
	}

	writeJSON(w, 200, result)
}

// CreateHighlight
func CreateHighlight(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Text          string `json:"text"`
		Note          string `json:"note"`
		Color         string `json:"color"`
		ClipID        *int64 `json:"clip_id"`
		SourceID      *int64 `json:"source_id"`
		NoteID        *int64 `json:"note_id"`
		PositionStart int    `json:"position_start"`
		PositionEnd   int    `json:"position_end"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Text) == "" {
		writeJSON(w, 400, map[string]string{"error": "Teks highlight wajib diisi"})
		return
	}

	if req.Color == "" {
		req.Color = "#fef08a"
	}

	res, err := config.DB.Exec(`
		INSERT INTO highlights 
		    (user_id, clip_id, source_id, note_id, text, note, color, position_start, position_end)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, req.ClipID, req.SourceID, req.NoteID, req.Text, req.Note,
		req.Color, req.PositionStart, req.PositionEnd)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": id})
}

// UpdateHighlight
func UpdateHighlight(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Note  *string `json:"note"`
		Color *string `json:"color"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE highlights SET "
	args := []interface{}{}
	if req.Note != nil {
		query += "note=?, "
		args = append(args, *req.Note)
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

	_, _ = config.DB.Exec(query, args...)
	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

// DeleteHighlight
func DeleteHighlight(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	_, _ = config.DB.Exec("DELETE FROM highlights WHERE id=? AND user_id=?", id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// ============================================================
// CITATION GENERATOR
// ============================================================

func generateCitation(format, author, year, title, publisher, url, publication, doi string) string {
	if author == "" {
		author = "Anonim"
	}
	if year == "" {
		year = "n.d."
	}

	switch format {
	case "mla":
		// MLA: Author. "Title." Publisher, Year. URL.
		s := author + ". \"" + title + ".\" "
		if publisher != "" {
			s += publisher + ", "
		}
		s += year + ". "
		if url != "" {
			s += url + "."
		}
		return s

	case "chicago":
		// Chicago: Author. "Title." Publisher (Year). URL.
		s := author + ". \"" + title + ".\" "
		if publisher != "" {
			s += publisher + " "
		}
		s += "(" + year + "). "
		if url != "" {
			s += url + "."
		}
		return s

	case "ieee":
		// IEEE: [1] Author, "Title," Publisher, Year. [Online]. Available: URL
		s := author + ", \"" + title + ",\" "
		if publisher != "" {
			s += publisher + ", "
		}
		s += year + ". "
		if url != "" {
			s += "[Online]. Available: " + url
		}
		return s

	case "harvard":
		// Harvard: Author (Year) Title. Available at: URL
		s := author + " (" + year + ") "
		s += title + ". "
		if url != "" {
			s += "Available at: " + url
		}
		return s

	default: // apa
		// APA: Author. (Year). Title. Publisher. URL
		s := author + ". (" + year + "). "
		s += title + ". "
		if publisher != "" {
			s += publisher + ". "
		}
		if doi != "" {
			s += "https://doi.org/" + doi
		} else if url != "" {
			s += url
		}
		return strings.TrimSpace(s)
	}
}

// GetRelatedSources — sumber dengan tag sama
func GetRelatedSources(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var tags string
	err := config.DB.QueryRow(`
		SELECT COALESCE(tags,'') FROM research_sources WHERE id = ? AND user_id = ?`,
		id, userID).Scan(&tags)
	if err != nil || tags == "" {
		writeJSON(w, 200, []interface{}{})
		return
	}

	tagList := strings.Split(tags, ",")
	if len(tagList) == 0 {
		writeJSON(w, 200, []interface{}{})
		return
	}

	// Query: same tag
	query := `SELECT DISTINCT id, title, COALESCE(author,''), COALESCE(year,'')
	          FROM research_sources WHERE user_id = ? AND id != ? AND (`
	args := []interface{}{userID, id}

	for i, t := range tagList {
		if i > 0 {
			query += " OR "
		}
		query += "tags LIKE ?"
		args = append(args, "%"+strings.TrimSpace(t)+"%")
	}
	query += ") LIMIT 10"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 200, []interface{}{})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var rid int64
		var title, author, year string
		_ = rows.Scan(&rid, &title, &author, &year)
		result = append(result, map[string]interface{}{
			"id":     rid,
			"title":  title,
			"author": author,
			"year":   year,
		})
	}

	writeJSON(w, 200, result)
}

var _ = fmt.Sprintf
