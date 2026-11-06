package handlers

import (
	"net/http"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ============ LIST NOTIFICATIONS ============
func ListNotifications(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	unreadOnly := r.URL.Query().Get("unread") == "true"

	query := `SELECT id, user_id, type, title, COALESCE(message,''),
	          COALESCE(link,''), is_read, created_at
	          FROM notifications WHERE user_id = ?`
	if unreadOnly {
		query += " AND is_read = FALSE"
	}
	query += " ORDER BY created_at DESC LIMIT 50"

	rows, err := config.DB.Query(query, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, uid int64
		var typ, title, msg, link string
		var isRead bool
		var created time.Time
		_ = rows.Scan(&id, &uid, &typ, &title, &msg, &link, &isRead, &created)
		result = append(result, map[string]interface{}{
			"id":         id,
			"type":       typ,
			"title":      title,
			"message":    msg,
			"link":       link,
			"is_read":    isRead,
			"created_at": created,
		})
	}
	writeJSON(w, 200, result)
}

// ============ UNREAD COUNT ============
func UnreadCount(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	var count int
	_ = config.DB.QueryRow(
		"SELECT COUNT(*) FROM notifications WHERE user_id=? AND is_read=FALSE",
		userID).Scan(&count)
	writeJSON(w, 200, map[string]int{"count": count})
}

// ============ MARK READ ============
func MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	_, _ = config.DB.Exec(
		"UPDATE notifications SET is_read=TRUE WHERE id=? AND user_id=?",
		id, userID)
	writeJSON(w, 200, map[string]string{"message": "OK"})
}

// ============ MARK ALL READ ============
func MarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	_, _ = config.DB.Exec(
		"UPDATE notifications SET is_read=TRUE WHERE user_id=? AND is_read=FALSE",
		userID)
	writeJSON(w, 200, map[string]string{"message": "OK"})
}

// ============ DELETE NOTIFICATION ============
func DeleteNotification(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	_, _ = config.DB.Exec(
		"DELETE FROM notifications WHERE id=? AND user_id=?", id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// ============ CLEAR ALL ============
func ClearNotifications(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	_, _ = config.DB.Exec("DELETE FROM notifications WHERE user_id=?", userID)
	writeJSON(w, 200, map[string]string{"message": "Cleared"})
}

// ============ CREATE NOTIFICATION (helper) ============
func CreateNotification(userID int64, typ, title, message, link string) {
	_, _ = config.DB.Exec(`
		INSERT INTO notifications (user_id, type, title, message, link)
		VALUES (?, ?, ?, ?, ?)`,
		userID, typ, title, message, link)
}
