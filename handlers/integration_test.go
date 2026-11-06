package handlers

// Tes integrasi: memanggil handler asli terhadap MySQL/MariaDB dengan skema asli.
// Otomatis dilewati bila NOTEBOOK_TEST_DB tidak bernilai "1".
//
// Contoh menjalankan (DB uji TERPISAH dari data asli!):
//   NOTEBOOK_TEST_DB=1 DB_USER=nbtest DB_PASS=nbtest DB_NAME=notebook_db go test ./handlers -run Integration -v

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

const dirtyHTML = `<p>aman <strong>tebal</strong></p><script>alert(1)</script>` +
	`<img src=x onerror="alert(2)"><a href="javascript:alert(3)">tautan</a>` +
	`<div class="adv-callout callout-info" data-type="info" data-title="&quot;&gt;&lt;img src=x onerror=alert(4)&gt;">c</div>` +
	`<iframe src="https://evil.example/"></iframe>`

func integrationDB(t *testing.T) {
	t.Helper()
	if os.Getenv("NOTEBOOK_TEST_DB") != "1" {
		t.Skip("integrasi dilewati (set NOTEBOOK_TEST_DB=1 dengan DB uji)")
	}
	if config.DB == nil {
		if err := config.InitDB(); err != nil {
			t.Fatalf("tidak bisa terhubung ke DB uji: %v", err)
		}
	}
}

