package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"notebook-app/auth"
	"notebook-app/config"
	"notebook-app/middleware"
	"notebook-app/models"
)

const (
	accessCookie  = "access_token"
	refreshCookie = "refresh_token"
	accessTTL     = 15 * time.Minute
	refreshTTL    = 7 * 24 * time.Hour

	// Dua tab yang me-refresh bersamaan memakai token yang sama. Selama jendela ini,
	// pemakaian ulang token hasil rotasi dianggap wajar (bukan pencurian).
	refreshReuseGraceSec = 10

	loginFailedMsg = "Email atau password salah"
	tooManyMsg     = "Terlalu banyak percobaan. Coba lagi beberapa menit lagi."
)

func issueTokens(w http.ResponseWriter, r *http.Request, userID int64) (access, refresh string, err error) {
	access, err = auth.GenerateAccessToken(userID)
	if err != nil {
		return
	}
	refresh, err = auth.GenerateRefreshToken()
	if err != nil {
		return
	}

	_, err = config.DB.Exec(
		`INSERT INTO refresh_tokens (user_id, token, user_agent, ip, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		userID, refresh, r.UserAgent(), clientIP(r), time.Now().Add(refreshTTL),
	)
	if err != nil {
		return
	}

	setAuthCookie(w, r, accessCookie, access, int(accessTTL.Seconds()))
	setAuthCookie(w, r, refreshCookie, refresh, int(refreshTTL.Seconds()))
	return
}

func clientIP(r *http.Request) string {
	return middleware.ClientIP(r)
}

// validateRegistration memeriksa email, username, dan password untuk akun baru.
func validateRegistration(email, username, password string) error {
	if email == "" || username == "" || password == "" {
		return errMsg("Email, username, dan password wajib diisi")
	}
	if len(email) > 254 || strings.ContainsAny(email, "<>\r\n \t\"") {
		return errMsg("Format email tidak valid")
	}
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || !strings.Contains(a.Address, "@") {
		return errMsg("Format email tidak valid")
	}
	if n := utf8.RuneCountInString(username); n < 2 || n > 50 {
		return errMsg("Username harus 2 sampai 50 karakter")
	}
	for _, r := range username {
		if unicode.IsControl(r) || r == '<' || r == '>' {
			return errMsg("Username mengandung karakter yang tidak diizinkan")
		}
	}
	return auth.ValidateAccountPassword(password, email, username)
}

type errMsg string

func (e errMsg) Error() string { return string(e) }

// ============ REGISTER ============
// Register — buat user baru TANPA auto-login. User harus login manual.
func Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Username = strings.TrimSpace(req.Username)

	if err := validateRegistration(req.Email, req.Username, req.Password); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	res, err := config.DB.Exec(
		"INSERT INTO users (email, username, password_hash) VALUES (?, ?, ?)",
		req.Email, req.Username, hash,
	)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Email sudah terdaftar"})
		return
	}
	id, _ := res.LastInsertId()

	logAuth(r, id, "register", "Akun dibuat")
	// TIDAK generate token — user login manual
	writeJSON(w, 201, map[string]interface{}{
		"message": "Registrasi berhasil. Silakan login.",
		"user":    models.User{ID: id, Email: req.Email, Username: req.Username},
	})
}

// ============ LOGIN ============
func Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" || req.Password == "" {
		writeJSON(w, 400, map[string]string{"error": "Email dan password wajib diisi"})
		return
	}

	// Kunci akun memakai email huruf kecil, termasuk untuk email yang tidak ada,
	// sehingga pembatas tidak membocorkan apakah akun terdaftar.
	acctKey := strings.ToLower(email)
	ip := clientIP(r)

	if secs := loginLockRemaining(acctKey, ip); secs > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeJSON(w, 429, map[string]string{"error": tooManyMsg})
		return
	}

	var user models.User
	var hash string
	var isActive bool
	err := config.DB.QueryRow(
		"SELECT id, email, username, password_hash, is_active FROM users WHERE email = ?",
		email,
	).Scan(&user.ID, &user.Email, &user.Username, &hash, &isActive)

	if err == sql.ErrNoRows {
		auth.DummyVerify(req.Password) // samakan waktu respons dengan akun yang ada
		recordLoginFailure(acctKey, ip)
		writeJSON(w, 401, map[string]string{"error": loginFailedMsg})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "Server error"})
		return
	}

	// Sandi diperiksa SEBELUM status akun, agar status nonaktif tidak bocor ke orang
	// yang tidak tahu sandinya.
	if !auth.VerifyPassword(hash, req.Password) {
		logAuth(r, user.ID, "login_failed", "Percobaan login gagal")
		recordLoginFailure(acctKey, ip)
		writeJSON(w, 401, map[string]string{"error": loginFailedMsg})
		return
	}
	if !isActive {
		writeJSON(w, 403, map[string]string{"error": "Akun Anda dinonaktifkan"})
		return
	}

	clearLoginFailures(acctKey)

	// Upgrade hash lama (SHA-256 satu putaran) ke PBKDF2 saat sandi terbukti benar.
	if auth.NeedsRehash(hash) {
		if newHash, herr := auth.HashPassword(req.Password); herr == nil {
			_, _ = config.DB.Exec("UPDATE users SET password_hash = ? WHERE id = ?", newHash, user.ID)
		}
	}

	// Update last_login_at
	_, _ = config.DB.Exec("UPDATE users SET last_login_at = NOW() WHERE id = ?", user.ID)

	// Bersihkan refresh token yang sudah lama kedaluwarsa.
	_, _ = config.DB.Exec(
		"DELETE FROM refresh_tokens WHERE user_id = ? AND expires_at < DATE_SUB(NOW(), INTERVAL 1 DAY)", user.ID)

	access, _, err := issueTokens(w, r, user.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "Server error"})
		return
	}

	csrf := auth.GenerateCSRFToken(access)
	logAuth(r, user.ID, "login", "Login berhasil")

	// Ambil role user
	var role string
	_ = config.DB.QueryRow("SELECT role FROM users WHERE id=?", user.ID).Scan(&role)

	writeJSON(w, 200, map[string]interface{}{
		"token":      access,
		"csrf_token": csrf,
		"user": map[string]interface{}{
			"id":       user.ID,
			"email":    user.Email,
			"username": user.Username,
			"role":     role,
		},
	})
}

// ============ REFRESH ============
// Rotasi refresh token dengan deteksi pemakaian ulang:
//   - token valid          : dicabut (revoked_at diisi), token baru diterbitkan
//   - dicabut oleh rotasi, <= 10 detik lalu : dua tab bersamaan; hanya token akses baru
//   - dicabut oleh rotasi, > 10 detik lalu  : pemakaian ulang -> semua sesi dicabut
//   - dicabut eksplisit (logout / cabut sesi): ditolak biasa, tanpa efek ke sesi lain
func Refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookie)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "No refresh token"})
		return
	}

	revokedAge := "NULL"
	if authSchemaReady {
		revokedAge = "TIMESTAMPDIFF(SECOND, rt.revoked_at, NOW())"
	}

	var userID int64
	var expires time.Time
	var revoked, isActive bool
	var age sql.NullInt64
	err = config.DB.QueryRow(
		`SELECT rt.user_id, rt.expires_at, rt.revoked, u.is_active, `+revokedAge+`
		 FROM refresh_tokens rt JOIN users u ON u.id = rt.user_id
		 WHERE rt.token = ?`,
		c.Value,
	).Scan(&userID, &expires, &revoked, &isActive, &age)

	if err != nil || time.Now().After(expires) {
		writeJSON(w, 401, map[string]string{"error": "Invalid refresh token"})
		return
	}

	// Akun dinonaktifkan: semua sesi berakhir (token akses terakhir habis dalam <= 15 menit).
	if !isActive {
		revokeAllRefreshTokens(userID)
		clearAuthCookies(w, r)
		writeJSON(w, 401, map[string]string{"error": "Invalid refresh token"})
		return
	}

	if revoked {
		switch {
		case !authSchemaReady || !age.Valid:
			// Dicabut eksplisit (atau skema lama): tolak saja.
			writeJSON(w, 401, map[string]string{"error": "Invalid refresh token"})
		case age.Int64 <= refreshReuseGraceSec:
			refreshGraceResponse(w, userID)
		default:
			log.Printf("[auth] refresh token hasil rotasi dipakai ulang (user %d, ip %s): semua sesi dicabut", userID, clientIP(r))
			revokeAllRefreshTokens(userID)
			clearAuthCookies(w, r)
			writeJSON(w, 401, map[string]string{"error": "Invalid refresh token"})
		}
		return
	}

	// Rotasi. UPDATE bersyarat memastikan hanya satu dari dua request bersamaan yang menang.
	rotate := "UPDATE refresh_tokens SET revoked = TRUE WHERE token = ? AND revoked = FALSE"
	if authSchemaReady {
		rotate = "UPDATE refresh_tokens SET revoked = TRUE, revoked_at = NOW() WHERE token = ? AND revoked = FALSE"
	}
	res, err := config.DB.Exec(rotate, c.Value)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "Server error"})
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		// Kalah balapan dengan request lain yang memakai token yang sama.
		refreshGraceResponse(w, userID)
		return
	}

	access, _, err := issueTokens(w, r, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "Server error"})
		return
	}

	csrf := auth.GenerateCSRFToken(access)
	writeJSON(w, 200, map[string]interface{}{
		"token":      access,
		"csrf_token": csrf,
	})
}

// refreshGraceResponse: hanya token akses baru; cookie milik request pemenang tidak disentuh,
// dan tidak ada refresh token baru (penyerang yang memutar ulang tidak mendapat sesi permanen).
func refreshGraceResponse(w http.ResponseWriter, userID int64) {
	access, err := auth.GenerateAccessToken(userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "Server error"})
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"token":      access,
		"csrf_token": auth.GenerateCSRFToken(access),
	})
}

// ============ LOGOUT ============
func Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(refreshCookie); err == nil {
		var uid int64
		if config.DB.QueryRow("SELECT user_id FROM refresh_tokens WHERE token = ?", c.Value).Scan(&uid) == nil {
			logAuth(r, uid, "logout", "Logout")
		}
		_, _ = config.DB.Exec("UPDATE refresh_tokens SET revoked = TRUE WHERE token = ?", c.Value)
	}
	clearAuthCookies(w, r)
	writeJSON(w, 200, map[string]string{"message": "Logged out"})
}

// ============ CSRF ============
func CSRFToken(w http.ResponseWriter, r *http.Request) {
	access := middleware.GetAccessToken(r)
	writeJSON(w, 200, map[string]string{
		"csrf_token": auth.GenerateCSRFToken(access),
	})
}

// ============ FORGOT / RESET PASSWORD ============

const forgotMsg = "Jika email terdaftar, tautan untuk mengatur ulang password telah dikirim."

// ForgotPassword: respons SELALU sama (tidak membocorkan apakah email terdaftar), dan
// token TIDAK pernah dikembalikan ke pemanggil. Token hanya sampai lewat email.
// Hanya hash SHA-256 token yang disimpan di database.
func ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" || len(email) > 254 {
		writeJSON(w, 400, map[string]string{"error": "Email wajib diisi"})
		return
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		writeJSON(w, 500, map[string]string{"error": "Server error"})
		return
	}
	raw := hex.EncodeToString(b)

	// Jeda 1 menit per akun: bila token baru saja diterbitkan, tidak membuat yang baru
	// (mencegah membanjiri kotak masuk korban). Akun nonaktif tidak mendapat token.
	res, err := config.DB.Exec(
		`UPDATE users SET reset_token = ?, reset_expires = DATE_ADD(NOW(), INTERVAL 1 HOUR)
		 WHERE email = ? AND is_active = TRUE
		   AND (reset_expires IS NULL OR reset_expires < DATE_ADD(NOW(), INTERVAL 59 MINUTE))`,
		hashToken(raw), email,
	)
	if err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			// Di goroutine agar waktu respons sama untuk email terdaftar maupun tidak.
			go sendResetMail(email, raw)
		}
	}

	writeJSON(w, 200, map[string]string{"message": forgotMsg})
}

func ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}
	token := strings.TrimSpace(req.Token)
	invalid := map[string]string{"error": "Token tidak valid atau kedaluwarsa"}
	if len(token) != 64 {
		writeJSON(w, 400, invalid)
		return
	}
	hashed := hashToken(token)

	var userID int64
	var email, username string
	err := config.DB.QueryRow(
		`SELECT id, email, username FROM users
		 WHERE reset_token = ? AND reset_expires > NOW() AND is_active = TRUE`,
		hashed,
	).Scan(&userID, &email, &username)
	if err != nil {
		writeJSON(w, 400, invalid)
		return
	}

	// Sandi ditolak: token tetap berlaku agar pengguna bisa mencoba lagi.
	if err := auth.ValidateAccountPassword(req.NewPassword, email, username); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	// Sekali pakai dan atomik: hanya satu request yang bisa mengosongkan token.
	res, err := config.DB.Exec(
		`UPDATE users SET password_hash = ?, reset_token = NULL, reset_expires = NULL
		 WHERE id = ? AND reset_token = ?`,
		hash, userID, hashed,
	)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "Server error"})
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		writeJSON(w, 400, invalid)
		return
	}

	// Semua sesi lama berakhir, dan hitungan gagal-login akun dibersihkan.
	revokeAllRefreshTokens(userID)
	logAuth(r, userID, "password_reset", "Password direset lewat tautan email")
	clearLoginFailures(strings.ToLower(email))

	writeJSON(w, 200, map[string]string{"message": "Password berhasil direset. Silakan masuk."})
}

// ============ PROFILE ============
func GetProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	var user models.User
	var role string
	var isActive bool
	err := config.DB.QueryRow(
		`SELECT id, email, username, COALESCE(avatar,''), role, is_active, created_at
		 FROM users WHERE id = ?`,
		userID,
	).Scan(&user.ID, &user.Email, &user.Username, &user.Avatar, &role, &isActive, &user.CreatedAt)

	if err == sql.ErrNoRows {
		writeJSON(w, 404, map[string]string{"error": "User tidak ditemukan"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	user.Role = role
	user.IsActive = isActive
	writeJSON(w, 200, user)
}

func UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	var req struct {
		Username string `json:"username"`
		Avatar   string `json:"avatar"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}
	if strings.TrimSpace(req.Username) == "" {
		writeJSON(w, 400, map[string]string{"error": "Username wajib diisi"})
		return
	}
	_, err := config.DB.Exec(
		"UPDATE users SET username = ?, avatar = ? WHERE id = ?",
		req.Username, req.Avatar, userID,
	)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Profile diupdate"})
}

// ============ CHANGE PASSWORD (untuk user sendiri) ============
func ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	var currentHash, email, username string
	err := config.DB.QueryRow(
		"SELECT password_hash, email, username FROM users WHERE id = ?", userID,
	).Scan(&currentHash, &email, &username)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "User tidak ditemukan"})
		return
	}

	if !auth.VerifyPassword(currentHash, req.OldPassword) {
		// 400, bukan 403: frontend memperlakukan 403 sebagai masalah CSRF dan mengulang request.
		writeJSON(w, 400, map[string]string{"error": "Password lama salah"})
		return
	}

	if err := auth.ValidateAccountPassword(req.NewPassword, email, username); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	_, _ = config.DB.Exec(
		"UPDATE users SET password_hash = ? WHERE id = ?", newHash, userID)

	// Revoke semua refresh token (paksa login ulang di semua perangkat)
	revokeAllRefreshTokens(userID)
	logAuth(r, userID, "password_change", "Password diubah")
	clearAuthCookies(w, r)

	writeJSON(w, 200, map[string]string{"message": "Password berhasil diubah"})
}
