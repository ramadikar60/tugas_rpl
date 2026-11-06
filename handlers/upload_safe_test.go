package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pad(b []byte, n int) []byte {
	out := make([]byte, n)
	copy(out, b)
	return out
}

var (
	magicPNG  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	magicJPEG = []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00")
	magicGIF  = []byte("GIF89a\x01\x00\x01\x00")
	magicPDF  = []byte("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	magicZIP  = []byte("PK\x03\x04\x14\x00\x00\x00\x08\x00")
	magicOLE  = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0, 0, 0, 0}
	magicWEBP = []byte("RIFF\x24\x00\x00\x00WEBPVP8 ")
	magicMP4  = []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00")
	magicMOV  = []byte("\x00\x00\x00\x14ftypqt  \x00\x00\x00\x00")
)

func TestClassifyUploadAccepts(t *testing.T) {
	cases := []struct {
		name     string
		head     []byte
		filename string
		wantMime string
		wantExt  string
	}{
		{"png", magicPNG, "foto.png", "image/png", ".png"},
		{"png-nama-menipu", magicPNG, "evil.html", "image/png", ".png"},
		{"png-tanpa-ekstensi", magicPNG, "gambar", "image/png", ".png"},
		{"jpeg-ekstensi-salah", magicJPEG, "x.exe", "image/jpeg", ".jpg"},
		{"gif", magicGIF, "a.gif", "image/gif", ".gif"},
		{"webp", magicWEBP, "a.webp", "image/webp", ".webp"},
		{"pdf", magicPDF, "dok.pdf", "application/pdf", ".pdf"},
		{"mp4", magicMP4, "v.mp4", "video/mp4", ".mp4"},
		{"mov", magicMOV, "v.MOV", "video/quicktime", ".mov"},
		{"teks", []byte("halo dunia\nini catatan"), "n.txt", "text/plain", ".txt"},
		{"csv", []byte("a,b,c\n1,2,3\n"), "d.csv", "text/csv", ".csv"},
		{"json", []byte(`{"a":1}`), "d.json", "application/json", ".json"},
		{"zip", magicZIP, "a.zip", "application/zip", ".zip"},
		{"docx", magicZIP, "laporan.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"},
		{"xlsx", magicZIP, "a.XLSX", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx"},
		{"doc-lama", pad(magicOLE, 64), "lama.doc", "application/msword", ".doc"},
		{"xls-lama", pad(magicOLE, 64), "lama.xls", "application/vnd.ms-excel", ".xls"},
	}
	for _, c := range cases {
		mime, ext, ok := classifyUpload(pad(c.head, 512), c.filename)
		if c.name == "teks" || c.name == "csv" || c.name == "json" {
			mime, ext, ok = classifyUpload(c.head, c.filename)
		}
		if !ok || mime != c.wantMime || ext != c.wantExt {
			t.Errorf("[%s] dapat (%q, %q, %v), mau (%q, %q, true)", c.name, mime, ext, ok, c.wantMime, c.wantExt)
		}
	}
}

func TestClassifyUploadRejects(t *testing.T) {
	cases := []struct {
		name     string
		head     []byte
		filename string
	}{
		{"html", []byte("<!DOCTYPE html><html><body><script>alert(1)</script></body></html>"), "a.html"},
		{"html-nama-png", []byte("<html><script>alert(1)</script></html>"), "a.png"},
		{"html-tanpa-doctype", []byte("<script>alert(document.cookie)</script>"), "a.txt"},
		{"svg-prolog", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`), "a.svg"},
		{"svg-tanpa-prolog", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`), "a.svg"},
		{"svg-nama-png", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`), "a.png"},
		{"teks-bawa-script-nama-html", []byte("halo <script>alert(1)</script> dunia"), "x.html"},
		{"teks-nama-svg", []byte("halo dunia"), "x.svg"},
		{"teks-nama-js", []byte("alert(1)"), "x.js"},
		{"exe-nama-png", append([]byte("MZ\x90\x00\x03\x00"), bytes.Repeat([]byte{0}, 64)...), "x.png"},
		{"acak-nama-doc", append([]byte{0x01, 0x02, 0x03, 0x04}, bytes.Repeat([]byte{7}, 64)...), "x.doc"},
		{"acak-nama-mov", append([]byte{0x01, 0x02, 0x03, 0x04}, bytes.Repeat([]byte{7}, 64)...), "x.mov"},
		{"zip-nama-html", magicZIP, "x.html"},
		{"zip-nama-exe", magicZIP, "x.exe"},
		{"kosong-nama-html", []byte{}, "x.html"},
		{"kosong-nama-svg", []byte{}, "x.svg"},
	}
	for _, c := range cases {
		head := c.head
		if len(head) > 0 && head[0] != '<' && head[0] != 'h' && head[0] != 'a' && head[0] != 'M' && head[0] != 0x01 {
			head = pad(head, 512)
		}
		if mime, ext, ok := classifyUpload(head, c.filename); ok {
			t.Errorf("[%s] seharusnya ditolak, tetapi diterima sebagai (%q, %q)", c.name, mime, ext)
		}
	}
}

func TestStoredExtensionNeverFromUserName(t *testing.T) {
	// Apa pun nama dari klien, ekstensi simpan hanya dari daftar tetap.
	allowedExt := map[string]bool{".jpg": true, ".png": true, ".gif": true, ".webp": true, ".pdf": true, ".mp4": true,
		".webm": true, ".mp3": true, ".wav": true, ".ogg": true, ".txt": true, ".csv": true, ".json": true, ".zip": true,
		".docx": true, ".xlsx": true, ".pptx": true, ".doc": true, ".xls": true, ".ppt": true, ".mov": true}
	names := []string{"a.html", "a.svg", "a.php", "a.exe", "a.js", "a.HtMl", "a.png.html", "a", "a.png/../x.html", ".html"}
	heads := [][]byte{pad(magicPNG, 512), pad(magicJPEG, 512), magicPDF, []byte("teks biasa"), magicZIP}
	for _, h := range heads {
		for _, n := range names {
			if _, ext, ok := classifyUpload(h, n); ok && !allowedExt[ext] {
				t.Errorf("ekstensi tak terduga %q untuk nama %q", ext, n)
			}
		}
	}
}

func TestCleanFilename(t *testing.T) {
	cases := map[string]string{
		"laporan.pdf":                 "laporan.pdf",
		`C:\Users\a\Desktop\foto.png`: "foto.png",
		"../../etc/passwd":            "passwd",
		"a<script>b.png":              "ascriptb.png",
		`na"ma.png`:                   "nama.png",
		"":                            "file",
		"   ":                         "file",
		"..":                          "..",
		"tab\tbaris\nbaru.txt":        "tabbarisbaru.txt",
	}
	for in, want := range cases {
		if got := cleanFilename(in); got != want {
			t.Errorf("cleanFilename(%q) = %q, mau %q", in, got, want)
		}
	}
	long := strings.Repeat("a", 500) + ".png"
	if got := cleanFilename(long); len([]rune(got)) > 200 {
		t.Errorf("nama tidak dipotong: %d rune", len([]rune(got)))
	}
}

func TestImageExtForMIME(t *testing.T) {
	for mime, want := range map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"} {
		if got, ok := imageExtForMIME(mime); !ok || got != want {
			t.Errorf("%s -> %q,%v", mime, got, ok)
		}
	}
	for _, bad := range []string{"image/svg+xml", "text/html", "application/pdf", "text/plain", ""} {
		if _, ok := imageExtForMIME(bad); ok {
			t.Errorf("%q seharusnya ditolak untuk gambar clipboard", bad)
		}
	}
}

