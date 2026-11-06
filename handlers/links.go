package handlers

import (
	"database/sql"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

// Regex untuk [[wiki links]] dan @username
var (
	wikiLinkRe = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	mentionRe  = regexp.MustCompile(`@([a-zA-Z0-9_]+)`)
)

// ============================================================
// PROCESS LINKS & MENTIONS (dipanggil dari note.go)
// ============================================================

// ProcessNoteLinks — parse [[wiki links]] di content, buat record note_links.
func ProcessNoteLinks(noteID, userID int64, content string) {
	// Hapus link lama dari note ini
	_, _ = config.DB.Exec("DELETE FROM note_links WHERE source_note_id=?", noteID)

	matches := wikiLinkRe.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		targetTitle := strings.TrimSpace(m[1])
		if targetTitle == "" {
			continue
		}

		// Cari note dengan title yang cocok
		var targetID int64
		err := config.DB.QueryRow(
			"SELECT id FROM notes WHERE user_id=? AND title=? AND is_trashed=FALSE LIMIT 1",
			userID, targetTitle).Scan(&targetID)
		if err != nil {
			continue
		}
		if targetID == noteID {
			continue
		}

		_, _ = config.DB.Exec(`
			INSERT IGNORE INTO note_links (source_note_id, target_note_id, user_id, link_text)
			VALUES (?, ?, ?, ?)`,
			noteID, targetID, userID, targetTitle)
	}
}

// ProcessMentions — parse @username di content, buat record note_mentions.
func ProcessMentions(noteID, userID int64, content string) {
	_, _ = config.DB.Exec("DELETE FROM note_mentions WHERE note_id=?", noteID)

	matches := mentionRe.FindAllStringSubmatch(content, -1)
	seen := map[string]bool{}

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		username := m[1]
		if seen[username] {
			continue
		}
		seen[username] = true

		var mentionedID int64
		err := config.DB.QueryRow(
			"SELECT id FROM users WHERE username=? LIMIT 1", username).Scan(&mentionedID)
		if err != nil {
			continue
		}
		if mentionedID == userID {
			continue
		}

		_, _ = config.DB.Exec(`
			INSERT INTO note_mentions (note_id, mentioned_user_id, mentioning_user_id)
			VALUES (?, ?, ?)`, noteID, mentionedID, userID)

		// Buat notification
		var mentionerName string
		_ = config.DB.QueryRow("SELECT username FROM users WHERE id=?", userID).Scan(&mentionerName)

		_, _ = config.DB.Exec(`
			INSERT INTO notifications (user_id, type, title, message, link)
			VALUES (?, 'mention', 'Anda disebut', ?, ?)`,
			mentionedID,
			"@"+mentionerName+" menyebut Anda dalam catatan",
			"/notes?id="+intToStr(noteID))
	}
}

// ============================================================
// BACKLINKS
// ============================================================

func ListBacklinks(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "id")

	rows, err := config.DB.Query(`
		SELECT nl.id, nl.source_note_id, nl.target_note_id, 
		       COALESCE(nl.link_text,''), n.title AS source_title
		FROM note_links nl
		JOIN notes n ON n.id = nl.source_note_id
		WHERE nl.target_note_id = ? AND nl.user_id = ?
		  AND n.is_trashed = FALSE
		ORDER BY nl.created_at DESC`, noteID, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, src, tgt int64
		var linkText, srcTitle string
		_ = rows.Scan(&id, &src, &tgt, &linkText, &srcTitle)
		result = append(result, map[string]interface{}{
			"id":             id,
			"source_note_id": src,
			"target_note_id": tgt,
			"link_text":      linkText,
			"source_title":   srcTitle,
		})
	}
	writeJSON(w, 200, result)
}

