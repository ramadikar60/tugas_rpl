package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
	"notebook-app/models"
)

const maxUploadSize = 50 << 20 // 50 MB

// ============ UPLOAD ============
func UploadAttachment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "note_id")

	// Verifikasi pemilik
	var count int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM notes WHERE id=? AND user_id=?",
		noteID, userID).Scan(&count)
	if count == 0 {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeJSON(w, 400, map[string]string{"error": "File terlalu besar"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "File tidak ditemukan"})
		return
	}
	defer file.Close()

	// Tipe ditentukan dari ISI file; header Content-Type dan ekstensi dari klien
	// tidak dipercaya. Ekstensi penyimpanan diturunkan dari tipe terdeteksi.
	buf := make([]byte, 512)
	n, _ := io.ReadFull(file, buf)
	_, _ = file.Seek(0, 0)
	mimeType, ext, ok := classifyUpload(buf[:n], header.Filename)
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "Tipe file tidak diizinkan"})
		return
	}
	origName := cleanFilename(header.Filename)

	randBytes := make([]byte, 16)
	_, _ = rand.Read(randBytes)
	storedName := hex.EncodeToString(randBytes) + ext

	dir := uploadDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		writeJSON(w, 500, map[string]string{"error": "Gagal menyiapkan folder"})
		return
	}

	dst, err := os.Create(filepath.Join(dir, storedName))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "Gagal menyimpan file"})
		return
	}
	defer dst.Close()

	size, _ := io.Copy(dst, file)

	res, err := config.DB.Exec(
		"INSERT INTO attachments (note_id, user_id, filename, stored_name, mime_type, size) VALUES (?, ?, ?, ?, ?, ?)",
		noteID, userID, origName, storedName, mimeType, size,
	)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	// ✨ AUTO-REGISTER KE MEDIA LIBRARY
	autoRegisterMedia(id, userID, origName, mimeType)

	writeJSON(w, 201, map[string]interface{}{
		"id":          id,
		"filename":    origName,
		"stored_name": storedName,
		"mime_type":   mimeType,
		"size":        size,
		"url":         "/uploads/" + storedName,
	})
}

// ============ AUTO-REGISTER MEDIA ============
// Setiap attachment yang di-upload otomatis tercatat di media_attachments
// supaya muncul di Media Library.
//
// CATATAN: fungsi detectMediaType() ada di media.go (package handlers yang sama)
func autoRegisterMedia(attachmentID, userID int64, filename, mimeType string) {
	mediaType := detectMediaType(mimeType)

	// Cek apakah sudah terdaftar (untuk mencegah duplikat)
	var exists int
	_ = config.DB.QueryRow(
		"SELECT COUNT(*) FROM media_attachments WHERE attachment_id = ?",
		attachmentID).Scan(&exists)
	if exists > 0 {
		return
	}

	_, _ = config.DB.Exec(`
		INSERT INTO media_attachments 
		    (attachment_id, user_id, media_type, title)
		VALUES (?, ?, ?, ?)`,
		attachmentID, userID, mediaType, filename)
}

