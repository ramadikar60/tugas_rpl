package auth

// ============================================================
// PASSWORD_POLICY.GO - Kebijakan sandi AKUN (Prompt 22A)
//
// Dipakai di: registrasi, reset sandi, ganti sandi, dan pembuatan/pengubahan
// akun oleh admin. Tidak dipakai untuk sandi catatan terkunci atau sandi berbagi.
// ============================================================

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	MinAccountPasswordLen = 10
	MaxAccountPasswordLen = 72 // byte: batas bcrypt
)

// Kata dasar yang terlalu umum. Dicocokkan terhadap sandi utuh DAN terhadap sandi
// tanpa angka/simbol di ujungnya ("Password123!" -> "password").
var commonPasswordBases = map[string]bool{
	"password": true, "passw0rd": true, "pass": true, "passwort": true, "katasandi": true, "kata sandi": true,
	"qwerty": true, "qwertyuiop": true, "qwertyuiopasdfghjkl": true, "asdfghjkl": true, "zxcvbnm": true,
	"asdfgh": true, "qazwsx": true, "qazwsxedc": true, "1q2w3e4r": true, "1q2w3e4r5t": true,
	"letmein": true, "welcome": true, "admin": true, "administrator": true, "root": true, "login": true,
	"master": true, "dragon": true, "monkey": true, "shadow": true, "sunshine": true, "princess": true,
	"football": true, "baseball": true, "superman": true, "batman": true, "trustno1": true, "iloveyou": true,
	"abc": true, "abcdef": true, "abcdefgh": true, "abcd": true, "abc123": true, "test": true, "testing": true,
	"user": true, "guest": true, "default": true, "secret": true, "changeme": true, "notebook": true,
	"sayang": true, "sayangku": true, "bismillah": true, "indonesia": true, "rahasia": true, "kucing": true,
	"anjing": true, "jakarta": true, "bandung": true, "surabaya": true, "garudapancasila": true,
	"cintaku": true, "cinta": true, "selamat": true, "merdeka": true, "mantap": true,
}

// Deretan murni yang mudah ditebak.
var commonSequences = []string{
	"1234567890", "0123456789", "9876543210", "0987654321", "12345678901", "123456789012",
	"1111111111", "0000000000", "abcdefghij", "qwertyuiop", "asdfghjkl;",
}

func trimTrailingDigitsAndSymbols(s string) string {
	return strings.TrimRightFunc(s, func(r rune) bool {
		return (r >= '0' && r <= '9') || strings.ContainsRune("!@#$%^&*()-_=+.,?", r)
	})
}

// ValidateAccountPassword mengembalikan pesan galat (berbahasa Indonesia) atau nil.
// email dan username dipakai untuk menolak sandi yang meniru identitas akun.
func ValidateAccountPassword(password, email, username string) error {
	if utf8.RuneCountInString(password) < MinAccountPasswordLen {
		return errors.New("Password minimal 10 karakter")
	}
	if len(password) > MaxAccountPasswordLen {
		return errors.New("Password maksimal 72 karakter")
	}

	low := strings.ToLower(strings.TrimSpace(password))

	// Terlalu sederhana: hanya satu atau dua karakter berbeda.
	uniq := map[rune]bool{}
	for _, r := range low {
		uniq[r] = true
	}
	if len(uniq) <= 2 {
		return errors.New("Password terlalu sederhana")
	}

	// Terlalu umum.
	if commonPasswordBases[low] || commonPasswordBases[trimTrailingDigitsAndSymbols(low)] {
		return errors.New("Password terlalu umum, pilih yang lebih sulit ditebak")
	}
	for _, seq := range commonSequences {
		if strings.Contains(low, seq) && len(low) <= len(seq)+3 {
			return errors.New("Password terlalu umum, pilih yang lebih sulit ditebak")
		}
	}

	// Meniru identitas akun.
	local := strings.ToLower(email)
	if i := strings.Index(local, "@"); i > 0 {
		local = local[:i]
	}
	for _, ident := range []string{local, strings.ToLower(strings.TrimSpace(username))} {
		if utf8.RuneCountInString(ident) >= 4 && strings.Contains(low, ident) {
			return errors.New("Password tidak boleh mengandung email atau username")
		}
	}
	if low == strings.ToLower(email) {
		return errors.New("Password tidak boleh sama dengan email")
	}
	return nil
}