func ListOutgoingLinks(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "id")

	rows, err := config.DB.Query(`
		SELECT nl.id, nl.source_note_id, nl.target_note_id,
		       COALESCE(nl.link_text,''), n.title AS target_title
		FROM note_links nl
		JOIN notes n ON n.id = nl.target_note_id
		WHERE nl.source_note_id = ? AND nl.user_id = ?
		  AND n.is_trashed = FALSE
		ORDER BY nl.created_at DESC`, noteID, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, src, tgt int64
		var linkText, tgtTitle string
		_ = rows.Scan(&id, &src, &tgt, &linkText, &tgtTitle)
		result = append(result, map[string]interface{}{
			"id":             id,
			"source_note_id": src,
			"target_note_id": tgt,
			"link_text":      linkText,
			"target_title":   tgtTitle,
		})
	}
	writeJSON(w, 200, result)
}

// ============================================================
// SEARCH NOTE TITLES (untuk autocomplete [[link]])
// ============================================================

func SearchNoteTitles(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	keyword := strings.TrimSpace(r.URL.Query().Get("q"))
	if keyword == "" {
		writeJSON(w, 200, []interface{}{})
		return
	}

	rows, err := config.DB.Query(`
		SELECT id, title FROM notes
		WHERE user_id = ? AND is_trashed = FALSE AND title LIKE ?
		ORDER BY updated_at DESC LIMIT 10`,
		userID, "%"+keyword+"%")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var title string
		_ = rows.Scan(&id, &title)
		result = append(result, map[string]interface{}{"id": id, "title": title})
	}
	writeJSON(w, 200, result)
}

// ============================================================
// RELATED NOTES (shared tags)
// ============================================================

func ListRelatedNotes(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "id")

	rows, err := config.DB.Query(`
		SELECT DISTINCT n.id, n.title, COUNT(DISTINCT nt2.tag_id) AS shared_tags
		FROM notes n
		JOIN note_tags nt1 ON nt1.note_id = ?
		JOIN note_tags nt2 ON nt2.tag_id = nt1.tag_id AND nt2.note_id = n.id
		WHERE n.user_id = ? AND n.id != ? AND n.is_trashed = FALSE
		GROUP BY n.id, n.title
		ORDER BY shared_tags DESC, n.updated_at DESC
		LIMIT 10`, noteID, userID, noteID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var title string
		var sharedTags int
		_ = rows.Scan(&id, &title, &sharedTags)
		result = append(result, map[string]interface{}{
			"id":          id,
			"title":       title,
			"shared_tags": sharedTags,
		})
	}
	writeJSON(w, 200, result)
}

// ============================================================
// USER MENTIONS
// ============================================================

func ListUserMentions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	rows, err := config.DB.Query(`
		SELECT nm.id, nm.note_id, n.title AS note_title, 
		       u.username, u.id AS mentioning_user_id,
		       nm.is_read, nm.created_at
		FROM note_mentions nm
		JOIN notes n ON n.id = nm.note_id
		JOIN users u ON u.id = nm.mentioning_user_id
		WHERE nm.mentioned_user_id = ?
		ORDER BY nm.created_at DESC LIMIT 30`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, noteID, mentioningID int64
		var noteTitle, username string
		var isRead bool
		var t sql.NullTime
		_ = rows.Scan(&id, &noteID, &noteTitle, &username, &mentioningID, &isRead, &t)
		result = append(result, map[string]interface{}{
			"id":              id,
			"note_id":         noteID,
			"note_title":      noteTitle,
			"mentioning_user": username,
			"mentioning_id":   mentioningID,
			"is_read":         isRead,
			"created_at":      t.Time,
		})
	}
	writeJSON(w, 200, result)
}

// ============================================================
// SEARCH USERS (untuk autocomplete @mention)
// ============================================================

func SearchUsers(w http.ResponseWriter, r *http.Request) {
	keyword := strings.TrimSpace(r.URL.Query().Get("q"))
	if keyword == "" {
		writeJSON(w, 200, []interface{}{})
		return
	}

	rows, err := config.DB.Query(`
		SELECT id, username FROM users
		WHERE is_active = TRUE AND username LIKE ?
		ORDER BY username LIMIT 10`, keyword+"%")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var username string
		_ = rows.Scan(&id, &username)
		result = append(result, map[string]interface{}{"id": id, "username": username})
	}
	writeJSON(w, 200, result)
}