// ============ DELETE ============
func DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var storedName string
	err := config.DB.QueryRow(
		"SELECT stored_name FROM attachments WHERE id=? AND user_id=?", id, userID,
	).Scan(&storedName)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Attachment tidak ditemukan"})
		return
	}

	_ = os.Remove(filepath.Join(uploadDir(), storedName))

	// Hapus juga record di media_attachments (kalau ada)
	_, _ = config.DB.Exec("DELETE FROM media_attachments WHERE attachment_id=?", id)

	_, _ = config.DB.Exec("DELETE FROM attachments WHERE id=? AND user_id=?", id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// ============ CLIPBOARD IMAGE ============
func UploadClipboardImage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "note_id")

	var req struct {
		DataURL  string `json:"data_url"`
		Filename string `json:"filename"`
	}
	if err := readJSONMax(r, &req, imageJSONLimit); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid body"})
		return
	}

	parts := strings.SplitN(req.DataURL, ",", 2)
	if len(parts) != 2 {
		writeJSON(w, 400, map[string]string{"error": "Invalid data URL"})
		return
	}

	raw, err := decodeBase64(parts[1])
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Decode gagal"})
		return
	}

	// Hanya gambar raster; tipe dari isi, bukan dari klien.
	head := raw
	if len(head) > 512 {
		head = head[:512]
	}
	clipMime := http.DetectContentType(head)
	clipExt, okImg := imageExtForMIME(clipMime)
	if !okImg || len(raw) == 0 {
		writeJSON(w, 400, map[string]string{"error": "Isi bukan gambar yang didukung"})
		return
	}

	randBytes := make([]byte, 16)
	_, _ = rand.Read(randBytes)
	storedName := hex.EncodeToString(randBytes) + clipExt

	dir := uploadDir()
	_ = os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, storedName)

	if err := os.WriteFile(path, raw, 0644); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	filename := cleanFilename(req.Filename)
	if req.Filename == "" {
		filename = "pasted" + clipExt
	}

	res, err := config.DB.Exec(
		"INSERT INTO attachments (note_id, user_id, filename, stored_name, mime_type, size) VALUES (?, ?, ?, ?, ?, ?)",
		noteID, userID, filename, storedName, clipMime, len(raw),
	)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	// ✨ AUTO-REGISTER KE MEDIA LIBRARY
	autoRegisterMedia(id, userID, filename, clipMime)

	writeJSON(w, 201, map[string]interface{}{
		"id":   id,
		"url":  "/uploads/" + storedName,
		"size": len(raw),
	})
}

// ============ RENAME ============
func RenameAttachment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Filename string `json:"filename"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Filename) == "" {
		writeJSON(w, 400, map[string]string{"error": "Nama file wajib diisi"})
		return
	}

	if len(req.Filename) > 500 {
		writeJSON(w, 400, map[string]string{"error": "Nama file terlalu panjang"})
		return
	}

	res, err := config.DB.Exec(
		"UPDATE attachments SET filename=? WHERE id=? AND user_id=?",
		req.Filename, id, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "Attachment tidak ditemukan"})
		return
	}

	// Sinkronkan nama di media_attachments
	_, _ = config.DB.Exec(
		"UPDATE media_attachments SET title=? WHERE attachment_id=? AND user_id=?",
		req.Filename, id, userID)

	writeJSON(w, 200, map[string]string{"message": "Renamed"})
}

// ============ DOWNLOAD ============
func DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var filename, storedName, mimeType string
	err := config.DB.QueryRow(
		"SELECT filename, stored_name, mime_type FROM attachments WHERE id=? AND user_id=?",
		id, userID).Scan(&filename, &storedName, &mimeType)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	path := filepath.Join(uploadDir(), storedName)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("Content-Type", mimeType)
	http.ServeFile(w, r, path)
}

// ============ PREVIEW ============
func PreviewAttachment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var storedName, mimeType string
	err := config.DB.QueryRow(
		"SELECT stored_name, mime_type FROM attachments WHERE id=? AND user_id=?",
		id, userID).Scan(&storedName, &mimeType)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, filepath.Join(uploadDir(), storedName))
}

// ============ LIST NOTE ATTACHMENTS ============
func ListNoteAttachments(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	noteID, _ := pathInt(r, "note_id")

	rows, err := config.DB.Query(`
		SELECT id, note_id, filename, stored_name, mime_type, size, created_at
		FROM attachments
		WHERE note_id=? AND user_id=?
		ORDER BY created_at DESC`, noteID, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []models.Attachment{}
	for rows.Next() {
		a := &models.Attachment{}
		var created time.Time
		_ = rows.Scan(&a.ID, &a.NoteID, &a.Filename, &a.StoredName,
			&a.MimeType, &a.Size, &created)
		a.CreatedAt = created
		a.URL = "/uploads/" + a.StoredName
		result = append(result, *a)
	}
	writeJSON(w, 200, result)
}