func TestSecureUploadHandlerHeaders(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		"a.png":  magicPNG,
		"b.html": []byte("<script>alert(1)</script>"),
		"c.svg":  []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`),
		"d.pdf":  magicPDF,
		"e.txt":  []byte("<script>alert(1)</script>"),
		"f.docx": magicZIP,
		"g.xml":  []byte("<?xml version='1.0'?><x/>"),
		"h.json": []byte(`{"a":1}`),
	}
	for n, b := range files {
		if err := os.WriteFile(filepath.Join(dir, n), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := SecureUploadHandler(dir)

	get := func(p string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		return rec
	}

	for n := range files {
		rec := get("/" + n)
		if rec.Code != 200 {
			t.Errorf("%s: status %d", n, rec.Code)
			continue
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: nosniff hilang", n)
		}
	}

	// File lama berbahaya: hanya unduh, tidak dirender.
	for _, n := range []string{"b.html", "g.xml", "f.docx"} {
		rec := get("/" + n)
		if rec.Header().Get("Content-Disposition") != "attachment" {
			t.Errorf("%s: harus attachment, dapat %q", n, rec.Header().Get("Content-Disposition"))
		}
		if rec.Header().Get("Content-Type") != "application/octet-stream" {
			t.Errorf("%s: Content-Type %q", n, rec.Header().Get("Content-Type"))
		}
		if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") {
			t.Errorf("%s: CSP sandbox hilang", n)
		}
	}
	// SVG lama: tampil sebagai gambar tetapi tidak bisa menjalankan skrip.
	rec := get("/c.svg")
	if rec.Header().Get("Content-Type") != "image/svg+xml" || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("c.svg: header salah: %v", rec.Header())
	}
	// Teks tidak pernah dirender sebagai HTML.
	rec = get("/e.txt")
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("e.txt: header salah: %v", rec.Header())
	}
	// Gambar: sandbox. PDF: tanpa CSP (penampil bawaan browser).
	if !strings.Contains(get("/a.png").Header().Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("a.png: CSP sandbox hilang")
	}
	if get("/d.pdf").Header().Get("Content-Security-Policy") != "" {
		t.Errorf("d.pdf: tidak boleh di-sandbox")
	}

	// Tanpa daftar isi folder.
	for _, p := range []string{"/", ""} {
		req := httptest.NewRequest("GET", "/x", nil)
		req.URL.Path = p
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 404 {
			t.Errorf("daftar folder (%q) harus 404, dapat %d", p, rec.Code)
		}
	}
	if get("/tidak-ada.png").Code != 404 {
		t.Errorf("file tak ada harus 404")
	}
}

func TestReadJSONLimit(t *testing.T) {
	mk := func(n int) *http.Request {
		body := `{"v":"` + strings.Repeat("a", n) + `"}`
		return httptest.NewRequest("POST", "/x", strings.NewReader(body))
	}
	var v struct{ V string }

	if err := readJSON(mk(1024), &v); err != nil || len(v.V) != 1024 {
		t.Errorf("body kecil harus lolos: %v", err)
	}
	if err := readJSON(mk(6<<20), &v); err == nil {
		t.Errorf("body 6 MB harus ditolak oleh batas bawaan 5 MB")
	}
	if err := readJSONMax(mk(6<<20), &v, bigJSONLimit); err != nil {
		t.Errorf("batas besar eksplisit harus menerima 6 MB: %v", err)
	}
	if err := readJSONMax(mk(2<<20), &v, 1<<20); err == nil {
		t.Errorf("body 2 MB harus ditolak oleh batas 1 MB")
	}
}
