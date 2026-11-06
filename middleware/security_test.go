package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
}

func do(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestSecurityHeadersDefaultEnforce(t *testing.T) {
	rec := do(SecurityHeaders(okHandler()), "/")
	h := rec.Header()
	if h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("nosniff hilang")
	}
	if h.Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options salah: %q", h.Get("X-Frame-Options"))
	}
	if h.Get("Referrer-Policy") == "" {
		t.Errorf("Referrer-Policy hilang")
	}
	csp := h.Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "object-src 'none'", "base-uri 'self'", "form-action 'self'",
		"frame-ancestors 'self'", "https://cdn.jsdelivr.net", "https://www.youtube.com"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP kehilangan %q: %s", want, csp)
		}
	}
	if h.Get("Content-Security-Policy-Report-Only") != "" {
		t.Errorf("mode enforce tidak boleh memasang Report-Only")
	}
	if h.Get("Strict-Transport-Security") != "" {
		t.Errorf("HSTS tidak boleh dipasang pada HTTP biasa")
	}
}

func TestSecurityHeadersCSPModes(t *testing.T) {
	t.Setenv("CSP_MODE", "report-only")
	h := do(SecurityHeaders(okHandler()), "/").Header()
	if h.Get("Content-Security-Policy") != "" || h.Get("Content-Security-Policy-Report-Only") == "" {
		t.Errorf("mode report-only salah: %v", h)
	}
	t.Setenv("CSP_MODE", "off")
	h = do(SecurityHeaders(okHandler()), "/").Header()
	if h.Get("Content-Security-Policy") != "" || h.Get("Content-Security-Policy-Report-Only") != "" {
		t.Errorf("mode off masih memasang CSP: %v", h)
	}
	if h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("header lain tetap harus dipasang saat CSP off")
	}
}

func TestSecurityHeadersSkipCSPForUploads(t *testing.T) {
	h := do(SecurityHeaders(okHandler()), "/uploads/a.pdf").Header()
	if h.Get("Content-Security-Policy") != "" {
		t.Errorf("/uploads/ mengatur CSP sendiri, global tidak boleh dipasang")
	}
	if h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("nosniff tetap wajib untuk /uploads/")
	}
}

func TestSecurityHeadersHSTS(t *testing.T) {
	req := httptest.NewRequest("GET", "https://example.com/", nil) // TLS non-nil
	rec := httptest.NewRecorder()
	SecurityHeaders(okHandler()).ServeHTTP(rec, req)
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Errorf("HSTS harus ada pada HTTPS")
	}

	// X-Forwarded-Proto tidak dipercaya tanpa TRUST_PROXY.
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec = httptest.NewRecorder()
	SecurityHeaders(okHandler()).ServeHTTP(rec, req)
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Errorf("X-Forwarded-Proto tidak boleh dipercaya tanpa TRUST_PROXY")
	}
	t.Setenv("TRUST_PROXY", "true")
	rec = httptest.NewRecorder()
	SecurityHeaders(okHandler()).ServeHTTP(rec, req)
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Errorf("dengan TRUST_PROXY, X-Forwarded-Proto=https harus memasang HSTS")
	}
}

func reqFrom(remote, xff string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = remote
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	return r
}

func TestClientIPIgnoresSpoofedHeaderByDefault(t *testing.T) {
	// Penyerang memutar X-Forwarded-For untuk menghindari batas per IP.
	for _, spoof := range []string{"1.2.3.4", "9.9.9.9, 8.8.8.8", "bukan-ip", "::1"} {
		if got := ClientIP(reqFrom("203.0.113.7:51234", spoof)); got != "203.0.113.7" {
			t.Errorf("XFF %q seharusnya diabaikan, dapat %q", spoof, got)
		}
	}
	if got := ClientIP(reqFrom("[2001:db8::1]:8080", "")); got != "2001:db8::1" {
		t.Errorf("IPv6 salah: %q", got)
	}
	if got := ClientIP(reqFrom("tanpa-port", "")); got != "tanpa-port" {
		t.Errorf("RemoteAddr tanpa port: %q", got)
	}
}

func TestClientIPTrustProxy(t *testing.T) {
	t.Setenv("TRUST_PROXY", "true")
	// Entri paling kanan = yang ditambahkan proxy kita; entri kiri bisa dipalsukan klien.
	if got := ClientIP(reqFrom("10.0.0.1:1234", "6.6.6.6, 198.51.100.9")); got != "198.51.100.9" {
		t.Errorf("harus memakai entri paling kanan, dapat %q", got)
	}
	if got := ClientIP(reqFrom("10.0.0.1:1234", "198.51.100.9")); got != "198.51.100.9" {
		t.Errorf("satu entri: %q", got)
	}
	if got := ClientIP(reqFrom("10.0.0.1:1234", "bukan-ip")); got != "10.0.0.1" {
		t.Errorf("XFF tidak valid harus jatuh ke RemoteAddr, dapat %q", got)
	}
	if got := ClientIP(reqFrom("10.0.0.1:1234", "")); got != "10.0.0.1" {
		t.Errorf("tanpa XFF harus RemoteAddr, dapat %q", got)
	}
}