func makeTestUser(t *testing.T) int64 {
	t.Helper()
	n := time.Now().UnixNano()
	res, err := config.DB.Exec("INSERT INTO users (email, username, password_hash) VALUES (?, ?, 'x')",
		fmt.Sprintf("it%d@example.test", n), fmt.Sprintf("it%d", n))
	if err != nil {
		t.Fatalf("buat user uji: %v", err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { _, _ = config.DB.Exec("DELETE FROM users WHERE id=?", id) })
	return id
}

func callAs(userID int64, h http.HandlerFunc, method, target string, body interface{}, pv map[string]string) *httptest.ResponseRecorder {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rd)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	for k, v := range pv {
		req.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func idFrom(t *testing.T, rec *httptest.ResponseRecorder) int64 {
	t.Helper()
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.ID == 0 {
		t.Fatalf("tidak ada id di respons (%d): %s", rec.Code, rec.Body.String())
	}
	return out.ID
}

func storedContent(t *testing.T, noteID int64) string {
	t.Helper()
	var c string
	if err := config.DB.QueryRow("SELECT content FROM notes WHERE id=?", noteID).Scan(&c); err != nil {
		t.Fatalf("baca catatan %d: %v", noteID, err)
	}
	return c
}

func assertCleanStored(t *testing.T, label, s string) {
	t.Helper()
	assertSafeHTML(t, label, s)
	if !strings.Contains(s, "<strong>tebal</strong>") {
		t.Errorf("[%s] isi sah hilang: %s", label, s)
	}
	for _, bad := range []string{"<script", "javascript:", "evil.example", "<iframe"} {
		if strings.Contains(strings.ToLower(s), bad) {
			t.Errorf("[%s] masih memuat %q: %s", label, bad, s)
		}
	}
}

func TestIntegrationNoteWritePathsSanitize(t *testing.T) {
	integrationDB(t)
	uid := makeTestUser(t)

	// CreateNote
	rec := callAs(uid, CreateNote, "POST", "/api/notes", map[string]interface{}{"title": "T", "content": dirtyHTML}, nil)
	if rec.Code != 201 {
		t.Fatalf("CreateNote: %d %s", rec.Code, rec.Body.String())
	}
	nid := idFrom(t, rec)
	assertCleanStored(t, "CreateNote", storedContent(t, nid))

	// UpdateNote
	pv := map[string]string{"id": fmt.Sprint(nid)}
	rec = callAs(uid, UpdateNote, "PUT", "/api/notes/x", map[string]interface{}{"content": "<p>v2 <strong>tebal</strong></p>" + dirtyHTML}, pv)
	if rec.Code != 200 {
		t.Fatalf("UpdateNote: %d %s", rec.Code, rec.Body.String())
	}
	assertCleanStored(t, "UpdateNote", storedContent(t, nid))

	// Semua versi tersimpan juga bersih.
	rows, err := config.DB.Query("SELECT content FROM note_versions WHERE note_id=?", nid)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		_ = rows.Scan(&c)
		assertSafeHTML(t, "versi", c)
	}

	// DuplicateNote dari data lama yang "kotor" (disimulasikan lewat SQL langsung).
	_, _ = config.DB.Exec("UPDATE notes SET content=? WHERE id=?", dirtyHTML, nid)
	rec = callAs(uid, DuplicateNote, "POST", "/api/notes/x/duplicate", nil, pv)
	if rec.Code != 201 && rec.Code != 200 {
		t.Fatalf("DuplicateNote: %d %s", rec.Code, rec.Body.String())
	}
	assertCleanStored(t, "DuplicateNote", storedContent(t, idFrom(t, rec)))
}

func TestIntegrationSharedPageSanitizesLegacyContent(t *testing.T) {
	integrationDB(t)
	uid := makeTestUser(t)
	rec := callAs(uid, CreateNote, "POST", "/api/notes", map[string]interface{}{"title": "S", "content": "<p>x</p>"}, nil)
	nid := idFrom(t, rec)

	// Data lama yang sudah kotor di DB sebelum sanitasi ada.
	_, _ = config.DB.Exec("UPDATE notes SET content=? WHERE id=?", dirtyHTML, nid)
	tok := fmt.Sprintf("tok-%d", time.Now().UnixNano())
	if _, err := config.DB.Exec(`INSERT INTO shares (user_id, item_type, item_id, share_mode, permission, token, title)
		VALUES (?, 'note', ?, 'public', 'viewer', ?, 'S')`, uid, nid, tok); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/shared/access?token="+tok, nil)
	out := httptest.NewRecorder()
	AccessSharedNote(out, req)
	if out.Code != 200 {
		t.Fatalf("AccessSharedNote: %d %s", out.Code, out.Body.String())
	}
	var resp struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal(out.Body.Bytes(), &resp)
	assertCleanStored(t, "halaman berbagi", resp.Content)
}

func TestIntegrationRestoreVersionOwnershipAndSanitize(t *testing.T) {
	integrationDB(t)
	owner, intruder := makeTestUser(t), makeTestUser(t)
	nid := idFrom(t, callAs(owner, CreateNote, "POST", "/api/notes", map[string]interface{}{"title": "R", "content": "<p>asli</p>"}, nil))

	// Versi lama yang kotor (sebelum sanitasi ada).
	res, err := config.DB.Exec("INSERT INTO note_versions (note_id, user_id, version_number, title, content, is_auto) VALUES (?, ?, 1, 'v', ?, 1)", nid, owner, dirtyHTML)
	if err != nil {
		t.Skipf("skema note_versions berbeda: %v", err)
	}
	vid, _ := res.LastInsertId()
	pv := map[string]string{"id": fmt.Sprint(nid), "version_id": fmt.Sprint(vid)}

	// Bukan pemilik: ditolak, dan catatan tidak berubah.
	var versionsBefore int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM note_versions WHERE note_id=?", nid).Scan(&versionsBefore)
	rec := callAs(intruder, RestoreNoteVersion, "POST", "/x", nil, pv)
	if rec.Code != 403 {
		t.Errorf("non-pemilik harus 403, dapat %d: %s", rec.Code, rec.Body.String())
	}
	var versionsAfter int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM note_versions WHERE note_id=?", nid).Scan(&versionsAfter)
	if versionsAfter != versionsBefore {
		t.Errorf("non-pemilik tidak boleh menambah versi: %d -> %d", versionsBefore, versionsAfter)
	}
	if storedContent(t, nid) != "<p>asli</p>" {
		t.Errorf("catatan berubah oleh non-pemilik")
	}

	// Pemilik: berhasil, dan isi hasil pulihan bersih.
	rec = callAs(owner, RestoreNoteVersion, "POST", "/x", nil, pv)
	if rec.Code != 200 {
		t.Fatalf("pemilik harus bisa restore: %d %s", rec.Code, rec.Body.String())
	}
	assertCleanStored(t, "restore", storedContent(t, nid))
}

func TestIntegrationClipperAndQuickCapture(t *testing.T) {
	integrationDB(t)
	uid := makeTestUser(t)

	// URL berbahaya ditolak.
	for _, u := range []string{"javascript:alert(1)", "data:text/html,<script>alert(1)</script>"} {
		rec := callAs(uid, ClipWebPage, "POST", "/api/clips", map[string]interface{}{"url": u, "title": "x"}, nil)
		if rec.Code != 400 {
			t.Errorf("clip URL %q harus 400, dapat %d", u, rec.Code)
		}
	}

	// Judul berisi HTML di-escape; konten disanitasi.
	rec := callAs(uid, ClipWebPage, "POST", "/api/clips", map[string]interface{}{
		"url": "https://example.com/a", "title": `<img src=x onerror=alert(1)>Judul`, "create_note": true,
		"content": dirtyHTML, "cover_image": "javascript:alert(1)",
	}, nil)
	if rec.Code != 200 && rec.Code != 201 {
		t.Fatalf("ClipWebPage: %d %s", rec.Code, rec.Body.String())
	}
	var nid int64
	_ = config.DB.QueryRow("SELECT id FROM notes WHERE user_id=? ORDER BY id DESC LIMIT 1", uid).Scan(&nid)
	if nid == 0 {
		t.Fatalf("catatan klip tidak dibuat: %s", rec.Body.String())
	}
	c := storedContent(t, nid)
	assertCleanStored(t, "klip->catatan", c)
	// Judul harus tersimpan sebagai teks ter-escape, bukan elemen.
	if !strings.HasPrefix(c, "<h1>&lt;img src=x onerror=alert(1)&gt;Judul</h1>") {
		t.Errorf("judul klip tidak di-escape: %s", c)
	}
	var cover string
	_ = config.DB.QueryRow("SELECT COALESCE(cover_image,'') FROM clips WHERE user_id=? ORDER BY id DESC LIMIT 1", uid).Scan(&cover)
	if strings.Contains(cover, "javascript") {
		t.Errorf("cover_image berbahaya tersimpan: %q", cover)
	}

	// Quick capture sebagai bookmark: URL berisi tanda kutip tidak boleh memecah atribut.
	rec = callAs(uid, ClipboardCapture, "POST", "/api/quick/clipboard",
		map[string]interface{}{"content": `https://a.com/x" onmouseover="alert(1)`, "as_type": "bookmark"}, nil)
	_ = config.DB.QueryRow("SELECT id FROM notes WHERE user_id=? ORDER BY id DESC LIMIT 1", uid).Scan(&nid)
	q := storedContent(t, nid)
	assertSafeHTML(t, "quick-bookmark", q)
	if strings.Contains(strings.ToLower(q), `onmouseover="`) {
		t.Errorf("injeksi atribut lolos di quick capture: %s", q)
	}
}

func TestIntegrationImportAndTemplateSanitize(t *testing.T) {
	integrationDB(t)
	uid := makeTestUser(t)

	rec := callAs(uid, ImportNotes, "POST", "/api/import/notes", map[string]interface{}{
		"notes": []map[string]string{{"title": "Impor", "content": dirtyHTML}},
	}, nil)
	if rec.Code != 200 && rec.Code != 201 {
		t.Fatalf("ImportNotes: %d %s", rec.Code, rec.Body.String())
	}
	var nid int64
	_ = config.DB.QueryRow("SELECT id FROM notes WHERE user_id=? ORDER BY id DESC LIMIT 1", uid).Scan(&nid)
	assertCleanStored(t, "impor", storedContent(t, nid))

	// Templat kotor yang sudah ada di DB lalu dipakai membuat catatan.
	res, err := config.DB.Exec("INSERT INTO templates (user_id, name, content, category) VALUES (?, 'T', ?, 'general')", uid, dirtyHTML)
	if err != nil {
		t.Skipf("skema templates berbeda: %v", err)
	}
	tid, _ := res.LastInsertId()
	rec = callAs(uid, CreateNoteFromTemplate, "POST", "/api/notes/from-template", map[string]interface{}{"template_id": tid}, nil)
	if rec.Code != 200 && rec.Code != 201 {
		t.Fatalf("CreateNoteFromTemplate: %d %s", rec.Code, rec.Body.String())
	}
	assertCleanStored(t, "dari-templat", storedContent(t, idFrom(t, rec)))
}

func multipartUpload(t *testing.T, field, filename, contentType string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, filename))
	h.Set("Content-Type", contentType)
	part, _ := mw.CreatePart(h)
	_, _ = part.Write(data)
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestIntegrationUploadRejectsDisguisedFiles(t *testing.T) {
	integrationDB(t)
	dir := t.TempDir()
	t.Setenv("UPLOAD_DIR", dir)
	uid := makeTestUser(t)
	nid := idFrom(t, callAs(uid, CreateNote, "POST", "/api/notes", map[string]interface{}{"title": "U", "content": "<p>x</p>"}, nil))
	pv := map[string]string{"note_id": fmt.Sprint(nid)}

	upload := func(filename, ctype string, data []byte) *httptest.ResponseRecorder {
		body, ct := multipartUpload(t, "file", filename, ctype, data)
		req := httptest.NewRequest("POST", "/x", body)
		req.Header.Set("Content-Type", ct)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
		req.SetPathValue("note_id", pv["note_id"])
		rec := httptest.NewRecorder()
		UploadAttachment(rec, req)
		return rec
	}
	filesOnDisk := func() []string {
		ents, _ := os.ReadDir(dir)
		var out []string
		for _, e := range ents {
			out = append(out, e.Name())
		}
		return out
	}

	script := []byte("halo <script>alert(document.cookie)</script>")
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`)
	html := []byte("<!DOCTYPE html><script>alert(1)</script>")

	// Serangan lama: Content-Type palsu + ekstensi berbahaya.
	for _, c := range []struct {
		name, fn, ct string
		data         []byte
	}{
		{"teks bawa script, nama .html, ctype text/plain", "x.html", "text/plain", script},
		{"svg dengan ctype image/png", "x.svg", "image/png", svg},
		{"svg bernama png", "x.png", "image/png", svg},
		{"html asli", "x.html", "text/html", html},
		{"html menyamar png", "x.png", "image/png", html},
	} {
		rec := upload(c.fn, c.ct, c.data)
		if rec.Code != 400 {
			t.Errorf("[%s] harus 400, dapat %d: %s", c.name, rec.Code, rec.Body.String())
		}
	}
	if len(filesOnDisk()) != 0 {
		t.Errorf("file ditolak tidak boleh tersimpan: %v", filesOnDisk())
	}

	// PNG asli dengan nama .html: diterima, tetapi disimpan sebagai .png.
	png := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), bytes.Repeat([]byte{0}, 64)...)
	rec := upload("menipu.html", "text/html", png)
	if rec.Code != 200 && rec.Code != 201 {
		t.Fatalf("PNG sah ditolak: %d %s", rec.Code, rec.Body.String())
	}
	files := filesOnDisk()
	if len(files) != 1 || !strings.HasSuffix(files[0], ".png") {
		t.Errorf("ekstensi simpan harus .png, dapat %v", files)
	}
	var mime string
	_ = config.DB.QueryRow("SELECT mime_type FROM attachments WHERE note_id=? ORDER BY id DESC LIMIT 1", nid).Scan(&mime)
	if mime != "image/png" {
		t.Errorf("mime tersimpan %q, harus image/png", mime)
	}
}

func TestIntegrationClipboardImageValidatesContent(t *testing.T) {
	integrationDB(t)
	dir := t.TempDir()
	t.Setenv("UPLOAD_DIR", dir)
	uid := makeTestUser(t)
	nid := idFrom(t, callAs(uid, CreateNote, "POST", "/api/notes", map[string]interface{}{"title": "C", "content": "<p>x</p>"}, nil))
	pv := map[string]string{"note_id": fmt.Sprint(nid)}

	enc := func(b []byte) string { return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b) }

	// Isi bukan gambar (HTML/SVG) walau diberi label image/png.
	for name, data := range map[string][]byte{
		"html":   []byte("<html><script>alert(1)</script></html>"),
		"svg":    []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`),
		"kosong": {},
	} {
		rec := callAs(uid, UploadClipboardImage, "POST", "/x", map[string]string{"data_url": enc(data), "filename": "a.png"}, pv)
		if rec.Code != 400 {
			t.Errorf("[%s] harus 400, dapat %d: %s", name, rec.Code, rec.Body.String())
		}
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("tidak boleh ada file tersimpan untuk isi non-gambar")
	}

	// PNG sah diterima dan disimpan .png.
	png := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), bytes.Repeat([]byte{0}, 64)...)
	rec := callAs(uid, UploadClipboardImage, "POST", "/x", map[string]string{"data_url": enc(png), "filename": `<b>x</b>.png`}, pv)
	if rec.Code != 200 && rec.Code != 201 {
		t.Fatalf("PNG sah ditolak: %d %s", rec.Code, rec.Body.String())
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 || filepath.Ext(ents[0].Name()) != ".png" {
		t.Errorf("harus tersimpan satu file .png: %v", ents)
	}
	var fname string
	_ = config.DB.QueryRow("SELECT filename FROM attachments WHERE note_id=? ORDER BY id DESC LIMIT 1", nid).Scan(&fname)
	if strings.ContainsAny(fname, "<>") {
		t.Errorf("nama file tidak dibersihkan: %q", fname)
	}
}

