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
// QUICK CAPTURE — Section 2
// ============================================================

// QuickNote — buat catatan cepat dengan judul dari content
func QuickNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Title      string  `json:"title"`
		Content    string  `json:"content"`
		NotebookID *int64  `json:"notebook_id"`
		Tags       []int64 `json:"tags"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	req.Content = sanitizeNoteHTML(req.Content)

	// Auto-generate title dari content kalau kosong
	title := strings.TrimSpace(req.Title)
	if title == "" {
		text := stripHTML(req.Content)
		words := strings.Fields(text)
		if len(words) > 0 {
			maxWords := 8
			if len(words) < maxWords {
				maxWords = len(words)
			}
			title = strings.Join(words[:maxWords], " ")
			if len(words) > maxWords {
				title += "..."
			}
		} else {
			title = "Quick Note — " + time.Now().Format("2 Jan 15:04")
		}
	}

	text := stripHTML(req.Content)
	words := len(strings.Fields(text))
	chars := len([]rune(text))

	res, err := config.DB.Exec(
		`INSERT INTO notes (user_id, notebook_id, title, content, content_text, word_count, char_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, req.NotebookID, title, req.Content, text, words, chars,
	)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	// Attach tags
	for _, tagID := range req.Tags {
		_, _ = config.DB.Exec(
			"INSERT IGNORE INTO note_tags (note_id, tag_id) VALUES (?, ?)",
			id, tagID)
	}

	UpdateChecklistStats(id, req.Content)
	ProcessNoteLinks(id, userID, req.Content)
	ProcessMentions(id, userID, req.Content)
	SaveVersion(id, userID, title, req.Content, false)

	writeJSON(w, 201, map[string]interface{}{
		"id":    id,
		"title": title,
	})
}

// QuickTask — buat task cepat
func QuickTask(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Title      string  `json:"title"`
		Priority   string  `json:"priority"`
		DueDate    *string `json:"due_date"`
		NotebookID *int64  `json:"notebook_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeJSON(w, 400, map[string]string{"error": "Judul wajib diisi"})
		return
	}

	if req.Priority == "" {
		req.Priority = "medium"
	}

	var dueDateArg interface{}
	if req.DueDate != nil && *req.DueDate != "" {
		if t, err := parseDateTimeInput(*req.DueDate); err == nil {
			dueDateArg = t
		}
	}

	res, err := config.DB.Exec(`
		INSERT INTO tasks (user_id, notebook_id, title, priority, due_date)
		VALUES (?, ?, ?, ?, ?)`,
		userID, req.NotebookID, title, req.Priority, dueDateArg)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	logTaskAction(id, userID, "created", "", title)

	writeJSON(w, 201, map[string]interface{}{
		"id":    id,
		"title": title,
	})
}

// QuickReminder — buat reminder cepat (task dengan reminder saja)
func QuickReminder(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Title      string `json:"title"`
		ReminderAt string `json:"reminder_at"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeJSON(w, 400, map[string]string{"error": "Judul reminder wajib diisi"})
		return
	}
	if req.ReminderAt == "" {
		writeJSON(w, 400, map[string]string{"error": "Waktu reminder wajib diisi"})
		return
	}

	reminderTime, err := parseDateTimeInput(req.ReminderAt)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Format waktu tidak valid"})
		return
	}

	res, err := config.DB.Exec(`
		INSERT INTO tasks (user_id, title, priority, reminder_at, due_date)
		VALUES (?, ?, 'medium', ?, ?)`,
		userID, title, reminderTime, reminderTime)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	logTaskAction(id, userID, "created", "", "Reminder: "+title)

	writeJSON(w, 201, map[string]interface{}{
		"id":    id,
		"title": title,
	})
}

// ClipboardCapture — capture dari clipboard (teks atau URL)
func ClipboardCapture(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Content string `json:"content"`
		AsType  string `json:"as_type"` // note | task | bookmark
		Source  string `json:"source"`  // url kalau ada
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeJSON(w, 400, map[string]string{"error": "Clipboard kosong"})
		return
	}

	// Deteksi apakah content adalah URL
	isURL := strings.HasPrefix(content, "http://") || strings.HasPrefix(content, "https://")

	if req.AsType == "" {
		if isURL {
			req.AsType = "bookmark"
		} else {
			req.AsType = "note"
		}
	}

	switch req.AsType {
	case "task":
		res, err := config.DB.Exec(`
			INSERT INTO tasks (user_id, title, priority)
			VALUES (?, ?, 'medium')`, userID, content)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		id, _ := res.LastInsertId()
		writeJSON(w, 201, map[string]interface{}{
			"id":    id,
			"type":  "task",
			"title": content,
		})

	case "bookmark":
		// Simpan URL sebagai note
		title := content
		if len(content) > 80 {
			title = content[:80] + "..."
		}
		// URL divalidasi dan di-escape; bukan URL http(s) disimpan sebagai teks biasa.
		var noteContent string
		if safe := safeHTTPURL(content); safe != "" {
			noteContent = `<p><a href="` + escHTML(safe) + `" target="_blank">` + escHTML(safe) + `</a></p>`
		} else {
			noteContent = "<p>" + escHTML(content) + "</p>"
		}

		res, err := config.DB.Exec(`
			INSERT INTO notes (user_id, title, content, content_text)
			VALUES (?, ?, ?, ?)`,
			userID, title, noteContent, content)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		id, _ := res.LastInsertId()
		writeJSON(w, 201, map[string]interface{}{
			"id":    id,
			"type":  "bookmark",
			"title": title,
		})

	default: // note
		content = sanitizeNoteHTML(content)
		text := stripHTML(content)
		words := len(strings.Fields(text))
		chars := len([]rune(text))

		title := text
		if len(title) > 60 {
			title = title[:60] + "..."
		}
		if title == "" {
			title = "Clipboard Capture"
		}

		res, err := config.DB.Exec(`
			INSERT INTO notes (user_id, title, content, content_text, word_count, char_count)
			VALUES (?, ?, ?, ?, ?, ?)`,
			userID, title, content, text, words, chars)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		id, _ := res.LastInsertId()
		writeJSON(w, 201, map[string]interface{}{
			"id":    id,
			"type":  "note",
			"title": title,
		})
	}
}

