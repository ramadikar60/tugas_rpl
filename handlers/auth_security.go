package handlers

// ============================================================
// AUTH_SECURITY.GO - Pendukung keamanan akun (Prompt 22A)
//  - cookie auth (flag Secure di HTTPS)
//  - pencabutan refresh token
//  - pembatas percobaan login per akun dan per IP (jeda bertahap)
//  - daftar perangkat/sesi aktif
// ============================================================

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"math/rand"
	"net/http"
	"strconv"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ---------- Skema ----------

// authSchemaReady true bila schema_v10.sql sudah diterapkan. Bila belum, fitur yang
// bergantung padanya (pembatas login, deteksi pemakaian ulang token) dinonaktifkan
// dan aplikasi tetap berjalan seperti sebelumnya.
var authSchemaReady bool

// CheckAuthSchema dipanggil sekali di main.go setelah InitDB().
func CheckAuthSchema() {
	var tables, cols int
	_ = config.DB.QueryRow(`SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'login_attempts'`).Scan(&tables)
	_ = config.DB.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'refresh_tokens' AND column_name = 'revoked_at'`).Scan(&cols)
	authSchemaReady = tables > 0 && cols > 0
	if !authSchemaReady {
		log.Printf("PERINGATAN: schema_v10.sql belum diterapkan. Pembatas login dan deteksi pemakaian ulang refresh token NONAKTIF. Jalankan schema_v10.sql.")
	}
}

// ---------- Cookie ----------

func setAuthCookie(w http.ResponseWriter, r *http.Request, name, value string, maxAgeSec int) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/", HttpOnly: true,
		Secure:   middleware.IsHTTPS(r),
		SameSite: http.SameSiteLaxMode, MaxAge: maxAgeSec,
	})
}

func clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	setAuthCookie(w, r, accessCookie, "", -1)
	setAuthCookie(w, r, refreshCookie, "", -1)
}

// ---------- Token ----------

// hashToken: SHA-256 hex. Token reset disimpan sebagai hash ini, bukan teks polos.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// revokeAllRefreshTokens mencabut semua sesi pengguna. revoked_at sengaja TIDAK diisi:
// pencabutan massal bukan rotasi, jadi tidak boleh masuk jendela toleransi refresh.
func revokeAllRefreshTokens(userID int64) {
	_, _ = config.DB.Exec("UPDATE refresh_tokens SET revoked = TRUE WHERE user_id = ?", userID)
}

// ---------- Pembatas percobaan login ----------

const (
	accountLockThreshold   = 10 // gagal berturut-turut per akun sebelum dikunci
	ipLockThreshold        = 30 // gagal berturut-turut per IP (IP bersama/NAT butuh batas lebih longgar)
	throttleStaleMinutes   = 15 // tanpa percobaan selama ini = hitungan mulai dari nol
	throttleMaxLockMinutes = 30
)

// loginLockRemaining mengembalikan sisa detik terkunci (0 = tidak terkunci)
// untuk akun ATAU IP. Dengan sengaja tidak memberi tahu mana yang terkunci.
func loginLockRemaining(accountKey, ip string) int {
	if !authSchemaReady {
		return 0
	}
	var secs int
	err := config.DB.QueryRow(`
		SELECT COALESCE(MAX(TIMESTAMPDIFF(SECOND, NOW(), locked_until)), 0)
		FROM login_attempts
		WHERE locked_until > NOW()
		  AND ((scope = 'account' AND identifier = ?) OR (scope = 'ip' AND identifier = ?))`,
		accountKey, ip).Scan(&secs)
	if err != nil {
		log.Printf("[auth] gagal membaca login_attempts: %v", err)
		return 0 // gagal-terbuka: lebih baik tanpa pembatas daripada semua orang terkunci
	}
	if secs < 1 {
		return 0
	}
	return secs
}

// recordLoginFailure mencatat satu kegagalan untuk akun (termasuk email yang tidak
// ada, agar tidak membocorkan keberadaan akun) dan untuk IP.
func recordLoginFailure(accountKey, ip string) {
	if !authSchemaReady {
		return
	}
	recordFailure("account", accountKey, accountLockThreshold)
	recordFailure("ip", ip, ipLockThreshold)

	// Pembersihan sesekali agar tabel tidak membengkak.
	if rand.Intn(50) == 0 {
		_, _ = config.DB.Exec(`DELETE FROM login_attempts
			WHERE last_attempt_at < DATE_SUB(NOW(), INTERVAL 1 DAY)
			  AND (locked_until IS NULL OR locked_until < NOW())`)
	}
}

func recordFailure(scope, id string, threshold int) {
	if id == "" {
		return
	}
	if len(id) > 255 {
		id = id[:255]
	}
	tx, err := config.DB.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()

	_, _ = tx.Exec(`INSERT IGNORE INTO login_attempts (scope, identifier, fail_count, last_attempt_at)
		VALUES (?, ?, 0, NOW())`, scope, id)

	var count int
	var stale, locked bool
	err = tx.QueryRow(`
		SELECT fail_count,
		       (last_attempt_at < DATE_SUB(NOW(), INTERVAL ? MINUTE) AND (locked_until IS NULL OR locked_until <= NOW())),
		       (locked_until IS NOT NULL AND locked_until > NOW())
		FROM login_attempts WHERE scope = ? AND identifier = ? FOR UPDATE`,
		throttleStaleMinutes, scope, id).Scan(&count, &stale, &locked)
	if err != nil {
		return
	}
	if locked {
		_ = tx.Commit() // sudah terkunci: percobaan ditolak di depan, tidak menambah hitungan
		return
	}
	if stale {
		count = 0
	}
	count++

	lockMin := 0
	if count >= threshold {
		shift := count - threshold
		if shift > 5 {
			shift = 5
		}
		lockMin = 1 << shift // 1, 2, 4, 8, 16, 32 menit
		if lockMin > throttleMaxLockMinutes {
			lockMin = throttleMaxLockMinutes
		}
	}
	if lockMin > 0 {
		_, _ = tx.Exec(`UPDATE login_attempts SET fail_count = ?, last_attempt_at = NOW(),
			locked_until = DATE_ADD(NOW(), INTERVAL ? MINUTE) WHERE scope = ? AND identifier = ?`,
			count, lockMin, scope, id)
	} else {
		_, _ = tx.Exec(`UPDATE login_attempts SET fail_count = ?, last_attempt_at = NOW(),
			locked_until = NULL WHERE scope = ? AND identifier = ?`, count, scope, id)
	}
	_ = tx.Commit()
}

// clearLoginFailures dipanggil saat login berhasil atau sandi direset.
func clearLoginFailures(accountKey string) {
	if !authSchemaReady {
		return
	}
	_, _ = config.DB.Exec("DELETE FROM login_attempts WHERE scope = 'account' AND identifier = ?", accountKey)
}

// ---------- Sesi / perangkat ----------

type sessionInfo struct {
	ID        int64  `json:"id"`
	UserAgent string `json:"user_agent"`
	IP        string `json:"ip"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	Current   bool   `json:"current"`
}

