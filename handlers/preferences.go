package handlers

import (
	"net/http"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ============ GET PREFERENCES ============
func GetPreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	// Insert default kalau belum ada
	_, _ = config.DB.Exec(
		"INSERT IGNORE INTO user_preferences (user_id) VALUES (?)", userID)

	var prefs struct {
		Theme            string `json:"theme"`
		AccentColor      string `json:"accent_color"`
		EditorWidth      string `json:"editor_width"`
		FontSize         int    `json:"font_size"`
		SidebarCollapsed bool   `json:"sidebar_collapsed"`
		CompactMode      bool   `json:"compact_mode"`
		Language         string `json:"language"`
	}
	err := config.DB.QueryRow(`
		SELECT theme, accent_color, editor_width, font_size,
		       sidebar_collapsed, compact_mode, language
		FROM user_preferences WHERE user_id = ?`, userID).
		Scan(&prefs.Theme, &prefs.AccentColor, &prefs.EditorWidth,
			&prefs.FontSize, &prefs.SidebarCollapsed, &prefs.CompactMode, &prefs.Language)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, prefs)
}

// ============ SAVE PREFERENCES ============
func SavePreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Theme            *string `json:"theme"`
		AccentColor      *string `json:"accent_color"`
		EditorWidth      *string `json:"editor_width"`
		FontSize         *int    `json:"font_size"`
		SidebarCollapsed *bool   `json:"sidebar_collapsed"`
		CompactMode      *bool   `json:"compact_mode"`
		Language         *string `json:"language"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	// Insert kalau belum ada
	_, _ = config.DB.Exec(
		"INSERT IGNORE INTO user_preferences (user_id) VALUES (?)", userID)

	query := "UPDATE user_preferences SET "
	args := []interface{}{}
	add := func(frag string, v interface{}) {
		query += frag
		args = append(args, v)
	}
	if req.Theme != nil {
		add("theme=?, ", *req.Theme)
	}
	if req.AccentColor != nil {
		add("accent_color=?, ", *req.AccentColor)
	}
	if req.EditorWidth != nil {
		add("editor_width=?, ", *req.EditorWidth)
	}
	if req.FontSize != nil {
		add("font_size=?, ", *req.FontSize)
	}
	if req.SidebarCollapsed != nil {
		add("sidebar_collapsed=?, ", *req.SidebarCollapsed)
	}
	if req.CompactMode != nil {
		add("compact_mode=?, ", *req.CompactMode)
	}
	if req.Language != nil {
		add("language=?, ", *req.Language)
	}

	if len(args) == 0 {
		writeJSON(w, 200, map[string]string{"message": "No changes"})
		return
	}

	query = query[:len(query)-2] + " WHERE user_id = ?"
	args = append(args, userID)

	_, err := config.DB.Exec(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Saved"})
}