func TestAutoVersionIntervalConfig(t *testing.T) {
	if got := autoVersionIntervalSec(); got != 300 {
		t.Errorf("bawaan harus 300 detik (5 menit), dapat %d", got)
	}
	for env, want := range map[string]int{"60": 60, "1": 1, "0": 300, "-5": 300, "abc": 300, "": 300} {
		t.Setenv("VERSION_AUTO_INTERVAL_SEC", env)
		if got := autoVersionIntervalSec(); got != want {
			t.Errorf("VERSION_AUTO_INTERVAL_SEC=%q -> %d, mau %d", env, got, want)
		}
	}
}

func TestIntegrationAutoVersionsLimitedPerInterval(t *testing.T) {
	integrationDB(t)
	uid := makeTestUser(t)
	nid := idFrom(t, callAs(uid, CreateNote, "POST", "/api/notes", map[string]interface{}{"title": "V", "content": "<p>awal</p>"}, nil))
	pv := map[string]string{"id": fmt.Sprint(nid)}
	count := func() int {
		var n int
		_ = config.DB.QueryRow("SELECT COUNT(*) FROM note_versions WHERE note_id=? AND is_auto=1", nid).Scan(&n)
		return n
	}
	put := func(html string) {
		if rec := callAs(uid, UpdateNote, "PUT", "/x", map[string]interface{}{"content": html}, pv); rec.Code != 200 {
			t.Fatalf("UpdateNote: %d %s", rec.Code, rec.Body.String())
		}
	}

	before := count()
	for i := 1; i <= 5; i++ { // lima simpan beruntun (seperti autosave)
		put(fmt.Sprintf("<p>revisi %d</p>", i))
	}
	if got := count() - before; got != 1 {
		t.Errorf("lima simpan beruntun harus menghasilkan 1 snapshot otomatis, dapat %d", got)
	}

	// Snapshot berumur 2 menit masih memblokir (aturan lama 30 detik akan meloloskannya).
	_, _ = config.DB.Exec("UPDATE note_versions SET created_at = DATE_SUB(NOW(), INTERVAL 2 MINUTE) WHERE note_id=?", nid)
	put("<p>revisi pada menit ke-2</p>")
	if got := count() - before; got != 1 {
		t.Errorf("snapshot berumur 2 menit harus masih memblokir (batas 5 menit), dapat %d", got)
	}

	// Setelah interval lewat (disimulasikan dengan memundurkan waktu), snapshot baru dibuat.
	_, _ = config.DB.Exec("UPDATE note_versions SET created_at = DATE_SUB(NOW(), INTERVAL 6 MINUTE) WHERE note_id=?", nid)
	put("<p>revisi setelah 6 menit</p>")
	if got := count() - before; got != 2 {
		t.Errorf("setelah 6 menit harus ada snapshot baru (total 2), dapat %d", got)
	}

	// Isi identik tidak membuat snapshot walau interval sudah lewat.
	_, _ = config.DB.Exec("UPDATE note_versions SET created_at = DATE_SUB(NOW(), INTERVAL 6 MINUTE) WHERE note_id=?", nid)
	put("<p>revisi setelah 6 menit</p>")
	if got := count() - before; got != 2 {
		t.Errorf("isi identik tidak boleh membuat snapshot, dapat %d", got)
	}

	// Versi manual tidak terkena batas (ukur selisihnya; CreateNote bisa membuat versi awal sendiri).
	countManual := func() int {
		var n int
		_ = config.DB.QueryRow("SELECT COUNT(*) FROM note_versions WHERE note_id=? AND is_auto=0", nid).Scan(&n)
		return n
	}
	manualBefore := countManual()
	SaveVersion(nid, uid, "manual 1", "<p>manual 1</p>", false)
	SaveVersion(nid, uid, "manual 2", "<p>manual 2</p>", false) // beruntun, tetap tersimpan
	if got := countManual() - manualBefore; got != 2 {
		t.Errorf("dua versi manual beruntun harus tersimpan semua, dapat %d", got)
	}
}