// ListSessions: perangkat dengan refresh token aktif milik pengguna.
func ListSessions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	current := ""
	if c, err := r.Cookie(refreshCookie); err == nil {
		current = c.Value
	}

	rows, err := config.DB.Query(`
		SELECT id, COALESCE(user_agent, ''), COALESCE(ip, ''), created_at, expires_at, token
		FROM refresh_tokens
		WHERE user_id = ? AND revoked = FALSE AND expires_at > NOW()
		ORDER BY created_at DESC LIMIT 50`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "Server error"})
		return
	}
	defer rows.Close()

	out := []sessionInfo{}
	for rows.Next() {
		var s sessionInfo
		var created, expires, tok string
		if err := rows.Scan(&s.ID, &s.UserAgent, &s.IP, &created, &expires, &tok); err != nil {
			continue
		}
		s.CreatedAt, s.ExpiresAt = created, expires
		s.Current = current != "" && tok == current
		if len(s.UserAgent) > 300 {
			s.UserAgent = s.UserAgent[:300]
		}
		out = append(out, s)
	}
	writeJSON(w, 200, out)
}

// RevokeSession: keluar dari satu perangkat.
func RevokeSession(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "ID tidak valid"})
		return
	}

	var tok string
	if err := config.DB.QueryRow("SELECT token FROM refresh_tokens WHERE id = ? AND user_id = ?", id, userID).Scan(&tok); err != nil {
		writeJSON(w, 404, map[string]string{"error": "Sesi tidak ditemukan"})
		return
	}
	// Pencabutan eksplisit (bukan rotasi): revoked_at dibiarkan kosong.
	_, _ = config.DB.Exec("UPDATE refresh_tokens SET revoked = TRUE WHERE id = ? AND user_id = ?", id, userID)

	if c, err := r.Cookie(refreshCookie); err == nil && c.Value == tok {
		clearAuthCookies(w, r)
	}
	logAuth(r, userID, "session_revoke", "Mencabut satu sesi login")
	writeJSON(w, 200, map[string]string{"message": "Sesi dicabut"})
}

// LogoutAll: keluar dari semua perangkat.
func LogoutAll(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	revokeAllRefreshTokens(userID)
	clearAuthCookies(w, r)
	logAuth(r, userID, "logout_all", "Keluar dari semua perangkat")
	writeJSON(w, 200, map[string]string{"message": "Keluar dari semua perangkat"})
}
