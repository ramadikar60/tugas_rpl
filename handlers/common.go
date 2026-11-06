package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"notebook-app/config"
)

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// ownsNote / ownsShare: pemeriksaan kepemilikan SEBELUM aksi mengubah data. Tanpa ini, aksi pada
// item milik orang lain tidak berefek (WHERE ... AND user_id=?) tetapi tetap membalas sukses,
// yang menyesatkan klien dan membuat log audit mencatat aksi yang tidak pernah terjadi.
func ownsNote(userID, id int64) bool {
	var n int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM notes WHERE id=? AND user_id=?", id, userID).Scan(&n)
	return n > 0
}

func ownsShare(userID, id int64) bool {
	var n int
	_ = config.DB.QueryRow("SELECT COUNT(*) FROM shares WHERE id=? AND user_id=?", id, userID).Scan(&n)
	return n > 0
}

// Batas ukuran body JSON. Bawaan 5 MB (env MAX_JSON_MB untuk mengubah).
// Endpoint yang memang membawa data besar memakai readJSONMax secara eksplisit.
const (
	defaultJSONLimitMB = 5
	bigJSONLimit       = 25 << 20 // whiteboard / mind map (data + thumbnail)
	importJSONLimit    = 50 << 20 // impor catatan
	imageJSONLimit     = 20 << 20 // gambar clipboard (base64)
)

func jsonBodyLimit() int64 {
	mb, err := strconv.Atoi(config.GetEnv("MAX_JSON_MB", ""))
	if err != nil || mb < 1 {
		mb = defaultJSONLimitMB
	}
	return int64(mb) << 20
}

func readJSON(r *http.Request, v interface{}) error {
	return readJSONMax(r, v, jsonBodyLimit())
}

// readJSONMax membaca JSON dengan batas ukuran. Melebihi batas menghasilkan error
// (pemanggil membalas 400) sehingga memori server tidak bisa dihabiskan satu request.
func readJSONMax(r *http.Request, v interface{}, max int64) error {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(nil, r.Body, max)
	return json.NewDecoder(r.Body).Decode(v)
}

func pathInt(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(r.PathValue(key), 10, 64)
}

func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// Helper convert int64 ke string
func intToStr(n int64) string {
	return strconv.FormatInt(n, 10)
}

// Helper ambil upload dir dari env
func uploadDir() string {
	return config.GetEnv("UPLOAD_DIR", "./uploads")
}
