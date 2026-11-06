package handlers

// Tes alur akun (Prompt 22A). Tes yang memerlukan DB dilewati kecuali NOTEBOOK_TEST_DB=1
// dan schema_v10.sql sudah diterapkan pada DB uji.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

var ipCounter int32

func testIP() string {
	n := atomic.AddInt32(&ipCounter, 1)
	return fmt.Sprintf("198.51.100.%d", n%250+1)
}

func authSetup(t *testing.T) {
	t.Helper()
	integrationDB(t)
	CheckAuthSchema()
	if !authSchemaReady {
		t.Fatalf("schema_v10.sql belum diterapkan pada DB uji")
	}
	_, _ = config.DB.Exec("DELETE FROM login_attempts")
	t.Setenv("TRUST_PROXY", "false")
}

func legacyHash(pw string) string {
	salt, _ := hex.DecodeString("00112233445566778899aabbccddeeff")
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(pw))
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(h.Sum(nil))
}

// makeCredUser membuat pengguna dengan hash FORMAT LAMA (cepat diverifikasi).
func makeCredUser(t *testing.T, password string, active bool) (id int64, email string) {
	t.Helper()
	n := time.Now().UnixNano()
	email = fmt.Sprintf("cred%d@example.test", n)
	res, err := config.DB.Exec("INSERT INTO users (email, username, password_hash, is_active) VALUES (?, ?, ?, ?)",
		email, fmt.Sprintf("cred%d", n), legacyHash(password), active)
	if err != nil {
		t.Fatalf("buat user: %v", err)
	}
	id, _ = res.LastInsertId()
	t.Cleanup(func() { _, _ = config.DB.Exec("DELETE FROM users WHERE id=?", id) })
	return id, email
}

