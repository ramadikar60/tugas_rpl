package handlers

// ============================================================
// MAIL.GO - Pengiriman email reset sandi (Prompt 22A)
//
// Env:
//   SMTP_HOST, SMTP_PORT (bawaan 587), SMTP_USER, SMTP_PASS, SMTP_FROM
//   APP_BASE_URL (bawaan http://localhost:<PORT>), dipakai membentuk tautan reset
//
// Port 587 memakai STARTTLS (bila server menawarkan); port 465 memakai TLS langsung.
// Autentikasi (SMTP_USER) hanya dikirim lewat koneksi terenkripsi, kecuali ke localhost.
//
// Bila SMTP_HOST kosong (mode pengembangan), tautan reset hanya ditulis ke log server.
// ============================================================

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strings"
	"time"

	"notebook-app/config"
)

var errSMTPNotConfigured = errors.New("SMTP belum diatur")

// sendResetMail dapat diganti di tes.
var sendResetMail = defaultSendResetMail

func appBaseURL() string {
	if v := strings.TrimRight(strings.TrimSpace(config.GetEnv("APP_BASE_URL", "")), "/"); v != "" {
		return v
	}
	return "http://localhost:" + config.GetEnv("PORT", "8080")
}

// resetLink: token ditaruh di fragmen (#) agar tidak ikut terkirim ke server
// (log akses) maupun ke situs lain lewat header Referer.
func resetLink(rawToken string) string {
	return appBaseURL() + "/#reset=" + rawToken
}

func defaultSendResetMail(to, rawToken string) {
	link := resetLink(rawToken)
	subject := "Atur ulang password Notebook"
	body := "Halo,\r\n\r\n" +
		"Kami menerima permintaan untuk mengatur ulang password akun Notebook Anda.\r\n" +
		"Buka tautan berikut dalam 1 jam (hanya berlaku sekali):\r\n\r\n" +
		link + "\r\n\r\n" +
		"Jika Anda tidak meminta ini, abaikan email ini; password Anda tidak berubah.\r\n"

	err := sendMail(to, subject, body)
	if errors.Is(err, errSMTPNotConfigured) {
		log.Printf("[reset] SMTP belum diatur (mode pengembangan). Tautan reset untuk %s (hanya tampil di log server): %s", to, link)
		return
	}
	if err != nil {
		// Tidak mencatat token atau isi email.
		log.Printf("[reset] gagal mengirim email reset ke %s: %v", to, err)
	}
}

func hasCRLF(s string) bool { return strings.ContainsAny(s, "\r\n") }

func sendMail(to, subject, body string) error {
	host := strings.TrimSpace(config.GetEnv("SMTP_HOST", ""))
	if host == "" {
		return errSMTPNotConfigured
	}
	port := strings.TrimSpace(config.GetEnv("SMTP_PORT", "587"))
	user := config.GetEnv("SMTP_USER", "")
	pass := config.GetEnv("SMTP_PASS", "")
	from := strings.TrimSpace(config.GetEnv("SMTP_FROM", user))

	// Cegah injeksi header lewat alamat atau subjek.
	if hasCRLF(to) || hasCRLF(from) || hasCRLF(subject) || to == "" || from == "" {
		return errors.New("alamat atau subjek email tidak valid")
	}

	msg := "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\n" +
		"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		wrapBase64(base64.StdEncoding.EncodeToString([]byte(body)))

	addr := net.JoinHostPort(host, port)
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	var conn net.Conn
	var err error
	if port == "465" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host})
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()

	if port != "465" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return err
			}
		}
	}
	if user != "" {
		// PlainAuth menolak mengirim sandi lewat koneksi tak terenkripsi (kecuali localhost).
		if err := c.Auth(smtp.PlainAuth("", user, pass, host)); err != nil {
			return err
		}
	}
	// Amplop SMTP butuh alamat polos; header From boleh memakai "Nama <alamat>".
	envelopeFrom := from
	if a, perr := netmail.ParseAddress(from); perr == nil {
		envelopeFrom = a.Address
	}
	if err := c.Mail(envelopeFrom); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func wrapBase64(s string) string {
	var b strings.Builder
	for len(s) > 76 {
		b.WriteString(s[:76])
		b.WriteString("\r\n")
		s = s[76:]
	}
	b.WriteString(s)
	b.WriteString("\r\n")
	return b.String()
}
