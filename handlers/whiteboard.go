package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ============================================================
// WHITEBOARD — Section 9
// ============================================================

type WhiteboardData struct {
	Elements []WhiteboardElement `json:"elements"`
	Viewport WhiteboardViewport  `json:"viewport"`
}

type WhiteboardElement struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"` // path | rect | circle | text | arrow | sticky | image | connector
	X        float64                `json:"x"`
	Y        float64                `json:"y"`
	Width    float64                `json:"width,omitempty"`
	Height   float64                `json:"height,omitempty"`
	Points   []Point                `json:"points,omitempty"` // untuk path
	Text     string                 `json:"text,omitempty"`
	Color    string                 `json:"color,omitempty"`
	Fill     string                 `json:"fill,omitempty"`
	Stroke   float64                `json:"stroke,omitempty"`
	FontSize float64                `json:"font_size,omitempty"`
	Rotation float64                `json:"rotation,omitempty"`
	ImageURL string                 `json:"image_url,omitempty"`
	Extra    map[string]interface{} `json:"extra,omitempty"`
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type WhiteboardViewport struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

// ListWhiteboards
func ListWhiteboards(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	rows, err := config.DB.Query(`
		SELECT id, name, project_id, thumbnail, is_favorite, created_at, updated_at
		FROM whiteboards WHERE user_id = ?
		ORDER BY is_favorite DESC, updated_at DESC`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name string
		var projectID sql.NullInt64
		var thumbnail sql.NullString
		var isFavorite bool
		var createdAt, updatedAt time.Time

		_ = rows.Scan(&id, &name, &projectID, &thumbnail, &isFavorite, &createdAt, &updatedAt)

		item := map[string]interface{}{
			"id":          id,
			"name":        name,
			"is_favorite": isFavorite,
			"created_at":  createdAt,
			"updated_at":  updatedAt,
		}
		if projectID.Valid {
			item["project_id"] = projectID.Int64
		}
		if thumbnail.Valid && thumbnail.String != "" {
			item["thumbnail"] = thumbnail.String
		}
		result = append(result, item)
	}

	writeJSON(w, 200, result)
}

// GetWhiteboard
func GetWhiteboard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var name string
	var projectID sql.NullInt64
	var dataJSON sql.NullString
	var thumbnail sql.NullString
	var isFavorite bool
	var createdAt, updatedAt time.Time

	err := config.DB.QueryRow(`
		SELECT name, project_id, data, thumbnail, is_favorite, created_at, updated_at
		FROM whiteboards WHERE id = ? AND user_id = ?`,
		id, userID).
		Scan(&name, &projectID, &dataJSON, &thumbnail, &isFavorite, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		writeJSON(w, 404, map[string]string{"error": "Whiteboard tidak ditemukan"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	result := map[string]interface{}{
		"id":          id,
		"name":        name,
		"is_favorite": isFavorite,
		"created_at":  createdAt,
		"updated_at":  updatedAt,
	}
	if projectID.Valid {
		result["project_id"] = projectID.Int64
	}
	if thumbnail.Valid {
		result["thumbnail"] = thumbnail.String
	}

	if dataJSON.Valid && dataJSON.String != "" {
		var data WhiteboardData
		if err := json.Unmarshal([]byte(dataJSON.String), &data); err == nil {
			result["data"] = data
		}
	} else {
		result["data"] = WhiteboardData{
			Elements: []WhiteboardElement{},
			Viewport: WhiteboardViewport{X: 0, Y: 0, Zoom: 1},
		}
	}

	writeJSON(w, 200, result)
}

// CreateWhiteboard
func CreateWhiteboard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Name      string `json:"name"`
		ProjectID *int64 `json:"project_id"`
	}
	if err := readJSONMax(r, &req, bigJSONLimit); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		req.Name = "Whiteboard Baru"
	}

	defaultData := WhiteboardData{
		Elements: []WhiteboardElement{},
		Viewport: WhiteboardViewport{X: 0, Y: 0, Zoom: 1},
	}
	dataBytes, _ := json.Marshal(defaultData)

	res, err := config.DB.Exec(`
		INSERT INTO whiteboards (user_id, name, project_id, data)
		VALUES (?, ?, ?, ?)`,
		userID, req.Name, req.ProjectID, string(dataBytes))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": id, "name": req.Name})
}

// UpdateWhiteboard
func UpdateWhiteboard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Name       *string         `json:"name"`
		Data       *WhiteboardData `json:"data"`
		Thumbnail  *string         `json:"thumbnail"`
		IsFavorite *bool           `json:"is_favorite"`
	}
	if err := readJSONMax(r, &req, bigJSONLimit); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE whiteboards SET "
	args := []interface{}{}

	if req.Name != nil {
		query += "name=?, "
		args = append(args, *req.Name)
	}
	if req.Data != nil {
		dataBytes, _ := json.Marshal(req.Data)
		query += "data=?, "
		args = append(args, string(dataBytes))
	}
	if req.Thumbnail != nil {
		query += "thumbnail=?, "
		args = append(args, *req.Thumbnail)
	}
	if req.IsFavorite != nil {
		query += "is_favorite=?, "
		args = append(args, *req.IsFavorite)
	}

	if len(args) == 0 {
		writeJSON(w, 400, map[string]string{"error": "Tidak ada field"})
		return
	}

	query = query[:len(query)-2] + " WHERE id=? AND user_id=?"
	args = append(args, id, userID)

	_, err := config.DB.Exec(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Saved"})
}

// DeleteWhiteboard
func DeleteWhiteboard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	_, _ = config.DB.Exec("DELETE FROM whiteboards WHERE id=? AND user_id=?", id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}
