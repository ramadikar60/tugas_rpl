package handlers

import (
	"log"

	"notebook-app/auth"
	"notebook-app/config"
)

// SeedAdmin — buat admin default kalau belum ada user admin sama sekali
// Dipanggil sekali di main.go setelah InitDB()
func SeedAdmin() {
	// Cek apakah sudah ada user dengan role admin
	var adminCount int
	err := config.DB.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminCount)
	if err != nil {
		log.Printf("SeedAdmin: gagal cek admin - %v", err)
		return
	}
	if adminCount > 0 {
		log.Printf("Admin sudah ada (%d user), skip seed", adminCount)
		return
	}

	// Kredensial dari .env. Tidak ada sandi bawaan di kode.
	email := config.GetEnv("ADMIN_EMAIL", "admin@notebook.local")
	username := config.GetEnv("ADMIN_USERNAME", "admin")
	password := config.GetEnv("ADMIN_PASSWORD", "")

	if password == "" {
		log.Printf("Belum ada akun admin dan ADMIN_PASSWORD belum diatur di .env, jadi admin TIDAK dibuat. " +
			"Isi ADMIN_EMAIL, ADMIN_USERNAME, dan ADMIN_PASSWORD (minimal 10 karakter), lalu jalankan ulang server.")
		return
	}
	if err := auth.ValidateAccountPassword(password, email, username); err != nil {
		log.Printf("SeedAdmin: ADMIN_PASSWORD ditolak (%v). Admin tidak dibuat.", err)
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		log.Printf("SeedAdmin: hash gagal - %v", err)
		return
	}

	_, err = config.DB.Exec(`
		INSERT INTO users (email, username, password_hash, role, is_active)
		VALUES (?, ?, ?, 'admin', TRUE)`,
		email, username, hash)
	if err != nil {
		log.Printf("SeedAdmin: insert gagal - %v", err)
		return
	}

	// Sandi tidak pernah ditulis ke log.
	log.Printf("Akun admin dibuat: %s. Hapus ADMIN_PASSWORD dari .env sekarang.", email)
}