// ============================================================
// GRAPH VIEW DATA
// ============================================================

func GetGraphData(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	// Nodes
	rows, err := config.DB.Query(`
		SELECT id, title, notebook_id
		FROM notes
		WHERE user_id = ? AND is_trashed = FALSE
		LIMIT 500`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	nodes := []map[string]interface{}{}
	nodeIDs := map[int64]bool{}

	for rows.Next() {
		var id int64
		var title string
		var nbID sql.NullInt64
		_ = rows.Scan(&id, &title, &nbID)
		n := map[string]interface{}{
			"id":    id,
			"title": title,
		}
		if nbID.Valid {
			n["notebook_id"] = nbID.Int64
		}
		nodes = append(nodes, n)
		nodeIDs[id] = true
	}

	// Edges
	rows2, err := config.DB.Query(`
		SELECT source_note_id, target_note_id
		FROM note_links
		WHERE user_id = ?`, userID)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"nodes": nodes, "edges": []interface{}{}})
		return
	}
	defer rows2.Close()

	edges := []map[string]interface{}{}
	for rows2.Next() {
		var src, tgt int64
		_ = rows2.Scan(&src, &tgt)
		if !nodeIDs[src] || !nodeIDs[tgt] {
			continue
		}
		edges = append(edges, map[string]interface{}{
			"source": src,
			"target": tgt,
		})
	}

	writeJSON(w, 200, map[string]interface{}{
		"nodes": nodes,
		"edges": edges,
	})
}

// ============================================================
// VERSION HISTORY
// ============================================================

// SaveVersion — buat snapshot versi note.
// autoVersionIntervalSec: jarak minimal antar snapshot otomatis (detik).
func autoVersionIntervalSec() int {
	n, err := strconv.Atoi(config.GetEnv("VERSION_AUTO_INTERVAL_SEC", ""))
	if err != nil || n < 1 {
		return 300
	}
	return n
}

