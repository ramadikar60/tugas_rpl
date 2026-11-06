package auth

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestAccessTokenRoundTrip(t *testing.T) {
	tok, err := GenerateAccessToken(42)
	if err != nil {
		t.Fatal(err)
	}
	id, err := VerifyAccessToken(tok)
	if err != nil || id != 42 {
		t.Fatalf("round trip gagal: id=%d err=%v", id, err)
	}
}

func TestVerifyRejectsTamperedExpiredAndWrongType(t *testing.T) {
	tok, _ := GenerateAccessToken(7)
	parts := strings.Split(tok, ".")

	// Tanda tangan diubah.
	sig := []byte(parts[2])
	if sig[0] == 'A' {
		sig[0] = 'B'
	} else {
		sig[0] = 'A'
	}
	if _, err := VerifyAccessToken(parts[0] + "." + parts[1] + "." + string(sig)); err == nil {
		t.Errorf("tanda tangan yang diubah harus ditolak")
	}

	// Payload diubah (mencoba menjadi user lain) dengan tanda tangan lama.
	forged := base64.RawURLEncoding.EncodeToString([]byte(`{"uid":1,"exp":9999999999,"typ":"access"}`))
	if _, err := VerifyAccessToken(parts[0] + "." + forged + "." + parts[2]); err == nil {
		t.Errorf("payload yang dipalsukan harus ditolak")
	}

	// Token kedaluwarsa.
	old, _ := generateJWT(7, "access", -time.Minute)
	if _, err := VerifyAccessToken(old); err == nil {
		t.Errorf("token kedaluwarsa harus ditolak")
	}

	// Jenis token salah: refresh tidak boleh dipakai sebagai akses.
	ref, _ := generateJWT(7, "refresh", time.Hour)
	if _, err := VerifyAccessToken(ref); err == nil {
		t.Errorf("token bertipe refresh tidak boleh lolos sebagai akses")
	}

	// alg "none" dan bentuk rusak.
	for _, bad := range []string{"", "a.b", "a.b.c.d", "....", parts[0] + "." + parts[1] + ".", "eyJhbGciOiJub25lIn0." + parts[1] + "."} {
		if _, err := VerifyAccessToken(bad); err == nil {
			t.Errorf("token rusak %q harus ditolak", bad)
		}
	}
}

func TestJWTKeyIsStableWithinProcess(t *testing.T) {
	a, b := jwtKey(), jwtKey()
	if len(a) == 0 || string(a) != string(b) {
		t.Errorf("kunci JWT harus tidak kosong dan stabil selama proses berjalan")
	}
	if sign("x") != sign("x") || sign("x") == sign("y") {
		t.Errorf("sign harus deterministik dan bergantung pada input")
	}
}
