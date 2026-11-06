package middleware

// ============================================================
// SECURITY.GO - Header keamanan dan ClientIP (Prompt 13)
//
// Env:
//   CSP_MODE     enforce (bawaan) | report-only | off
//   TRUST_PROXY  true bila aplikasi berada di belakang reverse proxy tepercaya.
//                Bawaan false: X-Forwarded-For DIABAIKAN (bisa dipalsukan klien).
// ============================================================

import (
	"net"
	"net/http"
	"strings"

	"notebook-app/config"
)

// CSP: 'unsafe-inline' pada script-src masih diperlukan karena index.html memakai
// atribut onclick inline. Perlindungan utama tetap sanitasi isi catatan; CSP
// membatasi asal skrip/frame, melarang object, dan mengunci base/form/frame-ancestors.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; " +
	"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; " +
	"font-src 'self' data: https://cdn.jsdelivr.net; " +
	"img-src 'self' data: blob: https:; " +
	"media-src 'self' blob:; " +
	"connect-src 'self' blob:; " +
	"frame-src https://www.youtube.com https://www.youtube-nocookie.com https://player.vimeo.com https://open.spotify.com; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'self'"

func trustProxy() bool {
	v := strings.ToLower(strings.TrimSpace(config.GetEnv("TRUST_PROXY", "false")))
	return v == "true" || v == "1" || v == "yes"
}

// IsHTTPS true bila koneksi memakai TLS, atau (dengan TRUST_PROXY=true) proxy
// melaporkan X-Forwarded-Proto=https. Dipakai untuk HSTS dan flag Secure pada cookie.
func IsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return trustProxy() && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// SecurityHeaders menambahkan header keamanan standar ke semua respons.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// HSTS hanya bila koneksi memang HTTPS.
		if IsHTTPS(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}

		// /uploads/ mengatur CSP-nya sendiri (sandbox per jenis file).
		if !strings.HasPrefix(r.URL.Path, "/uploads/") {
			switch strings.ToLower(strings.TrimSpace(config.GetEnv("CSP_MODE", "enforce"))) {
			case "off":
			case "report-only":
				h.Set("Content-Security-Policy-Report-Only", contentSecurityPolicy)
			default:
				h.Set("Content-Security-Policy", contentSecurityPolicy)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ClientIP mengembalikan alamat klien. X-Forwarded-For hanya dipercaya bila
// TRUST_PROXY=true, dan yang dipakai adalah entri paling kanan (ditambahkan
// oleh proxy kita sendiri, bukan oleh klien).
func ClientIP(r *http.Request) string {
	remote := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	if trustProxy() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			cand := strings.TrimSpace(parts[len(parts)-1])
			if ip := net.ParseIP(cand); ip != nil {
				return ip.String()
			}
		}
	}
	return remote
}