func SaveVersion(noteID, userID int64, title, content string, isAuto bool) {
	var lastVersion int
	var lastContent string
	err := config.DB.QueryRow(`
		SELECT version_number, COALESCE(content,'') FROM note_versions
		WHERE note_id = ? ORDER BY version_number DESC LIMIT 1`,
		noteID).Scan(&lastVersion, &lastContent)

	if err != nil {
		lastVersion = 0
	}

	// Skip kalau content tidak berubah
	if lastContent == content {
		return
	}

	// Rate limiting untuk auto: maksimal 1 snapshot per catatan per interval
	// (bawaan 5 menit, env VERSION_AUTO_INTERVAL_SEC). Versi manual tidak dibatasi.
	if isAuto {
		var recentCount int
		_ = config.DB.QueryRow(`
			SELECT COUNT(*) FROM note_versions
			WHERE note_id = ? AND is_auto = TRUE 
			  AND created_at > DATE_SUB(NOW(), INTERVAL ? SECOND)`,
			noteID, autoVersionIntervalSec()).Scan(&recentCount)
		if recentCount > 0 {
			return
		}
	}

	text := stripHTML(content)
	words := len(strings.Fields(text))
	chars := len([]rune(text))

	_, _ = config.DB.Exec(`
		INSERT INTO note_versions 
		    (note_id, user_id, version_number, title, content, content_text, word_count, char_count, is_auto)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		noteID, userID, lastVersion+1, title, content, text, words, chars, isAuto)
}

func ListNoteVersions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "id")

	var count int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM notes WHERE id=? AND user_id=?",
		noteID, userID).Scan(&count)
	if count == 0 {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}

	rows, err := config.DB.Query(`
		SELECT nv.id, nv.version_number, COALESCE(nv.title,''), 
		       nv.word_count, nv.char_count, COALESCE(nv.change_summary,''),
		       nv.is_auto, nv.created_at,
		       COALESCE(u.username,'system') AS username
		FROM note_versions nv
		LEFT JOIN users u ON u.id = nv.user_id
		WHERE nv.note_id = ?
		ORDER BY nv.version_number DESC LIMIT 100`, noteID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var vn, wc, cc int
		var title, summary, username string
		var isAuto bool
		var t sql.NullTime
		_ = rows.Scan(&id, &vn, &title, &wc, &cc, &summary, &isAuto, &t, &username)
		result = append(result, map[string]interface{}{
			"id":             id,
			"version_number": vn,
			"title":          title,
			"word_count":     wc,
			"char_count":     cc,
			"change_summary": summary,
			"is_auto":        isAuto,
			"created_at":     t.Time,
			"username":       username,
		})
	}
	writeJSON(w, 200, result)
}

func GetNoteVersion(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "id")
	versionID, _ := pathInt(r, "version_id")

	var count int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM notes WHERE id=? AND user_id=?",
		noteID, userID).Scan(&count)
	if count == 0 {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}

	var id, vn int64
	var title, content string
	var isAuto bool
	var t sql.NullTime
	err := config.DB.QueryRow(`
		SELECT id, version_number, COALESCE(title,''), COALESCE(content,''), 
		       is_auto, created_at
		FROM note_versions WHERE id = ? AND note_id = ?`,
		versionID, noteID).Scan(&id, &vn, &title, &content, &isAuto, &t)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Versi tidak ditemukan"})
		return
	}

	writeJSON(w, 200, map[string]interface{}{
		"id":             id,
		"version_number": vn,
		"title":          title,
		"content":        content,
		"is_auto":        isAuto,
		"created_at":     t.Time,
	})
}

func RestoreNoteVersion(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "id")
	versionID, _ := pathInt(r, "version_id")

	// Hanya pemilik catatan yang boleh memulihkan versi.
	var ownerCount int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM notes WHERE id=? AND user_id=?",
		noteID, userID).Scan(&ownerCount)
	if ownerCount == 0 {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}

	// Ambil versi
	var title, content string
	err := config.DB.QueryRow(`
		SELECT COALESCE(title,''), COALESCE(content,'') 
		FROM note_versions WHERE id = ? AND note_id = ?`,
		versionID, noteID).Scan(&title, &content)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Versi tidak ditemukan"})
		return
	}

	// Versi lama mungkin tersimpan sebelum ada sanitasi.
	content = sanitizeNoteHTML(content)

	// Save snapshot versi saat ini dulu sebelum restore
	var currTitle, currContent string
	_ = config.DB.QueryRow(
		"SELECT title, content FROM notes WHERE id=?", noteID).
		Scan(&currTitle, &currContent)
	SaveVersion(noteID, userID, currTitle, currContent, false)

	// Update note
	text := stripHTML(content)
	words := len(strings.Fields(text))
	chars := len([]rune(text))

	_, err = config.DB.Exec(`
		UPDATE notes SET title=?, content=?, content_text=?, 
		                 word_count=?, char_count=?, updated_at=NOW()
		WHERE id=? AND user_id=?`,
		title, content, text, words, chars, noteID, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Process link & mention dari content restored
	ProcessNoteLinks(noteID, userID, content)
	ProcessMentions(noteID, userID, content)
	UpdateChecklistStats(noteID, content)

	writeJSON(w, 200, map[string]string{"message": "Versi dipulihkan"})
}

func DeleteNoteVersion(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "id")
	versionID, _ := pathInt(r, "version_id")

	_, _ = config.DB.Exec(`
		DELETE nv FROM note_versions nv
		JOIN notes n ON n.id = nv.note_id
		WHERE nv.id = ? AND nv.note_id = ? AND n.user_id = ?`,
		versionID, noteID, userID)

	writeJSON(w, 200, map[string]string{"message": "Versi dihapus"})
}

// CleanupOldVersions — simpan 50 versi terakhir per note
func CleanupOldVersions(noteID int64) {
	_, _ = config.DB.Exec(`
		DELETE FROM note_versions
		WHERE note_id = ? AND id NOT IN (
			SELECT id FROM (
				SELECT id FROM note_versions
				WHERE note_id = ?
				ORDER BY version_number DESC
				LIMIT 50
			) AS t
		)`, noteID, noteID)
}

// ============================================================
// Helper waktu
// ============================================================

var _ = time.Now