func doAuth(h http.HandlerFunc, method, path string, body interface{}, ip string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	var rd io.Reader = bytes.NewReader(nil)
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":5000"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func jsonField(rec *httptest.ResponseRecorder, key string) string {
	var m map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if v, ok := m[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func loginOK(t *testing.T, email, pw, ip string) *httptest.ResponseRecorder {
	t.Helper()
	rec := doAuth(Login, "POST", "/api/auth/login", map[string]string{"email": email, "password": pw}, ip)
	if rec.Code != 200 {
		t.Fatalf("login gagal: %d %s", rec.Code, rec.Body.String())
	}
	return rec
}

func tokenState(t *testing.T, token string) (revoked, hasRevokedAt bool) {
	t.Helper()
	var r, ra int
	if err := config.DB.QueryRow("SELECT revoked, revoked_at IS NOT NULL FROM refresh_tokens WHERE token=?", token).Scan(&r, &ra); err != nil {
		t.Fatalf("token tidak ada: %v", err)
	}
	return r == 1, ra == 1
}

func TestIntegrationRegisterEnforcesPolicy(t *testing.T) {
	authSetup(t)
	ip := testIP()
	n := time.Now().UnixNano()
	email := fmt.Sprintf("reg%d@example.test", n)
	user := fmt.Sprintf("reguser%d", n)
	t.Cleanup(func() { _, _ = config.DB.Exec("DELETE FROM users WHERE email=?", email) })

	bad := []map[string]string{
		{"email": email, "username": user, "password": "pendek1!"},
		{"email": email, "username": user, "password": "Password123!"},
		{"email": email, "username": user, "password": "qwertyuiop"},
		{"email": email, "username": user, "password": user + "-xyz-1"},
		{"email": "bukan-email", "username": user, "password": "kuda-laut-biru-77"},
		{"email": "A <" + email + ">", "username": user, "password": "kuda-laut-biru-77"},
		{"email": email, "username": "<b>x</b>", "password": "kuda-laut-biru-77"},
		{"email": email, "username": "x", "password": "kuda-laut-biru-77"},
	}
	for i, b := range bad {
		if rec := doAuth(Register, "POST", "/x", b, ip); rec.Code != 400 {
			t.Errorf("kasus %d (%v) harus 400, dapat %d: %s", i, b, rec.Code, rec.Body.String())
		}
	}
	var cnt int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM users WHERE email=?", email).Scan(&cnt)
	if cnt != 0 {
		t.Fatalf("pendaftaran yang ditolak tidak boleh membuat user")
	}

	good := map[string]string{"email": email, "username": user, "password": "kuda-laut-biru-77"}
	if rec := doAuth(Register, "POST", "/x", good, ip); rec.Code != 201 {
		t.Fatalf("pendaftaran sah harus 201: %d %s", rec.Code, rec.Body.String())
	}
	var hash string
	_ = config.DB.QueryRow("SELECT password_hash FROM users WHERE email=?", email).Scan(&hash)
	if !strings.HasPrefix(hash, "$2a$12$") {
		t.Errorf("hash akun baru harus bcrypt cost 12: %s", hash[:min(len(hash), 30)])
	}
	if rec := doAuth(Register, "POST", "/x", good, ip); rec.Code != 400 {
		t.Errorf("email ganda harus ditolak")
	}
}

func TestIntegrationLoginUpgradesLegacyHashAndSetsCookies(t *testing.T) {
	authSetup(t)
	ip := testIP()
	id, email := makeCredUser(t, "kuda-laut-biru-77", true)

	rec := loginOK(t, email, "kuda-laut-biru-77", ip)
	if jsonField(rec, "token") == "" || jsonField(rec, "csrf_token") == "" {
		t.Errorf("respons login harus memuat token dan csrf_token")
	}
	for _, name := range []string{"access_token", "refresh_token"} {
		c := cookieNamed(rec, name)
		if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Secure {
			t.Errorf("cookie %s salah (HttpOnly/Lax/non-Secure di HTTP): %+v", name, c)
		}
	}
	var hash string
	_ = config.DB.QueryRow("SELECT password_hash FROM users WHERE id=?", id).Scan(&hash)
	if !strings.HasPrefix(hash, "$2a$12$") {
		t.Errorf("hash lama harus di-upgrade ke bcrypt saat login, dapat %q", hash[:min(len(hash), 24)])
	}
	loginOK(t, email, "kuda-laut-biru-77", ip) // sandi sama masih berlaku setelah upgrade

	// Sandi salah dan email tidak ada: respons identik (tidak membedakan).
	wrong := doAuth(Login, "POST", "/x", map[string]string{"email": email, "password": "salah-banget-1"}, ip)
	ghost := doAuth(Login, "POST", "/x", map[string]string{"email": "tidak-ada@example.test", "password": "salah-banget-1"}, ip)
	if wrong.Code != 401 || ghost.Code != 401 || wrong.Body.String() != ghost.Body.String() {
		t.Errorf("sandi salah dan email tak ada harus identik: %d %q vs %d %q", wrong.Code, wrong.Body.String(), ghost.Code, ghost.Body.String())
	}
	if rec := doAuth(Login, "POST", "/x", map[string]string{"email": "", "password": ""}, ip); rec.Code != 400 {
		t.Errorf("input kosong harus 400")
	}
}

func TestIntegrationLoginCookieSecureBehindTrustedProxy(t *testing.T) {
	authSetup(t)
	_, email := makeCredUser(t, "kuda-laut-biru-77", true)

	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"email":"`+email+`","password":"kuda-laut-biru-77"}`))
	req.RemoteAddr = testIP() + ":1"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	Login(rec, req)
	if c := cookieNamed(rec, "refresh_token"); c == nil || c.Secure {
		t.Errorf("tanpa TRUST_PROXY, X-Forwarded-Proto tidak boleh dipercaya (cookie non-Secure): %+v", c)
	}

	t.Setenv("TRUST_PROXY", "true")
	req2 := httptest.NewRequest("POST", "/x", strings.NewReader(`{"email":"`+email+`","password":"kuda-laut-biru-77"}`))
	req2.RemoteAddr = "10.0.0.1:1"
	req2.Header.Set("X-Forwarded-Proto", "https")
	req2.Header.Set("X-Forwarded-For", testIP())
	rec2 := httptest.NewRecorder()
	Login(rec2, req2)
	if c := cookieNamed(rec2, "refresh_token"); c == nil || !c.Secure {
		t.Errorf("dengan TRUST_PROXY + https, cookie harus Secure: %+v", c)
	}
}

func TestIntegrationDisabledAccountDoesNotLeakStatus(t *testing.T) {
	authSetup(t)
	ip := testIP()
	_, email := makeCredUser(t, "kuda-laut-biru-77", false)

	wrong := doAuth(Login, "POST", "/x", map[string]string{"email": email, "password": "salah-banget-1"}, ip)
	if wrong.Code != 401 || !strings.Contains(wrong.Body.String(), "Email atau password salah") {
		t.Errorf("akun nonaktif + sandi salah harus tampak seperti sandi salah biasa: %d %s", wrong.Code, wrong.Body.String())
	}
	right := doAuth(Login, "POST", "/x", map[string]string{"email": email, "password": "kuda-laut-biru-77"}, ip)
	if right.Code != 403 || cookieNamed(right, "refresh_token") != nil {
		t.Errorf("sandi benar pada akun nonaktif: 403 tanpa cookie, dapat %d", right.Code)
	}
}

func TestIntegrationLoginThrottlePerAccountAndIP(t *testing.T) {
	authSetup(t)
	pw := "kuda-laut-biru-77"
	_, email := makeCredUser(t, pw, true)

	// Per akun: 10 kegagalan (dari IP berbeda-beda) mengunci akun, bahkan untuk sandi benar.
	for i := 0; i < accountLockThreshold; i++ {
		rec := doAuth(Login, "POST", "/x", map[string]string{"email": email, "password": "salah-banget-1"}, testIP())
		if rec.Code != 401 {
			t.Fatalf("kegagalan ke-%d harus 401, dapat %d", i+1, rec.Code)
		}
	}
	rec := doAuth(Login, "POST", "/x", map[string]string{"email": email, "password": pw}, testIP())
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" || cookieNamed(rec, "refresh_token") != nil {
		t.Fatalf("akun terkunci harus 429 + Retry-After walau sandi benar: %d %s", rec.Code, rec.Body.String())
	}
	if secs := loginLockRemaining(strings.ToLower(email), "203.0.113.250"); secs < 1 || secs > 60 {
		t.Errorf("kunci pertama sekitar 1 menit, dapat %d detik", secs)
	}
	// Email dengan huruf besar tetap terkunci (kunci akun memakai huruf kecil).
	if rec := doAuth(Login, "POST", "/x", map[string]string{"email": strings.ToUpper(email), "password": pw}, testIP()); rec.Code != 429 {
		t.Errorf("variasi huruf besar harus tetap terkunci, dapat %d", rec.Code)
	}

	// Kunci bertahap: setelah kunci lewat, kegagalan berikutnya menggandakan jeda.
	_, _ = config.DB.Exec("UPDATE login_attempts SET locked_until = DATE_SUB(NOW(), INTERVAL 1 SECOND) WHERE scope='account'")
	doAuth(Login, "POST", "/x", map[string]string{"email": email, "password": "salah-banget-1"}, testIP())
	if secs := loginLockRemaining(strings.ToLower(email), "203.0.113.250"); secs < 61 || secs > 120 {
		t.Errorf("kunci kedua harus sekitar 2 menit, dapat %d detik", secs)
	}

	// Pengaturan ulang: setelah kunci berakhir dan login sukses, hitungan akun dihapus.
	_, _ = config.DB.Exec("UPDATE login_attempts SET locked_until = NULL WHERE scope='account'")
	loginOK(t, email, pw, testIP())
	var n int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM login_attempts WHERE scope='account' AND identifier=?", strings.ToLower(email)).Scan(&n)
	if n != 0 {
		t.Errorf("login sukses harus menghapus hitungan kegagalan akun")
	}

	// Per IP: banyak akun berbeda dari satu IP memicu kunci IP (tiap akun < ambang akun).
	_, _ = config.DB.Exec("DELETE FROM login_attempts")
	ip := "203.0.113.77"
	var emails []string
	for i := 0; i < 4; i++ {
		_, e := makeCredUser(t, pw, true)
		emails = append(emails, e)
	}
	count := 0
	for round := 0; round < 8 && count < ipLockThreshold; round++ {
		for _, e := range emails {
			if count >= ipLockThreshold {
				break
			}
			if rec := doAuth(Login, "POST", "/x", map[string]string{"email": e, "password": "salah-banget-1"}, ip); rec.Code != 401 {
				t.Fatalf("kegagalan IP ke-%d harus 401, dapat %d", count+1, rec.Code)
			}
			count++
		}
	}
	rec = doAuth(Login, "POST", "/x", map[string]string{"email": emails[0], "password": pw}, ip)
	if rec.Code != 429 {
		t.Errorf("IP terkunci harus 429 untuk akun mana pun, dapat %d", rec.Code)
	}
	if rec := doAuth(Login, "POST", "/x", map[string]string{"email": emails[0], "password": pw}, testIP()); rec.Code != 200 {
		t.Errorf("IP lain tidak boleh ikut terkunci, dapat %d", rec.Code)
	}
}

func TestIntegrationThrottleAppliesToUnknownEmailToo(t *testing.T) {
	authSetup(t)
	ghost := fmt.Sprintf("hantu%d@example.test", time.Now().UnixNano())
	for i := 0; i < accountLockThreshold; i++ {
		if rec := doAuth(Login, "POST", "/x", map[string]string{"email": ghost, "password": "salah-banget-1"}, testIP()); rec.Code != 401 {
			t.Fatalf("percobaan %d harus 401, dapat %d", i+1, rec.Code)
		}
	}
	rec := doAuth(Login, "POST", "/x", map[string]string{"email": ghost, "password": "salah-banget-1"}, testIP())
	if rec.Code != 429 {
		t.Errorf("email yang tidak terdaftar harus terkunci sama seperti akun nyata (agar tak membocorkan keberadaan), dapat %d", rec.Code)
	}
}

func TestIntegrationRefreshRotationGraceAndReuseDetection(t *testing.T) {
	authSetup(t)
	ip := testIP()
	pw := "kuda-laut-biru-77"
	id, email := makeCredUser(t, pw, true)

	r1 := loginOK(t, email, pw, ip)
	t1 := cookieNamed(r1, "refresh_token")

	// Rotasi normal.
	rot := doAuth(Refresh, "POST", "/x", nil, ip, t1)
	if rot.Code != 200 || jsonField(rot, "token") == "" {
		t.Fatalf("refresh harus 200: %d %s", rot.Code, rot.Body.String())
	}
	t2 := cookieNamed(rot, "refresh_token")
	if t2 == nil || t2.Value == t1.Value {
		t.Fatalf("refresh harus menerbitkan refresh token baru")
	}
	if rv, ra := tokenState(t, t1.Value); !rv || !ra {
		t.Errorf("token lama harus dicabut oleh rotasi dengan revoked_at terisi (revoked=%v revoked_at=%v)", rv, ra)
	}

	// Dua tab bersamaan: token lama dipakai lagi dalam 10 detik = wajar.
	grace := doAuth(Refresh, "POST", "/x", nil, ip, t1)
	if grace.Code != 200 || jsonField(grace, "token") == "" {
		t.Errorf("pemakaian ulang dalam jendela toleransi harus 200: %d %s", grace.Code, grace.Body.String())
	}
	if len(grace.Result().Cookies()) != 0 {
		t.Errorf("respons toleransi tidak boleh mengganti cookie/refresh token")
	}
	var live int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM refresh_tokens WHERE user_id=? AND revoked=0", id).Scan(&live)
	if live != 1 {
		t.Errorf("toleransi tidak boleh menerbitkan refresh token tambahan, aktif=%d", live)
	}

	// Pemakaian ulang setelah jendela lewat = pencurian: semua sesi dicabut.
	_, _ = config.DB.Exec("UPDATE refresh_tokens SET revoked_at = DATE_SUB(NOW(), INTERVAL 30 SECOND) WHERE token=?", t1.Value)
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)
	replay := doAuth(Refresh, "POST", "/x", nil, ip, t1)
	if replay.Code != 401 {
		t.Fatalf("pemakaian ulang harus 401, dapat %d", replay.Code)
	}
	if rv, _ := tokenState(t, t2.Value); !rv {
		t.Errorf("deteksi pemakaian ulang harus mencabut SEMUA sesi, termasuk token baru")
	}
	if doAuth(Refresh, "POST", "/x", nil, ip, t2).Code != 401 {
		t.Errorf("token baru harus ikut tidak berlaku")
	}
	if !strings.Contains(logBuf.String(), "dipakai ulang") {
		t.Errorf("kejadian pemakaian ulang harus dicatat di log")
	}
	if c := cookieNamed(replay, "refresh_token"); c == nil || c.MaxAge >= 0 {
		t.Errorf("cookie harus dihapus pada pemakaian ulang")
	}
}

func TestIntegrationExplicitRevocationDoesNotTriggerReuseDetection(t *testing.T) {
	authSetup(t)
	ip := testIP()
	pw := "kuda-laut-biru-77"
	_, email := makeCredUser(t, pw, true)

	a := cookieNamed(loginOK(t, email, pw, ip), "refresh_token")
	b := cookieNamed(loginOK(t, email, pw, ip), "refresh_token")

	// Keluar di perangkat A, lalu A mencoba refresh dengan token lamanya.
	doAuth(Logout, "POST", "/x", nil, ip, a)
	if doAuth(Refresh, "POST", "/x", nil, ip, a).Code != 401 {
		t.Errorf("token yang sudah logout harus ditolak")
	}
	if doAuth(Refresh, "POST", "/x", nil, ip, b).Code != 200 {
		t.Errorf("sesi perangkat lain tidak boleh ikut terdampak pencabutan eksplisit")
	}
}

func TestIntegrationRefreshRejectsDeactivatedAccount(t *testing.T) {
	authSetup(t)
	ip := testIP()
	pw := "kuda-laut-biru-77"
	id, email := makeCredUser(t, pw, true)
	tok := cookieNamed(loginOK(t, email, pw, ip), "refresh_token")
	other := cookieNamed(loginOK(t, email, pw, ip), "refresh_token")

	_, _ = config.DB.Exec("UPDATE users SET is_active=0 WHERE id=?", id)
	if rec := doAuth(Refresh, "POST", "/x", nil, ip, tok); rec.Code != 401 {
		t.Errorf("akun nonaktif tidak boleh me-refresh: %d", rec.Code)
	}
	if rv, _ := tokenState(t, other.Value); !rv {
		t.Errorf("semua sesi akun nonaktif harus dicabut")
	}
}

func resetCapture(t *testing.T) chan [2]string {
	t.Helper()
	ch := make(chan [2]string, 10)
	orig := sendResetMail
	sendResetMail = func(to, tok string) { ch <- [2]string{to, tok} }
	t.Cleanup(func() { sendResetMail = orig })
	return ch
}

func waitMail(t *testing.T, ch chan [2]string) ([2]string, bool) {
	t.Helper()
	select {
	case m := <-ch:
		return m, true
	case <-time.After(1500 * time.Millisecond):
		return [2]string{}, false
	}
}

func TestIntegrationForgotAndResetPasswordFlow(t *testing.T) {
	authSetup(t)
	ip := testIP()
	oldPW, newPW := "kuda-laut-biru-77", "burung-hantu-ungu-42"
	id, email := makeCredUser(t, oldPW, true)
	session := cookieNamed(loginOK(t, email, oldPW, ip), "refresh_token")
	mails := resetCapture(t)

	// Respons identik untuk email terdaftar dan tidak, tanpa token.
	rec := doAuth(ForgotPassword, "POST", "/x", map[string]string{"email": email}, ip)
	ghost := doAuth(ForgotPassword, "POST", "/x", map[string]string{"email": "tidak-ada@example.test"}, ip)
	if rec.Code != 200 || ghost.Code != 200 || rec.Body.String() != ghost.Body.String() {
		t.Fatalf("respons forgot harus identik: %q vs %q", rec.Body.String(), ghost.Body.String())
	}
	if strings.Contains(rec.Body.String(), "token") || jsonField(rec, "reset_token") != "" {
		t.Fatalf("token reset TIDAK boleh ada di respons: %s", rec.Body.String())
	}
	m, ok := waitMail(t, mails)
	if !ok || m[0] != email || len(m[1]) != 64 {
		t.Fatalf("email reset harus terkirim ke akun terdaftar dengan token 64 hex: %v %v", m, ok)
	}
	raw := m[1]
	if _, ok := waitMail(t, mails); ok {
		t.Errorf("email tak terdaftar tidak boleh memicu pengiriman")
	}

	// Yang tersimpan di DB hanya hash.
	var stored string
	_ = config.DB.QueryRow("SELECT reset_token FROM users WHERE id=?", id).Scan(&stored)
	if stored == raw || stored != hashToken(raw) || len(stored) != 64 {
		t.Errorf("DB harus menyimpan SHA-256 token, bukan token polos")
	}

	// Jeda 1 menit: permintaan kedua tidak mengirim email baru dan tidak mengganti token.
	doAuth(ForgotPassword, "POST", "/x", map[string]string{"email": email}, ip)
	if _, ok := waitMail(t, mails); ok {
		t.Errorf("permintaan kedua dalam semenit tidak boleh mengirim email lagi")
	}
	var stored2 string
	_ = config.DB.QueryRow("SELECT reset_token FROM users WHERE id=?", id).Scan(&stored2)
	if stored2 != stored {
		t.Errorf("token tidak boleh berganti selama jeda")
	}

	// Token salah / format salah ditolak.
	for _, bad := range []string{"", "abc", strings.Repeat("0", 64), strings.Repeat("g", 64), stored /* hash dipakai sebagai token */} {
		if rec := doAuth(ResetPassword, "POST", "/x", map[string]string{"token": bad, "new_password": newPW}, ip); rec.Code != 400 {
			t.Errorf("token %q harus ditolak, dapat %d", bad, rec.Code)
		}
	}
	// Sandi lemah ditolak, token TETAP berlaku.
	if rec := doAuth(ResetPassword, "POST", "/x", map[string]string{"token": raw, "new_password": "Password123!"}, ip); rec.Code != 400 {
		t.Errorf("sandi lemah harus ditolak: %d", rec.Code)
	}
	var still string
	_ = config.DB.QueryRow("SELECT COALESCE(reset_token,'') FROM users WHERE id=?", id).Scan(&still)
	if still != stored {
		t.Errorf("token harus tetap berlaku setelah sandi ditolak")
	}

	// Sandi baru diterima.
	if rec := doAuth(ResetPassword, "POST", "/x", map[string]string{"token": raw, "new_password": newPW}, ip); rec.Code != 200 {
		t.Fatalf("reset harus 200: %d %s", rec.Code, rec.Body.String())
	}
	var rt, re int
	_ = config.DB.QueryRow("SELECT reset_token IS NULL, reset_expires IS NULL FROM users WHERE id=?", id).Scan(&rt, &re)
	if rt != 1 || re != 1 {
		t.Errorf("reset_token dan reset_expires harus dikosongkan")
	}
	// Sekali pakai.
	if rec := doAuth(ResetPassword, "POST", "/x", map[string]string{"token": raw, "new_password": "burung-hantu-lain-99"}, ip); rec.Code != 400 {
		t.Errorf("token tidak boleh dipakai dua kali: %d", rec.Code)
	}
	// Sesi lama berakhir; sandi lama gagal; sandi baru berhasil.
	if rv, _ := tokenState(t, session.Value); !rv {
		t.Errorf("semua sesi lama harus dicabut setelah reset")
	}
	if rec := doAuth(Login, "POST", "/x", map[string]string{"email": email, "password": oldPW}, ip); rec.Code != 401 {
		t.Errorf("sandi lama tidak boleh berlaku lagi")
	}
	loginOK(t, email, newPW, ip)
}

func TestIntegrationResetTokenExpiryAndInactiveAccounts(t *testing.T) {
	authSetup(t)
	ip := testIP()
	mails := resetCapture(t)
	id, email := makeCredUser(t, "kuda-laut-biru-77", true)

	doAuth(ForgotPassword, "POST", "/x", map[string]string{"email": email}, ip)
	m, ok := waitMail(t, mails)
	if !ok {
		t.Fatal("email reset tidak terkirim")
	}
	_, _ = config.DB.Exec("UPDATE users SET reset_expires = DATE_SUB(NOW(), INTERVAL 1 MINUTE) WHERE id=?", id)
	if rec := doAuth(ResetPassword, "POST", "/x", map[string]string{"token": m[1], "new_password": "burung-hantu-ungu-42"}, ip); rec.Code != 400 {
		t.Errorf("token kedaluwarsa harus ditolak: %d", rec.Code)
	}
	// Kedaluwarsa juga membuka jalan untuk permintaan baru (tanpa harus menunggu jeda).
	doAuth(ForgotPassword, "POST", "/x", map[string]string{"email": email}, ip)
	if _, ok := waitMail(t, mails); !ok {
		t.Errorf("setelah kedaluwarsa, permintaan baru harus bisa")
	}

	// Akun nonaktif tidak mendapat email reset; respons tetap sama.
	_, inactiveEmail := makeCredUser(t, "kuda-laut-biru-77", false)
	rec := doAuth(ForgotPassword, "POST", "/x", map[string]string{"email": inactiveEmail}, ip)
	if rec.Code != 200 {
		t.Errorf("respons akun nonaktif harus sama (200): %d", rec.Code)
	}
	if _, ok := waitMail(t, mails); ok {
		t.Errorf("akun nonaktif tidak boleh mendapat email reset")
	}
}

func TestIntegrationChangePasswordRulesAndRevocation(t *testing.T) {
	authSetup(t)
	ip := testIP()
	oldPW, newPW := "kuda-laut-biru-77", "burung-hantu-ungu-42"
	id, email := makeCredUser(t, oldPW, true)
	tok := cookieNamed(loginOK(t, email, oldPW, ip), "refresh_token")

	call := func(body map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/x", strings.NewReader(mustJSON(body)))
		req.RemoteAddr = ip + ":1"
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, id))
		rec := httptest.NewRecorder()
		ChangePassword(rec, req)
		return rec
	}
	if rec := call(map[string]string{"old_password": "salah-banget-1", "new_password": newPW}); rec.Code != 400 {
		t.Errorf("sandi lama salah harus 400 (bukan 403, supaya tidak dikira masalah CSRF): %d", rec.Code)
	}
	if rec := call(map[string]string{"old_password": oldPW, "new_password": "Password123!"}); rec.Code != 400 {
		t.Errorf("sandi baru lemah harus ditolak: %d", rec.Code)
	}
	if rv, _ := tokenState(t, tok.Value); rv {
		t.Errorf("percobaan gagal tidak boleh mencabut sesi")
	}
	rec := call(map[string]string{"old_password": oldPW, "new_password": newPW})
	if rec.Code != 200 {
		t.Fatalf("ganti sandi harus 200: %d %s", rec.Code, rec.Body.String())
	}
	if rv, _ := tokenState(t, tok.Value); !rv {
		t.Errorf("setelah ganti sandi semua sesi harus dicabut")
	}
	loginOK(t, email, newPW, ip)
}

func mustJSON(v interface{}) string { b, _ := json.Marshal(v); return string(b) }

func TestIntegrationSessionListAndRevoke(t *testing.T) {
	authSetup(t)
	ip := testIP()
	pw := "kuda-laut-biru-77"
	id, email := makeCredUser(t, pw, true)
	otherID, otherEmail := makeCredUser(t, pw, true)

	a := cookieNamed(loginOK(t, email, pw, ip), "refresh_token")
	b := cookieNamed(loginOK(t, email, pw, ip), "refresh_token")
	oc := cookieNamed(loginOK(t, otherEmail, pw, ip), "refresh_token")

	authed := func(h http.HandlerFunc, uid int64, method string, pv map[string]string, ck *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/x", nil)
		req.RemoteAddr = ip + ":1"
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
		for k, v := range pv {
			req.SetPathValue(k, v)
		}
		if ck != nil {
			req.AddCookie(ck)
		}
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}

	rec := authed(ListSessions, id, "GET", nil, a)
	var list []sessionInfo
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Fatalf("harus ada 2 sesi, dapat %d: %s", len(list), rec.Body.String())
	}
	cur := 0
	for _, s := range list {
		if s.Current {
			cur++
		}
	}
	if cur != 1 {
		t.Errorf("tepat satu sesi harus bertanda current, dapat %d", cur)
	}
	if strings.Contains(rec.Body.String(), a.Value) || strings.Contains(rec.Body.String(), b.Value) {
		t.Errorf("nilai token tidak boleh muncul di daftar sesi")
	}

	// Tidak bisa mencabut sesi milik pengguna lain.
	var otherSession int64
	_ = config.DB.QueryRow("SELECT id FROM refresh_tokens WHERE token=?", oc.Value).Scan(&otherSession)
	if rec := authed(RevokeSession, id, "DELETE", map[string]string{"id": fmt.Sprint(otherSession)}, nil); rec.Code != 404 {
		t.Errorf("mencabut sesi orang lain harus 404, dapat %d", rec.Code)
	}
	if rv, _ := tokenState(t, oc.Value); rv {
		t.Errorf("sesi orang lain tidak boleh tercabut")
	}
	_ = otherID

	// Cabut sesi b: b tidak bisa refresh; a tetap bisa.
	var bID int64
	_ = config.DB.QueryRow("SELECT id FROM refresh_tokens WHERE token=?", b.Value).Scan(&bID)
	if rec := authed(RevokeSession, id, "DELETE", map[string]string{"id": fmt.Sprint(bID)}, a); rec.Code != 200 {
		t.Fatalf("cabut sesi sendiri harus 200: %d", rec.Code)
	}
	if doAuth(Refresh, "POST", "/x", nil, ip, b).Code != 401 {
		t.Errorf("sesi yang dicabut tidak boleh bisa refresh")
	}
	if rv, _ := tokenState(t, a.Value); rv {
		t.Errorf("sesi lain tidak boleh ikut tercabut")
	}

	// Keluar dari semua perangkat.
	rec = authed(LogoutAll, id, "POST", nil, a)
	if rec.Code != 200 {
		t.Fatalf("logout-all harus 200: %d", rec.Code)
	}
	if rv, _ := tokenState(t, a.Value); !rv {
		t.Errorf("logout-all harus mencabut semua sesi")
	}
	if c := cookieNamed(rec, "refresh_token"); c == nil || c.MaxAge >= 0 {
		t.Errorf("logout-all harus menghapus cookie")
	}
	if rv, _ := tokenState(t, oc.Value); rv {
		t.Errorf("logout-all tidak boleh menyentuh pengguna lain")
	}
}

func TestIntegrationSeedAdminNeverLogsOrDefaultsPassword(t *testing.T) {
	integrationDB(t)
	var admins int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM users WHERE role='admin'").Scan(&admins)
	if admins > 0 {
		t.Skip("sudah ada admin di DB uji; tes seed dilewati")
	}
	email := fmt.Sprintf("seed%d@example.test", time.Now().UnixNano())
	t.Setenv("ADMIN_EMAIL", email)
	t.Setenv("ADMIN_USERNAME", "seedadmin")
	t.Cleanup(func() { _, _ = config.DB.Exec("DELETE FROM users WHERE email=?", email) })

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	count := func() int {
		var n int
		_ = config.DB.QueryRow("SELECT COUNT(*) FROM users WHERE email=?", email).Scan(&n)
		return n
	}

	t.Setenv("ADMIN_PASSWORD", "")
	SeedAdmin()
	if count() != 0 || !strings.Contains(buf.String(), "ADMIN_PASSWORD belum diatur") {
		t.Errorf("tanpa ADMIN_PASSWORD, admin tidak boleh dibuat dengan sandi bawaan")
	}
	t.Setenv("ADMIN_PASSWORD", "admin123")
	SeedAdmin()
	if count() != 0 {
		t.Errorf("ADMIN_PASSWORD lemah harus ditolak")
	}
	secret := "kuda-laut-admin-77x"
	t.Setenv("ADMIN_PASSWORD", secret)
	buf.Reset()
	SeedAdmin()
	if count() != 1 {
		t.Fatalf("admin dengan sandi kuat harus dibuat: %s", buf.String())
	}
	if strings.Contains(buf.String(), secret) {
		t.Errorf("sandi admin TIDAK boleh muncul di log")
	}
	var hash string
	_ = config.DB.QueryRow("SELECT password_hash FROM users WHERE email=?", email).Scan(&hash)
	if !strings.HasPrefix(hash, "$2a$") {
		t.Errorf("hash admin harus bcrypt")
	}
}

// ---------- Email (tanpa DB) ----------

// fakeSMTP: server SMTP minimal yang merekam pesan (tanpa TLS/AUTH).
func fakeSMTP(t *testing.T) (addr string, got chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	got = make(chan string, 1)
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		w := func(s string) { fmt.Fprint(conn, s+"\r\n") }
		w("220 fake ESMTP")
		buf := make([]byte, 4096)
		var data strings.Builder
		inData := false
		var pending string
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			pending += string(buf[:n])
			for {
				i := strings.Index(pending, "\r\n")
				if i < 0 {
					break
				}
				line := pending[:i]
				pending = pending[i+2:]
				if inData {
					if line == "." {
						inData = false
						got <- data.String()
						w("250 OK queued")
					} else {
						data.WriteString(line + "\n")
					}
					continue
				}
				up := strings.ToUpper(line)
				switch {
				case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
					w("250 fake")
				case strings.HasPrefix(up, "MAIL FROM"), strings.HasPrefix(up, "RCPT TO"):
					data.WriteString(line + "\n")
					w("250 OK")
				case up == "DATA":
					inData = true
					w("354 go ahead")
				case up == "QUIT":
					w("221 bye")
					return
				default:
					w("250 OK")
				}
			}
		}
	}()
	return ln.Addr().String(), got
}

func TestMailSendsResetLinkWithFragmentToken(t *testing.T) {
	addr, got := fakeSMTP(t)
	host, port, _ := net.SplitHostPort(addr)
	t.Setenv("SMTP_HOST", host)
	t.Setenv("SMTP_PORT", port)
	t.Setenv("SMTP_USER", "")
	t.Setenv("SMTP_FROM", "Notebook <noreply@example.test>")
	t.Setenv("APP_BASE_URL", "https://catatan.example.test/")

	token := strings.Repeat("ab", 32)
	defaultSendResetMail("budi@example.test", token)

	select {
	case msg := <-got:
		if !strings.Contains(msg, "MAIL FROM:<noreply@example.test>") {
			t.Errorf("amplop MAIL FROM harus alamat polos: %s", msg)
		}
		if !strings.Contains(msg, "RCPT TO:<budi@example.test>") {
			t.Errorf("RCPT TO salah: %s", msg)
		}
		if !strings.Contains(msg, "From: Notebook <noreply@example.test>") || !strings.Contains(msg, "To: budi@example.test") {
			t.Errorf("header From/To salah: %s", msg)
		}
		// Badan base64 -> decode dan cari tautan.
		_, body, _ := strings.Cut(msg, "\n\n")
		var enc strings.Builder
		for _, l := range strings.Split(body, "\n") {
			if strings.HasPrefix(l, "MAIL") || strings.HasPrefix(l, "RCPT") {
				continue
			}
			enc.WriteString(strings.TrimSpace(l))
		}
		dec, err := base64.StdEncoding.DecodeString(enc.String())
		if err != nil {
			t.Fatalf("badan bukan base64 sah: %v", err)
		}
		want := "https://catatan.example.test/#reset=" + token
		if !strings.Contains(string(dec), want) {
			t.Errorf("tautan reset (token di fragmen #) tidak ada: %s", dec)
		}
		if strings.Contains(string(dec), "?reset=") || strings.Contains(string(dec), "?token=") {
			t.Errorf("token tidak boleh di query string")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("email tidak terkirim ke SMTP palsu")
	}
}

func TestMailRejectsHeaderInjectionAndHandlesDevMode(t *testing.T) {
	addr, got := fakeSMTP(t)
	host, port, _ := net.SplitHostPort(addr)
	t.Setenv("SMTP_HOST", host)
	t.Setenv("SMTP_PORT", port)
	t.Setenv("SMTP_FROM", "noreply@example.test")

	for _, to := range []string{"a@b.test\r\nBcc: korban@example.test", "a@b.test\nBcc: x@y.test", ""} {
		if err := sendMail(to, "Subjek", "isi"); err == nil {
			t.Errorf("alamat %q seharusnya ditolak", to)
		}
	}
	if err := sendMail("a@b.test", "Subjek\r\nBcc: x@y.test", "isi"); err == nil {
		t.Errorf("subjek dengan CRLF seharusnya ditolak")
	}
	select {
	case m := <-got:
		t.Errorf("tidak boleh ada pesan yang sampai ke SMTP: %s", m)
	case <-time.After(300 * time.Millisecond):
	}

	// Mode pengembangan: tanpa SMTP_HOST, tautan hanya dicatat di log.
	t.Setenv("SMTP_HOST", "")
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	defaultSendResetMail("dev@example.test", strings.Repeat("cd", 32))
	if !strings.Contains(buf.String(), "mode pengembangan") || !strings.Contains(buf.String(), "#reset="+strings.Repeat("cd", 32)) {
		t.Errorf("mode pengembangan harus mencatat tautan di log: %s", buf.String())
	}
}
