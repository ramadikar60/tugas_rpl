package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"notebook-app/auth"
	"notebook-app/config"
	"notebook-app/middleware"
	"notebook-app/models"
)

// ============================================================
// LIST NOTES — Include notes yang dibagikan ke user
// ============================================================
func ListNotes(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	q := r.URL.Query()

	filter := q.Get("filter")
	notebookID := q.Get("notebook_id")
	tagID := q.Get("tag_id")
	onlyShared := q.Get("only_shared") == "true"

	// Query dasar — UNION antara owned + shared
	query := `
		SELECT DISTINCT
		    n.id, n.user_id AS owner_id, n.notebook_id, n.title, n.content,
		    n.is_pinned, n.is_favorite, n.is_archived, n.is_trashed, n.trashed_at,
		    n.is_locked, n.is_readonly, n.has_checklist, n.checklist_total, n.checklist_done,
		    n.word_count, n.char_count, n.sort_order,
		    n.created_at, n.updated_at,
		    CASE WHEN n.user_id = ? THEN 'owner' ELSE s.permission END AS access_permission,
		    COALESCE(u.username, '') AS owner_username,
		    COALESCE(s.id, 0) AS share_id
		FROM notes n
		LEFT JOIN users u ON u.id = n.user_id
		LEFT JOIN shares s ON s.item_type = 'note'
		    AND s.item_id = n.id
		    AND s.shared_with_user_id = ?
		    AND s.is_active = TRUE
		    AND (s.expires_at IS NULL OR s.expires_at > NOW())
		WHERE
		    (n.user_id = ? OR s.id IS NOT NULL)
	`
	args := []interface{}{userID, userID, userID}

	// Filter hanya shared
	if onlyShared {
		query += " AND n.user_id != ? AND s.id IS NOT NULL"
		args = append(args, userID)
	}

	// Filter by tag
	if tagID != "" {
		query += ` AND n.id IN (SELECT note_id FROM note_tags WHERE tag_id = ?)`
		args = append(args, tagID)
	}

	// Filter status
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
		query += " AND n.is_trashed = FALSE AND n.is_archived = FALSE"
	}

	// Filter notebook
	if notebookID != "" {
		query += " AND n.notebook_id = ?"
		args = append(args, notebookID)
	}

	// Order: owned dulu, lalu shared, lalu pinned & updated
	query += ` ORDER BY 
	    CASE WHEN n.user_id = ? THEN 0 ELSE 1 END,
	    n.is_pinned DESC, 
	    n.updated_at DESC`
	args = append(args, userID)

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	notes := []map[string]interface{}{}
	for rows.Next() {
		var id, ownerID int64
		var nbID, shareID sql.NullInt64
		var title, content, ownerUsername, accessPermission string
		var isPinned, isFavorite, isArchived, isTrashed bool
		var isLocked, isReadonly, hasChecklist bool
		var checklistTotal, checklistDone, wordCount, charCount, sortOrder int
		var trashedAt sql.NullTime
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &ownerID, &nbID, &title, &content,
			&isPinned, &isFavorite, &isArchived, &isTrashed, &trashedAt,
			&isLocked, &isReadonly, &hasChecklist, &checklistTotal, &checklistDone,
			&wordCount, &charCount, &sortOrder,
			&createdAt, &updatedAt,
			&accessPermission, &ownerUsername, &shareID)

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
		if shareID.Valid && shareID.Int64 > 0 {
			note["share_id"] = shareID.Int64
		}

		notes = append(notes, note)
	}

	writeJSON(w, 200, notes)
}

