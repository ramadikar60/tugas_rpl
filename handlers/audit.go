package handlers

// ============================================================
// AUDIT.GO - Log aktivitas (SRS FR-119 sampai FR-124)
//
// Audit(...) membungkus handler: setelah handler SELESAI dengan status 2xx, satu baris
// dicatat di activity_log (pengguna, aksi, jenis dan id item, IP, user agent).
// Permintaan yang gagal (4xx/5xx) tidak dicatat sebagai aksi.
// ============================================================

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

// Deskripsi bawaan tiap aksi (Bahasa Indonesia).
var auditDescriptions = map[string]string{
	"note_create":          "Membuat catatan",
	"note_update":          "Mengubah catatan",
	"note_trash":           "Memindahkan catatan ke sampah",
	"note_restore":         "Memulihkan catatan dari sampah",
	"note_delete":          "Menghapus catatan permanen",
	"note_duplicate":       "Menduplikat catatan",
	"note_lock":            "Mengunci catatan",
	"note_unlock":          "Membuka kunci catatan",
	"note_version_restore": "Memulihkan versi catatan",
	"note_from_template":   "Membuat catatan dari templat",
	"trash_empty":          "Mengosongkan sampah",
	"attachment_upload":    "Mengunggah lampiran",
	"attachment_download":  "Mengunduh lampiran",
	"attachment_rename":    "Mengganti nama lampiran",
	"attachment_delete":    "Menghapus lampiran",
	"share_create":         "Membagikan item",
	"share_update":         "Mengubah izin berbagi",
	"share_revoke":         "Mencabut berbagi",
	"comment_create":       "Menambah komentar",
}

// statusRecorder mencatat kode status (dan awal isi respons bila diminta).
type statusRecorder struct {
	http.ResponseWriter
	status  int
	capture bool
	body    bytes.Buffer
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	if s.capture && s.body.Len() < 4096 {
		room := 4096 - s.body.Len()
		if len(b) < room {
			room = len(b)
		}
		s.body.Write(b[:room])
	}
	return s.ResponseWriter.Write(b)
}

// Flush dan Unwrap menjaga fitur ResponseWriter asli (streaming, ResponseController).
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Audit mencatat aksi pada setiap permintaan yang berhasil.
func Audit(action, itemType string, h http.HandlerFunc) http.HandlerFunc {
	return AuditThrottled(action, itemType, 0, h)
}

// AuditEdit: pengeditan beruntun (simpan otomatis) dicatat sekali per 5 menit per item.
func AuditEdit(action, itemType string, h http.HandlerFunc) http.HandlerFunc {
	return AuditThrottled(action, itemType, 5*time.Minute, h)
}

// AuditThrottled sama seperti Audit, tetapi aksi yang sama pada item yang sama oleh pengguna
// yang sama hanya dicatat sekali per window (mis. pengeditan beruntun oleh simpan otomatis).
func AuditThrottled(action, itemType string, window time.Duration, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, capture: true}
		h(rec, r)

		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		if rec.status < 200 || rec.status >= 300 {
			return
		}
		userID := middleware.GetUserID(r)
		if userID == 0 || config.DB == nil {
			return
		}
		itemID := auditItemID(r, rec)

		if window > 0 && itemID != 0 {
			var recent int
			_ = config.DB.QueryRow(`SELECT COUNT(*) FROM activity_log
				WHERE user_id = ? AND action = ? AND item_id = ? AND created_at > DATE_SUB(NOW(), INTERVAL ? SECOND)`,
				userID, action, itemID, int(window.Seconds())).Scan(&recent)
			if recent > 0 {
				return
			}
		}
		LogActivity(userID, action, itemType, itemID, auditDescriptions[action], clientIP(r), r.UserAgent())
	}
}

// auditItemID mengambil id item dari path ({id}, {note_id}) atau dari isi respons (pembuatan).
func auditItemID(r *http.Request, rec *statusRecorder) int64 {
	for _, key := range []string{"id", "note_id"} {
		if v := r.PathValue(key); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				return n
			}
		}
	}
	var out struct {
		ID json.Number `json:"id"`
	}
	if err := json.Unmarshal(rec.body.Bytes(), &out); err == nil {
		if n, err := out.ID.Int64(); err == nil {
			return n
		}
	}
	return 0
}

// logAuth mencatat peristiwa akun (login, logout, sandi, sesi).
func logAuth(r *http.Request, userID int64, action, description string) {
	if userID == 0 || config.DB == nil {
		return
	}
	LogActivity(userID, action, "user", userID, description, clientIP(r), r.UserAgent())
}
