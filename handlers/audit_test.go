package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"notebook-app/config"
	"notebook-app/middleware"
)

func countLogs(t *testing.T, uid int64, action string) int {
	t.Helper()
	var n int
	if err := config.DB.QueryRow("SELECT COUNT(*) FROM activity_log WHERE user_id=? AND action=?", uid, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func lastLog(t *testing.T, uid int64, action string) (itemType string, itemID int64, desc, ip, ua string) {
	t.Helper()
	var it, d, i, u *string
	var id *int64
	err := config.DB.QueryRow(`SELECT item_type, item_id, description, ip, user_agent FROM activity_log
		WHERE user_id=? AND action=? ORDER BY id DESC LIMIT 1`, uid, action).Scan(&it, &id, &d, &i, &u)
	if err != nil {
		t.Fatalf("log %q tidak ada: %v", action, err)
	}
	s := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	var n int64
	if id != nil {
		n = *id
	}
	return s(it), n, s(d), s(i), s(u)
}

// callWrapped memanggil handler (yang sudah dibungkus) sebagai pengguna tertentu.
func callWrapped(h http.HandlerFunc, uid int64, method, target string, body interface{}, pv map[string]string) *httptest.ResponseRecorder {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "UjiAudit/1.0")
	req.RemoteAddr = "198.51.100.9:4000"
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
	for k, v := range pv {
		req.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestIntegrationAuditNoteLifecycle(t *testing.T) {
	integrationDB(t)
	uid := makeTestUser(t)

	create := Audit("note_create", "note", CreateNote)
	update := AuditEdit("note_update", "note", UpdateNote)
	trash := Audit("note_trash", "note", TrashNote)
	restore := Audit("note_restore", "note", RestoreNote)
	del := Audit("note_delete", "note", DeleteNotePermanently)

	rec := callWrapped(create, uid, "POST", "/x", map[string]interface{}{"title": "A", "content": "<p>x</p>"}, nil)
	nid := idFrom(t, rec)
	it, id, desc, ip, ua := lastLog(t, uid, "note_create")
	if it != "note" || id != nid || desc != "Membuat catatan" || ip != "198.51.100.9" || ua != "UjiAudit/1.0" {
		t.Errorf("log note_create salah: %q %d %q %q %q", it, id, desc, ip, ua)
	}

	// Pengeditan beruntun (simpan otomatis) dicatat sekali per 5 menit.
	pv := map[string]string{"id": fmt.Sprint(nid)}
	for i := 0; i < 5; i++ {
		if rec := callWrapped(update, uid, "PUT", "/x", map[string]interface{}{"content": fmt.Sprintf("<p>rev %d</p>", i)}, pv); rec.Code != 200 {
			t.Fatalf("update: %d", rec.Code)
		}
	}
	if n := countLogs(t, uid, "note_update"); n != 1 {
		t.Errorf("lima edit beruntun harus tercatat 1 kali, dapat %d", n)
	}
	_, _ = config.DB.Exec("UPDATE activity_log SET created_at = DATE_SUB(NOW(), INTERVAL 6 MINUTE) WHERE user_id=? AND action='note_update'", uid)
	callWrapped(update, uid, "PUT", "/x", map[string]interface{}{"content": "<p>setelah 6 menit</p>"}, pv)
	if n := countLogs(t, uid, "note_update"); n != 2 {
		t.Errorf("setelah 6 menit harus tercatat lagi, dapat %d", n)
	}

	callWrapped(trash, uid, "POST", "/x", nil, pv)
	callWrapped(restore, uid, "POST", "/x", nil, pv)
	callWrapped(trash, uid, "POST", "/x", nil, pv)
	callWrapped(del, uid, "DELETE", "/x", nil, pv)
	for action, want := range map[string]int{"note_trash": 2, "note_restore": 1, "note_delete": 1} {
		if got := countLogs(t, uid, action); got != want {
			t.Errorf("%s: dapat %d, mau %d", action, got, want)
		}
	}
	if _, id, d, _, _ := lastLog(t, uid, "note_delete"); id != nid || d != "Menghapus catatan permanen" {
		t.Errorf("log note_delete salah: %d %q", id, d)
	}
}

func TestIntegrationAuditIsNotForgeableByNonOwners(t *testing.T) {
	integrationDB(t)
	owner, intruder := makeTestUser(t), makeTestUser(t)
	nid := idFrom(t, callAs(owner, CreateNote, "POST", "/x", map[string]interface{}{"title": "P", "content": "<p>x</p>"}, nil))
	npv := map[string]string{"id": fmt.Sprint(nid)}
	sid := idFrom(t, callWrapped(CreateShare, owner, "POST", "/x",
		map[string]interface{}{"item_type": "note", "item_id": nid, "share_mode": "public", "permission": "viewer"}, nil))
	spv := map[string]string{"id": fmt.Sprint(sid)}

	attempts := []struct {
		action, itemType string
		h                http.HandlerFunc
		method           string
		pv               map[string]string
		body             interface{}
	}{
		{"note_trash", "note", TrashNote, "POST", npv, nil},
		{"note_restore", "note", RestoreNote, "POST", npv, nil},
		{"note_delete", "note", DeleteNotePermanently, "DELETE", npv, nil},
		{"note_lock", "note", LockNote, "POST", npv, map[string]string{"password": "rahasia123"}},
		{"share_update", "share", UpdateSharePermission, "PUT", spv, map[string]string{"permission": "editor"}},
		{"share_revoke", "share", RevokeShare, "DELETE", spv, nil},
	}
	for _, a := range attempts {
		rec := callWrapped(Audit(a.action, a.itemType, a.h), intruder, a.method, "/x", a.body, a.pv)
		if rec.Code != 404 {
			t.Errorf("%s oleh non-pemilik harus 404, dapat %d", a.action, rec.Code)
		}
		if n := countLogs(t, intruder, a.action); n != 0 {
			t.Errorf("%s: aksi yang ditolak tidak boleh meninggalkan log (log palsu), dapat %d", a.action, n)
		}
	}

	// Data milik pemilik tidak boleh tersentuh sama sekali.
	var trashed, locked int
	_ = config.DB.QueryRow("SELECT is_trashed, is_locked FROM notes WHERE id=?", nid).Scan(&trashed, &locked)
	if trashed != 0 || locked != 0 {
		t.Errorf("catatan pemilik berubah oleh non-pemilik: trashed=%d locked=%d", trashed, locked)
	}
	var perm string
	var shares int
	_ = config.DB.QueryRow("SELECT permission FROM shares WHERE id=?", sid).Scan(&perm)
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM shares WHERE id=?", sid).Scan(&shares)
	if shares != 1 || perm != "viewer" {
		t.Errorf("share pemilik berubah/terhapus oleh non-pemilik: ada=%d izin=%q", shares, perm)
	}

	// Pemilik sendiri tetap bisa, dan itu tercatat.
	if rec := callWrapped(Audit("note_trash", "note", TrashNote), owner, "POST", "/x", nil, npv); rec.Code != 200 {
		t.Errorf("pemilik harus bisa memindahkan ke sampah: %d", rec.Code)
	}
	if countLogs(t, owner, "note_trash") != 1 {
		t.Errorf("aksi pemilik harus tercatat")
	}
	// Catatan yang tidak ada: 404 dan tidak tercatat.
	callWrapped(Audit("note_delete", "note", DeleteNotePermanently), owner, "DELETE", "/x", nil, map[string]string{"id": "99999999"})
	if countLogs(t, owner, "note_delete") != 0 {
		t.Errorf("hapus catatan yang tidak ada tidak boleh dicatat")
	}
	// Tanpa pengguna (id 0): tidak mencatat apa pun dan tidak panik.
	callWrapped(Audit("note_create", "note", CreateNote), 0, "POST", "/x", map[string]interface{}{"title": "x"}, nil)
}

func TestIntegrationAuditAttachmentsAndShares(t *testing.T) {
	integrationDB(t)
	t.Setenv("UPLOAD_DIR", t.TempDir())
	uid := makeTestUser(t)
	nid := idFrom(t, callAs(uid, CreateNote, "POST", "/x", map[string]interface{}{"title": "L", "content": "<p>x</p>"}, nil))

	// Unggah
	png := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), bytes.Repeat([]byte{0}, 64)...)
	body, ct := multipartUpload(t, "file", "gambar.png", "image/png", png)
	req := httptest.NewRequest("POST", "/x", body)
	req.Header.Set("Content-Type", ct)
	req.RemoteAddr = "198.51.100.9:4000"
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
	req.SetPathValue("note_id", fmt.Sprint(nid))
	rec := httptest.NewRecorder()
	Audit("attachment_upload", "note", UploadAttachment)(rec, req)
	if rec.Code != 200 && rec.Code != 201 {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	if it, id, _, _, _ := lastLog(t, uid, "attachment_upload"); it != "note" || id != nid {
		t.Errorf("log upload: %q %d", it, id)
	}

	var aid int64
	_ = config.DB.QueryRow("SELECT id FROM attachments WHERE note_id=? ORDER BY id DESC LIMIT 1", nid).Scan(&aid)
	apv := map[string]string{"id": fmt.Sprint(aid)}

	// Unduh
	dl := callWrapped(Audit("attachment_download", "attachment", DownloadAttachment), uid, "GET", "/x", nil, apv)
	if dl.Code != 200 {
		t.Fatalf("unduh: %d", dl.Code)
	}
	if it, id, d, _, _ := lastLog(t, uid, "attachment_download"); it != "attachment" || id != aid || d != "Mengunduh lampiran" {
		t.Errorf("log unduh: %q %d %q", it, id, d)
	}
	// Ganti nama dan hapus
	callWrapped(Audit("attachment_rename", "attachment", RenameAttachment), uid, "PUT", "/x", map[string]string{"filename": "baru.png"}, apv)
	callWrapped(Audit("attachment_delete", "attachment", DeleteAttachment), uid, "DELETE", "/x", nil, apv)
	for _, a := range []string{"attachment_rename", "attachment_delete"} {
		if countLogs(t, uid, a) != 1 {
			t.Errorf("%s harus tercatat 1 kali", a)
		}
	}

	// Berbagi: buat (id dari respons), ubah izin, cabut.
	rec = callWrapped(Audit("share_create", "share", CreateShare), uid, "POST", "/x",
		map[string]interface{}{"item_type": "note", "item_id": nid, "share_mode": "public", "permission": "viewer"}, nil)
	if rec.Code != 201 {
		t.Fatalf("buat share: %d %s", rec.Code, rec.Body.String())
	}
	sid := idFrom(t, rec)
	if it, id, _, _, _ := lastLog(t, uid, "share_create"); it != "share" || id != sid {
		t.Errorf("log share_create: %q %d (mau share %d)", it, id, sid)
	}
	spv := map[string]string{"id": fmt.Sprint(sid)}
	callWrapped(Audit("share_update", "share", UpdateSharePermission), uid, "PUT", "/x", map[string]string{"permission": "editor"}, spv)
	callWrapped(Audit("share_revoke", "share", RevokeShare), uid, "DELETE", "/x", nil, spv)
	for _, a := range []string{"share_update", "share_revoke"} {
		if countLogs(t, uid, a) != 1 {
			t.Errorf("%s harus tercatat 1 kali", a)
		}
	}
}

func TestIntegrationAuditAccountEvents(t *testing.T) {
	authSetup(t)
	ip := testIP()
	pw := "kuda-laut-biru-77"
	id, email := makeCredUser(t, pw, true)

	// Salah sandi: dicatat untuk akun yang ada, tidak untuk email yang tak dikenal.
	doAuth(Login, "POST", "/x", map[string]string{"email": email, "password": "salah-banget-1"}, ip)
	doAuth(Login, "POST", "/x", map[string]string{"email": "hantu@example.test", "password": "salah-banget-1"}, ip)
	if countLogs(t, id, "login_failed") != 1 {
		t.Errorf("login_failed harus tercatat 1 kali untuk akun yang ada")
	}
	var ghost int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM activity_log WHERE action='login_failed' AND user_id NOT IN (?)", id).Scan(&ghost)

	a := cookieNamed(loginOK(t, email, pw, ip), "refresh_token")
	if countLogs(t, id, "login") != 1 {
		t.Errorf("login harus tercatat")
	}
	if _, _, d, lip, _ := lastLog(t, id, "login"); d != "Login berhasil" || lip != ip {
		t.Errorf("log login: %q ip=%q (mau %q)", d, lip, ip)
	}
	b := cookieNamed(loginOK(t, email, pw, ip), "refresh_token")

	doAuth(Logout, "POST", "/x", nil, ip, a)
	if countLogs(t, id, "logout") != 1 {
		t.Errorf("logout harus tercatat")
	}

	callWrapped(LogoutAll, id, "POST", "/x", nil, nil)
	if countLogs(t, id, "logout_all") != 1 {
		t.Errorf("logout_all harus tercatat")
	}
	_ = b

	// Ganti sandi dan reset sandi.
	callAsUser := func(h http.HandlerFunc, body map[string]string) *httptest.ResponseRecorder {
		return callWrapped(h, id, "POST", "/x", body, nil)
	}
	if rec := callAsUser(ChangePassword, map[string]string{"old_password": pw, "new_password": "burung-hantu-ungu-42"}); rec.Code != 200 {
		t.Fatalf("ganti sandi: %d %s", rec.Code, rec.Body.String())
	}
	if countLogs(t, id, "password_change") != 1 {
		t.Errorf("password_change harus tercatat")
	}
	mails := resetCapture(t)
	doAuth(ForgotPassword, "POST", "/x", map[string]string{"email": email}, ip)
	m, ok := waitMail(t, mails)
	if !ok {
		t.Fatal("email reset tidak terkirim")
	}
	doAuth(ResetPassword, "POST", "/x", map[string]string{"token": m[1], "new_password": "elang-merah-biru-91"}, ip)
	if countLogs(t, id, "password_reset") != 1 {
		t.Errorf("password_reset harus tercatat")
	}

	// Registrasi.
	rn := fmt.Sprintf("auditreg%d@example.test", id)
	t.Cleanup(func() { _, _ = config.DB.Exec("DELETE FROM users WHERE email=?", rn) })
	doAuth(Register, "POST", "/x", map[string]string{"email": rn, "username": fmt.Sprintf("auditreg%d", id), "password": "kuda-laut-biru-77"}, ip)
	var rid int64
	_ = config.DB.QueryRow("SELECT id FROM users WHERE email=?", rn).Scan(&rid)
	if rid == 0 || countLogs(t, rid, "register") != 1 {
		t.Errorf("register harus tercatat")
	}

	// Tidak ada kata sandi atau token di deskripsi log mana pun.
	rows, _ := config.DB.Query("SELECT COALESCE(description,'') FROM activity_log WHERE user_id=?", id)
	defer rows.Close()
	for rows.Next() {
		var d string
		_ = rows.Scan(&d)
		if strings.Contains(d, pw) || strings.Contains(d, "burung-hantu") || strings.Contains(d, m[1]) {
			t.Errorf("deskripsi log memuat rahasia: %q", d)
		}
	}
}

func TestIntegrationActivityLogFilterByAction(t *testing.T) {
	integrationDB(t)
	uid, other := makeTestUser(t), makeTestUser(t)
	for _, a := range []string{"login", "logout", "login_failed", "note_create"} {
		LogActivity(uid, a, "user", uid, "x", "1.1.1.1", "ua")
	}
	LogActivity(other, "login", "user", other, "milik orang lain", "2.2.2.2", "ua")

	get := func(q string) []string {
		rec := callWrapped(ListActivityLog, uid, "GET", "/api/activity"+q, nil, nil)
		var rows []map[string]interface{}
		_ = json.Unmarshal(rec.Body.Bytes(), &rows)
		var out []string
		for _, r := range rows {
			out = append(out, fmt.Sprint(r["action"]))
		}
		return out
	}
	if got := get(""); len(got) != 4 {
		t.Errorf("tanpa filter harus 4 entri milik sendiri, dapat %v", got)
	}
	got := get("?action=login,logout,login_failed")
	if len(got) != 3 || strings.Contains(strings.Join(got, ","), "note_create") {
		t.Errorf("filter login/logout/login_failed salah: %v", got)
	}
	for _, g := range got {
		if g == "note_create" {
			t.Errorf("note_create tidak boleh lolos filter")
		}
	}
	// Masukan berbahaya diabaikan (tidak ada injeksi SQL; bila semua tidak valid, tanpa filter).
	if got := get("?action=login'%20OR%201=1--"); len(got) != 4 {
		t.Errorf("filter tak valid harus diabaikan (tanpa filter), dapat %v", got)
	}
	if got := get("?action=login,%3BDROP%20TABLE%20users"); len(got) != 1 || got[0] != "login" {
		t.Errorf("bagian tak valid diabaikan, bagian valid dipakai: %v", got)
	}
	// Tidak pernah memuat milik pengguna lain.
	if n := len(get("?action=login")); n != 1 {
		t.Errorf("hanya log login milik sendiri (1), dapat %d", n)
	}
}