// ============================================================
// GET NOTE — Bisa diakses owner atau shared user
// ============================================================
func GetNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, err := pathInt(r, "id")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid id"})
		return
	}

	n := &models.Note{}
	var nbID sql.NullInt64
	var trashedAt, lockedAt, readOnlyAt sql.NullTime
	var ownerID int64
	var ownerUsername string

	err = config.DB.QueryRow(`
		SELECT n.id, n.user_id, n.notebook_id, n.title, n.content, n.is_pinned, n.is_favorite,
		       n.is_archived, n.is_trashed, n.trashed_at, n.is_locked, n.is_readonly,
		       n.locked_at, n.read_only_at, n.has_checklist, n.checklist_total, n.checklist_done,
		       n.word_count, n.char_count, n.sort_order, n.created_at, n.updated_at,
		       COALESCE(u.username, '') AS owner_username
		FROM notes n
		LEFT JOIN users u ON u.id = n.user_id
		WHERE n.id=?`, id).
		Scan(&n.ID, &ownerID, &nbID, &n.Title, &n.Content,
			&n.IsPinned, &n.IsFavorite, &n.IsArchived, &n.IsTrashed, &trashedAt,
			&n.IsLocked, &n.IsReadonly, &lockedAt, &readOnlyAt,
			&n.HasChecklist, &n.ChecklistTotal, &n.ChecklistDone,
			&n.WordCount, &n.CharCount, &n.SortOrder,
			&n.CreatedAt, &n.UpdatedAt, &ownerUsername)

	if err == sql.ErrNoRows {
		writeJSON(w, 404, map[string]string{"error": "Note tidak ditemukan"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	var accessPermission string
	var accessShareID int64

	if ownerID == userID {
		n.UserID = userID
		accessPermission = "owner"
	} else {
		err := config.DB.QueryRow(`
			SELECT id, permission FROM shares
			WHERE item_type = 'note' AND item_id = ?
			  AND shared_with_user_id = ?
			  AND is_active = TRUE
			  AND (expires_at IS NULL OR expires_at > NOW())
			LIMIT 1`, id, userID).Scan(&accessShareID, &accessPermission)

		if err == sql.ErrNoRows {
			writeJSON(w, 403, map[string]string{"error": "Anda tidak punya akses ke catatan ini"})
			return
		}
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		n.UserID = ownerID
	}

	n.NotebookID = nil
	if nbID.Valid {
		v := nbID.Int64
		n.NotebookID = &v
	}
	if trashedAt.Valid {
		t := trashedAt.Time
		n.TrashedAt = &t
	}
	if lockedAt.Valid {
		t := lockedAt.Time
		n.LockedAt = &t
	}
	if readOnlyAt.Valid {
		t := readOnlyAt.Time
		n.ReadOnlyAt = &t
	}

	loadTags(n)
	loadAttachments(n)

	if accessPermission == "owner" {
		_, _ = config.DB.Exec("UPDATE notes SET last_viewed_at=NOW() WHERE id=?", id)
	}

	response := map[string]interface{}{
		"id":              n.ID,
		"user_id":         n.UserID,
		"notebook_id":     n.NotebookID,
		"title":           n.Title,
		"content":         n.Content,
		"is_pinned":       n.IsPinned,
		"is_favorite":     n.IsFavorite,
		"is_archived":     n.IsArchived,
		"is_trashed":      n.IsTrashed,
		"is_locked":       n.IsLocked,
		"is_readonly":     n.IsReadonly,
		"has_checklist":   n.HasChecklist,
		"checklist_total": n.ChecklistTotal,
		"checklist_done":  n.ChecklistDone,
		"word_count":      n.WordCount,
		"char_count":      n.CharCount,
		"sort_order":      n.SortOrder,
		"tags":            n.Tags,
		"attachments":     n.Attachments,
		"created_at":      n.CreatedAt,
		"updated_at":      n.UpdatedAt,

		"access_permission": accessPermission,
		"owner_username":    ownerUsername,
		"share_id":          accessShareID,
	}

	if trashedAt.Valid {
		response["trashed_at"] = trashedAt.Time
	}
	if lockedAt.Valid {
		response["locked_at"] = lockedAt.Time
	}
	if readOnlyAt.Valid {
		response["read_only_at"] = readOnlyAt.Time
	}

	writeJSON(w, 200, response)
}

// ============================================================
// CREATE NOTE
// ============================================================
func CreateNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	var req struct {
		Title      string `json:"title"`
		Content    string `json:"content"`
		NotebookID *int64 `json:"notebook_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}
	if req.Title == "" {
		req.Title = "Tanpa judul"
	}
	req.Content = sanitizeNoteHTML(req.Content)

	text := stripHTML(req.Content)
	words := len(strings.Fields(text))
	chars := len([]rune(text))

	res, err := config.DB.Exec(
		`INSERT INTO notes (user_id, notebook_id, title, content, content_text, word_count, char_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, req.NotebookID, req.Title, req.Content, text, words, chars,
	)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	UpdateChecklistStats(id, req.Content)
	ProcessNoteLinks(id, userID, req.Content)
	ProcessMentions(id, userID, req.Content)
	SaveVersion(id, userID, req.Title, req.Content, false)

	writeJSON(w, 201, map[string]interface{}{"id": id, "title": req.Title})
}

// ============================================================
// UPDATE NOTE — Owner full edit, Shared editor bisa edit, Viewer ditolak
// ============================================================
func UpdateNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, err := pathInt(r, "id")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid id"})
		return
	}

	var ownerID int64
	var accessPermission string
	var shareID int64

	_ = config.DB.QueryRow("SELECT user_id FROM notes WHERE id=?", id).Scan(&ownerID)
	if ownerID == 0 {
		writeJSON(w, 404, map[string]string{"error": "Note tidak ditemukan"})
		return
	}

	if ownerID == userID {
		accessPermission = "owner"
	} else {
		err := config.DB.QueryRow(`
			SELECT id, permission FROM shares
			WHERE item_type = 'note' AND item_id = ?
			  AND shared_with_user_id = ?
			  AND is_active = TRUE
			  AND (expires_at IS NULL OR expires_at > NOW())
			LIMIT 1`, id, userID).Scan(&shareID, &accessPermission)
		if err != nil {
			writeJSON(w, 403, map[string]string{"error": "Anda tidak punya akses ke catatan ini"})
			return
		}
		if accessPermission != "editor" {
			writeJSON(w, 403, map[string]string{"error": "Anda hanya bisa melihat catatan ini (viewer)"})
			return
		}
	}

	var isLocked bool
	_ = config.DB.QueryRow("SELECT is_locked FROM notes WHERE id=?", id).Scan(&isLocked)
	if isLocked {
		writeJSON(w, 403, map[string]string{"error": "Catatan terkunci"})
		return
	}

	var req struct {
		Title      *string `json:"title"`
		Content    *string `json:"content"`
		NotebookID *int64  `json:"notebook_id"`
		IsPinned   *bool   `json:"is_pinned"`
		IsFavorite *bool   `json:"is_favorite"`
		IsArchived *bool   `json:"is_archived"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	if req.Content != nil {
		clean := sanitizeNoteHTML(*req.Content)
		req.Content = &clean
	}

	var oldTitle, oldContent string
	_ = config.DB.QueryRow("SELECT title, COALESCE(content,'') FROM notes WHERE id=?", id).
		Scan(&oldTitle, &oldContent)

	query := "UPDATE notes SET "
	args := []interface{}{}
	add := func(frag string, v interface{}) {
		query += frag
		args = append(args, v)
	}

	if req.Title != nil {
		add("title=?, ", *req.Title)
	}
	if req.Content != nil {
		add("content=?, ", *req.Content)
		text := stripHTML(*req.Content)
		add("content_text=?, ", text)
		add("word_count=?, ", len(strings.Fields(text)))
		add("char_count=?, ", len([]rune(text)))
	}
	if req.NotebookID != nil && accessPermission == "owner" {
		add("notebook_id=?, ", *req.NotebookID)
	}
	if req.IsPinned != nil && accessPermission == "owner" {
		add("is_pinned=?, ", *req.IsPinned)
	}
	if req.IsFavorite != nil && accessPermission == "owner" {
		add("is_favorite=?, ", *req.IsFavorite)
	}
	if req.IsArchived != nil && accessPermission == "owner" {
		add("is_archived=?, ", *req.IsArchived)
	}

	if len(args) == 0 {
		writeJSON(w, 400, map[string]string{"error": "Tidak ada field"})
		return
	}
	query = query[:len(query)-2] + " WHERE id=?"
	args = append(args, id)

	if _, err := config.DB.Exec(query, args...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	if req.Content != nil {
		UpdateChecklistStats(id, *req.Content)
		ProcessNoteLinks(id, ownerID, *req.Content)
		ProcessMentions(id, ownerID, *req.Content)

		title := oldTitle
		if req.Title != nil {
			title = *req.Title
		}
		SaveVersion(id, userID, title, *req.Content, true)
		CleanupOldVersions(id)

		if accessPermission == "editor" {
			oldLen := len(oldContent)
			newLen := len(*req.Content)
			titleChanged := req.Title != nil && *req.Title != oldTitle

			var desc string
			if titleChanged {
				desc = "Mengubah judul: \"" + oldTitle + "\" → \"" + *req.Title + "\""
			} else if oldLen != newLen {
				diff := newLen - oldLen
				if diff > 0 {
					desc = fmt.Sprintf("Menambah %d karakter pada isi catatan", diff)
				} else {
					desc = fmt.Sprintf("Menghapus %d karakter dari isi catatan", -diff)
				}
			} else {
				desc = "Mengedit isi catatan"
			}

			var editorName string
			_ = config.DB.QueryRow("SELECT username FROM users WHERE id=?", userID).Scan(&editorName)

			LogActivity(
				ownerID,
				"note_edited_by_shared",
				"note",
				id,
				"@"+editorName+" ("+accessPermission+") "+desc,
				clientIP(r),
				r.UserAgent(),
			)

			_, _ = config.DB.Exec(`
				INSERT INTO notifications (user_id, type, title, message, link)
				VALUES (?, 'note_edited', 'Catatan diedit oleh user lain', ?, ?)`,
				ownerID,
				"@"+editorName+" mengedit: \""+title+"\" — "+desc,
				"/notes?id="+intToStr(id))
		}
	}

	writeJSON(w, 200, map[string]string{"message": "Saved"})
}

// ============================================================
// TRASH / RESTORE / DELETE
// ============================================================
func TrashNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	if !ownsNote(userID, id) {
		writeJSON(w, 404, map[string]string{"error": "Catatan tidak ditemukan"})
		return
	}
	_, _ = config.DB.Exec(
		"UPDATE notes SET is_trashed=TRUE, trashed_at=NOW() WHERE id=? AND user_id=?",
		id, userID)
	writeJSON(w, 200, map[string]string{"message": "Moved to trash"})
}

func RestoreNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	if !ownsNote(userID, id) {
		writeJSON(w, 404, map[string]string{"error": "Catatan tidak ditemukan"})
		return
	}
	_, _ = config.DB.Exec(
		"UPDATE notes SET is_trashed=FALSE, trashed_at=NULL WHERE id=? AND user_id=?",
		id, userID)
	writeJSON(w, 200, map[string]string{"message": "Restored"})
}

func DeleteNotePermanently(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	if !ownsNote(userID, id) {
		writeJSON(w, 404, map[string]string{"error": "Catatan tidak ditemukan"})
		return
	}

	rows, err := config.DB.Query(
		"SELECT stored_name FROM attachments WHERE note_id=? AND user_id=?", id, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var storedName string
			if err := rows.Scan(&storedName); err == nil {
				_ = os.Remove(filepath.Join(uploadDir(), storedName))
			}
		}
	}

	_, _ = config.DB.Exec("DELETE FROM notes WHERE id=? AND user_id=?", id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted permanently"})
}

func DuplicateNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var title, content string
	var notebookID sql.NullInt64
	err := config.DB.QueryRow(
		"SELECT title, content, notebook_id FROM notes WHERE id=? AND user_id=?",
		id, userID,
	).Scan(&title, &content, &notebookID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Note tidak ditemukan"})
		return
	}

	var nbArg interface{}
	if notebookID.Valid {
		nbArg = notebookID.Int64
	}

	content = sanitizeNoteHTML(content)
	text := stripHTML(content)
	res, _ := config.DB.Exec(
		"INSERT INTO notes (user_id, notebook_id, title, content, content_text, word_count, char_count) VALUES (?, ?, ?, ?, ?, ?, ?)",
		userID, nbArg, title+" (Copy)", content, text,
		len(strings.Fields(text)), len([]rune(text)),
	)
	newID, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": newID})
}

// ============================================================
// EMPTY TRASH & CLEANUP
// ============================================================
func EmptyTrash(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	rows, err := config.DB.Query(`
		SELECT a.stored_name FROM attachments a
		JOIN notes n ON n.id = a.note_id
		WHERE n.user_id = ? AND n.is_trashed = TRUE`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var storedName string
			if err := rows.Scan(&storedName); err == nil {
				_ = os.Remove(filepath.Join(uploadDir(), storedName))
			}
		}
	}

	_, _ = config.DB.Exec(
		"DELETE FROM notes WHERE user_id = ? AND is_trashed = TRUE", userID)
	writeJSON(w, 200, map[string]string{"message": "Sampah dikosongkan"})
}

func CleanupTrash(days int) {
	rows, err := config.DB.Query(`
		SELECT a.stored_name FROM attachments a
		JOIN notes n ON n.id = a.note_id
		WHERE n.is_trashed = TRUE 
		  AND n.trashed_at IS NOT NULL
		  AND n.trashed_at < DATE_SUB(NOW(), INTERVAL ? DAY)`, days)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var storedName string
			if err := rows.Scan(&storedName); err == nil {
				_ = os.Remove(filepath.Join(uploadDir(), storedName))
			}
		}
	}

	res, err := config.DB.Exec(`
		DELETE FROM notes 
		WHERE is_trashed = TRUE 
		  AND trashed_at IS NOT NULL
		  AND trashed_at < DATE_SUB(NOW(), INTERVAL ? DAY)`, days)
	if err != nil {
		log.Printf("CleanupTrash error: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("Auto-hapus %d note dari sampah (lebih dari %d hari)", n, days)
	}
}

// ============================================================
// LOCK / READONLY (Owner only)
// ============================================================
func LockNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	if !ownsNote(userID, id) {
		writeJSON(w, 404, map[string]string{"error": "Catatan tidak ditemukan"})
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	_ = readJSON(r, &req)

	hash := ""
	if req.Password != "" {
		h, err := auth.HashPassword(req.Password)
		if err == nil {
			hash = h
		}
	}

	_, err := config.DB.Exec(`
		UPDATE notes SET is_locked = TRUE, locked_at = NOW(), locked_hash = ?
		WHERE id = ? AND user_id = ?`, hash, id, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Note terkunci"})
}

func UnlockNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Password string `json:"password"`
	}
	_ = readJSON(r, &req)

	var storedHash sql.NullString
	err := config.DB.QueryRow(
		"SELECT locked_hash FROM notes WHERE id=? AND user_id=?",
		id, userID).Scan(&storedHash)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Note tidak ditemukan"})
		return
	}

	if storedHash.Valid && storedHash.String != "" {
		if !auth.VerifyPassword(storedHash.String, req.Password) {
			writeJSON(w, 403, map[string]string{"error": "Password salah"})
			return
		}
	}

	_, _ = config.DB.Exec(`
		UPDATE notes SET is_locked = FALSE, locked_at = NULL, locked_hash = NULL
		WHERE id = ? AND user_id = ?`, id, userID)
	writeJSON(w, 200, map[string]string{"message": "Note dibuka"})
}

func ToggleReadOnly(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		IsReadonly bool `json:"is_readonly"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	_, err := config.DB.Exec(`
		UPDATE notes SET is_readonly = ?, read_only_at = IF(?, NOW(), NULL)
		WHERE id = ? AND user_id = ?`, req.IsReadonly, req.IsReadonly, id, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "OK"})
}