// ============================================================
// DRAFTS — Section 2 (Save Draft)
// ============================================================

// SaveDraft — simpan draft (belum jadi note/task)
func SaveDraft(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Type    string `json:"type"` // note | task | reminder
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	req.Content = sanitizeNoteHTML(req.Content)

	// Upsert: hanya 1 draft per user per type
	var existingID int64
	err := config.DB.QueryRow(`
		SELECT id FROM drafts WHERE user_id = ? AND type = ?`,
		userID, req.Type).Scan(&existingID)

	if err == sql.ErrNoRows {
		_, err = config.DB.Exec(`
			INSERT INTO drafts (user_id, type, title, content)
			VALUES (?, ?, ?, ?)`,
			userID, req.Type, req.Title, req.Content)
	} else {
		_, err = config.DB.Exec(`
			UPDATE drafts SET title = ?, content = ?, updated_at = NOW()
			WHERE id = ?`,
			req.Title, req.Content, existingID)
	}

	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]string{"message": "Draft tersimpan"})
}

// GetDraft — ambil draft
func GetDraft(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	draftType := r.URL.Query().Get("type")
	if draftType == "" {
		draftType = "note"
	}

	var id int64
	var title, content string
	var updatedAt time.Time

	err := config.DB.QueryRow(`
		SELECT id, COALESCE(title,''), COALESCE(content,''), updated_at
		FROM drafts WHERE user_id = ? AND type = ?`,
		userID, draftType).Scan(&id, &title, &content, &updatedAt)

	if err == sql.ErrNoRows {
		writeJSON(w, 200, map[string]interface{}{
			"type":    draftType,
			"title":   "",
			"content": "",
			"empty":   true,
		})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]interface{}{
		"id":         id,
		"type":       draftType,
		"title":      title,
		"content":    content,
		"updated_at": updatedAt,
	})
}

// DeleteDraft — hapus draft (setelah dikonversi jadi note/task)
func DeleteDraft(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	draftType := r.URL.Query().Get("type")

	_, _ = config.DB.Exec(
		"DELETE FROM drafts WHERE user_id = ? AND type = ?",
		userID, draftType)

	writeJSON(w, 200, map[string]string{"message": "Draft dihapus"})
}

// ============================================================
// RECENT — untuk Command Palette
// ============================================================

// RecentItems — ambil item terbaru untuk quick access
func RecentItems(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	result := map[string]interface{}{}

	// Recent notes
	noteRows, err := config.DB.Query(`
		SELECT id, title, updated_at FROM notes
		WHERE user_id = ? AND is_trashed = FALSE
		ORDER BY updated_at DESC LIMIT 10`, userID)
	if err == nil {
		defer noteRows.Close()
		notes := []map[string]interface{}{}
		for noteRows.Next() {
			var id int64
			var title string
			var updatedAt time.Time
			_ = noteRows.Scan(&id, &title, &updatedAt)
			notes = append(notes, map[string]interface{}{
				"id":         id,
				"title":      title,
				"updated_at": updatedAt,
			})
		}
		result["notes"] = notes
	}

	// Recent tasks
	taskRows, err := config.DB.Query(`
		SELECT id, title, status, created_at FROM tasks
		WHERE user_id = ? AND status NOT IN ('done', 'cancelled')
		ORDER BY created_at DESC LIMIT 10`, userID)
	if err == nil {
		defer taskRows.Close()
		tasks := []map[string]interface{}{}
		for taskRows.Next() {
			var id int64
			var title, status string
			var createdAt time.Time
			_ = taskRows.Scan(&id, &title, &status, &createdAt)
			tasks = append(tasks, map[string]interface{}{
				"id":         id,
				"title":      title,
				"status":     status,
				"created_at": createdAt,
			})
		}
		result["tasks"] = tasks
	}

	writeJSON(w, 200, result)
}
