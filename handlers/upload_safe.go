package handlers

// ============================================================
// UPLOAD_SAFE.GO - Penguatan upload lampiran (Prompt 13)
//
// Aturan:
//  1. Tipe file ditentukan dari ISI (sniffing), bukan dari header Content-Type
//     atau nama file dari klien.
//  2. Ekstensi penyimpanan diturunkan dari tipe yang terdeteksi.
//  3. SVG dan HTML ditolak.
//  4. /uploads/ disajikan dengan nosniff + CSP sandbox, dan file di luar daftar
//     aman (termasuk file lama) hanya bisa diunduh, tidak dirender.
// ============================================================

import (
	"bytes"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

// classifyUpload mengembalikan (mime, ekstensi simpan, ok).
// head = beberapa ratus byte pertama file.
func classifyUpload(head []byte, filename string) (string, string, bool) {
	sniffed := strings.TrimSpace(strings.SplitN(http.DetectContentType(head), ";", 2)[0])
	userExt := strings.ToLower(filepath.Ext(filename))

	// 1. Tipe yang ditentukan penuh oleh isi.
	switch sniffed {
	case "image/jpeg":
		return "image/jpeg", ".jpg", true
	case "image/png":
		return "image/png", ".png", true
	case "image/gif":
		return "image/gif", ".gif", true
	case "image/webp":
		return "image/webp", ".webp", true
	case "application/pdf":
		return "application/pdf", ".pdf", true
	case "video/mp4":
		return "video/mp4", ".mp4", true
	case "video/webm":
		return "video/webm", ".webm", true
	case "audio/mpeg":
		return "audio/mpeg", ".mp3", true
	case "audio/wave", "audio/wav":
		return "audio/wav", ".wav", true
	case "application/ogg", "audio/ogg":
		return "audio/ogg", ".ogg", true
	}

	// 2. Tipe generik: isi tidak cukup spesifik, ekstensi dari klien hanya boleh
	//    memilih di dalam keluarga yang aman untuk isi tersebut.
	switch sniffed {
	case "text/plain":
		switch userExt {
		case ".txt":
			return "text/plain", ".txt", true
		case ".csv":
			return "text/csv", ".csv", true
		case ".json":
			return "application/json", ".json", true
		}
	case "application/zip":
		switch userExt {
		case ".zip":
			return "application/zip", ".zip", true
		case ".docx":
			return "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx", true
		case ".xlsx":
			return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx", true
		case ".pptx":
			return "application/vnd.openxmlformats-officedocument.presentationml.presentation", ".pptx", true
		}
	case "application/octet-stream":
		switch userExt {
		case ".doc", ".xls", ".ppt":
			// Format Office lama (OLE Compound File).
			if bytes.HasPrefix(head, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}) {
				switch userExt {
				case ".doc":
					return "application/msword", ".doc", true
				case ".xls":
					return "application/vnd.ms-excel", ".xls", true
				default:
					return "application/vnd.ms-powerpoint", ".ppt", true
				}
			}
		case ".mov":
			// QuickTime: atom "ftyp"/"moov"/"mdat"/"wide"/"free" pada byte 4-8.
			if len(head) >= 12 {
				switch string(head[4:8]) {
				case "ftyp", "moov", "mdat", "wide", "free":
					return "video/quicktime", ".mov", true
				}
			}
		}
	}
	return "", "", false
}

// imageExtForMIME dipakai untuk gambar clipboard.
func imageExtForMIME(mime string) (string, bool) {
	switch mime {
	case "image/png":
		return ".png", true
	case "image/jpeg":
		return ".jpg", true
	case "image/gif":
		return ".gif", true
	case "image/webp":
		return ".webp", true
	}
	return "", false
}

// cleanFilename membersihkan nama file dari klien untuk ditampilkan/disimpan.
func cleanFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	if name == "." || name == "/" {
		name = ""
	}
	var b strings.Builder
	n := 0
	for _, r := range name {
		if unicode.IsControl(r) || r == '<' || r == '>' || r == '"' {
			continue
		}
		b.WriteRune(r)
		n++
		if n >= 200 {
			break
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		out = "file"
	}
	return out
}

// SecureUploadHandler menyajikan folder upload dengan header keamanan.
func SecureUploadHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Tanpa daftar isi folder.
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")

		switch strings.ToLower(path.Ext(r.URL.Path)) {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".mp4", ".webm", ".mp3", ".wav", ".ogg":
			h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		case ".svg":
			// File lama: boleh tampil sebagai gambar, tetapi tidak bisa menjalankan skrip
			// walau dibuka langsung.
			h.Set("Content-Type", "image/svg+xml")
			h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		case ".pdf":
			// Penampil PDF bawaan browser gagal bila di-sandbox; cukup nosniff.
		case ".txt":
			h.Set("Content-Type", "text/plain; charset=utf-8")
			h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		case ".csv":
			h.Set("Content-Type", "text/csv; charset=utf-8")
			h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		case ".json":
			h.Set("Content-Type", "application/json; charset=utf-8")
			h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		default:
			// Sisanya (Office, zip, serta file lama .html/.xml/.js dll) hanya diunduh.
			h.Set("Content-Type", "application/octet-stream")
			h.Set("Content-Disposition", "attachment")
			h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		}
		fs.ServeHTTP(w, r)
	})
}