func RecalcWordCount(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var content string
	err := config.DB.QueryRow(
		"SELECT content FROM notes WHERE id=? AND user_id=?", id, userID).Scan(&content)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Not found"})
		return
	}

	text := stripHTML(content)
	words := len(strings.Fields(text))
	chars := len([]rune(text))

	_, _ = config.DB.Exec(
		"UPDATE notes SET word_count=?, char_count=? WHERE id=?",
		words, chars, id)

	writeJSON(w, 200, map[string]interface{}{
		"word_count": words,
		"char_count": chars,
	})
}

// ============================================================
// HELPERS
// ============================================================
func loadTags(n *models.Note) {
	rows, err := config.DB.Query(`
		SELECT t.id, t.name, t.color FROM tags t
		JOIN note_tags nt ON nt.tag_id = t.id
		WHERE nt.note_id = ?`, n.ID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		t := &models.Tag{}
		_ = rows.Scan(&t.ID, &t.Name, &t.Color)
		n.Tags = append(n.Tags, t)
	}
}

func loadAttachments(n *models.Note) {
	rows, err := config.DB.Query(
		"SELECT id, note_id, filename, stored_name, mime_type, size, created_at FROM attachments WHERE note_id=?",
		n.ID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		a := &models.Attachment{}
		var created time.Time
		_ = rows.Scan(&a.ID, &a.NoteID, &a.Filename, &a.StoredName, &a.MimeType, &a.Size, &created)
		a.CreatedAt = created
		a.URL = "/uploads/" + a.StoredName
		n.Attachments = append(n.Attachments, a)
	}
}

func UpdateChecklistStats(noteID int64, content string) {
	total := strings.Count(content, "data-list=\"checked\"") + strings.Count(content, "data-list=\"unchecked\"")
	done := strings.Count(content, "data-list=\"checked\"")
	hasChecklist := total > 0

	_, _ = config.DB.Exec(`
		UPDATE notes SET has_checklist=?, checklist_total=?, checklist_done=?
		WHERE id=?`, hasChecklist, total, done, noteID)
}
