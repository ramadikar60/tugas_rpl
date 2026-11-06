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
// MIND MAP — Section 8
// ============================================================

// MindMapData — struktur data mind map
type MindMapData struct {
	Nodes []MindMapNode `json:"nodes"`
	Edges []MindMapEdge `json:"edges"`
}

type MindMapNode struct {
	ID        string  `json:"id"`
	ParentID  string  `json:"parent_id,omitempty"`
	Text      string  `json:"text"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Color     string  `json:"color,omitempty"`
	Collapsed bool    `json:"collapsed,omitempty"`
}

type MindMapEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ListMindMaps
func ListMindMaps(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	rows, err := config.DB.Query(`
		SELECT id, name, note_id, is_favorite, created_at, updated_at
		FROM mind_maps WHERE user_id = ?
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
		var noteID sql.NullInt64
		var isFavorite bool
		var createdAt, updatedAt time.Time

		_ = rows.Scan(&id, &name, &noteID, &isFavorite, &createdAt, &updatedAt)

		item := map[string]interface{}{
			"id":          id,
			"name":        name,
			"is_favorite": isFavorite,
			"created_at":  createdAt,
			"updated_at":  updatedAt,
		}
		if noteID.Valid {
			item["note_id"] = noteID.Int64
		}
		result = append(result, item)
	}

	writeJSON(w, 200, result)
}

// GetMindMap
func GetMindMap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var name string
	var noteID sql.NullInt64
	var dataJSON sql.NullString
	var isFavorite bool
	var createdAt, updatedAt time.Time

	err := config.DB.QueryRow(`
		SELECT name, note_id, data, is_favorite, created_at, updated_at
		FROM mind_maps WHERE id = ? AND user_id = ?`,
		id, userID).
		Scan(&name, &noteID, &dataJSON, &isFavorite, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		writeJSON(w, 404, map[string]string{"error": "Mind map tidak ditemukan"})
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
	if noteID.Valid {
		result["note_id"] = noteID.Int64
	}

	// Parse data JSON
	if dataJSON.Valid && dataJSON.String != "" {
		var data MindMapData
		if err := json.Unmarshal([]byte(dataJSON.String), &data); err == nil {
			result["data"] = data
		}
	} else {
		// Default: single root node
		result["data"] = MindMapData{
			Nodes: []MindMapNode{
				{ID: "root", Text: "Central Idea", X: 0, Y: 0, Color: "#6366f1"},
			},
			Edges: []MindMapEdge{},
		}
	}

	writeJSON(w, 200, result)
}

// CreateMindMap
func CreateMindMap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Name   string `json:"name"`
		NoteID *int64 `json:"note_id"`
	}
	if err := readJSONMax(r, &req, bigJSONLimit); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		req.Name = "Mind Map Baru"
	}

	// Default data: single root node
	defaultData := MindMapData{
		Nodes: []MindMapNode{
			{ID: "root", Text: req.Name, X: 0, Y: 0, Color: "#6366f1"},
		},
		Edges: []MindMapEdge{},
	}
	dataBytes, _ := json.Marshal(defaultData)

	res, err := config.DB.Exec(`
		INSERT INTO mind_maps (user_id, name, note_id, data)
		VALUES (?, ?, ?, ?)`,
		userID, req.Name, req.NoteID, string(dataBytes))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": id, "name": req.Name})
}

// UpdateMindMap
func UpdateMindMap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Name       *string      `json:"name"`
		Data       *MindMapData `json:"data"`
		IsFavorite *bool        `json:"is_favorite"`
	}
	if err := readJSONMax(r, &req, bigJSONLimit); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE mind_maps SET "
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
	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

// DeleteMindMap
func DeleteMindMap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	_, _ = config.DB.Exec("DELETE FROM mind_maps WHERE id=? AND user_id=?", id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// GenerateFromNote — Note-to-Mind Map
// Parse note content & buat struktur mind map otomatis
func GenerateFromNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		NoteID int64 `json:"note_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	// Ambil note
	var title, content string
	err := config.DB.QueryRow(`
		SELECT title, COALESCE(content,'') FROM notes
		WHERE id = ? AND user_id = ?`, req.NoteID, userID).
		Scan(&title, &content)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "Note tidak ditemukan"})
		return
	}

	// Parse HTML: extract headings & list items
	nodes := []MindMapNode{
		{ID: "root", Text: title, X: 0, Y: 0, Color: "#6366f1"},
	}
	edges := []MindMapEdge{}

	// Extract heading tags & list items
	lines := extractStructure(content)
	parentID := "root"
	angleStep := 360.0 / float64(max(1, len(lines)))

	for i, line := range lines {
		nodeID := "node-" + intToStr(int64(i+1))
		angle := float64(i) * angleStep * 3.14159 / 180.0
		radius := 200.0

		color := "#0ea5e9"
		if line.Level == 2 {
			color = "#16a34a"
		} else if line.Level == 3 {
			color = "#f59e0b"
		}

		nodes = append(nodes, MindMapNode{
			ID:    nodeID,
			Text:  line.Text,
			X:     radius * cos(angle),
			Y:     radius * sin(angle),
			Color: color,
		})

		// Kalau level 2+, parent adalah heading level di atasnya
		edges = append(edges, MindMapEdge{From: parentID, To: nodeID})
	}

	dataBytes, _ := json.Marshal(MindMapData{Nodes: nodes, Edges: edges})

	res, err := config.DB.Exec(`
		INSERT INTO mind_maps (user_id, name, note_id, data)
		VALUES (?, ?, ?, ?)`,
		userID, title+" (Mind Map)", req.NoteID, string(dataBytes))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	writeJSON(w, 201, map[string]interface{}{
		"id":    id,
		"name":  title + " (Mind Map)",
		"nodes": len(nodes),
	})
}

type structureLine struct {
	Level int
	Text  string
}

// extractStructure — extract headings & list dari HTML
func extractStructure(html string) []structureLine {
	result := []structureLine{}

	// Simple extractor: cari <h1>, <h2>, <h3>, <li>
	lines := strings.Split(html, "<")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		level := 0

		if strings.HasPrefix(line, "h1") {
			level = 1
		} else if strings.HasPrefix(line, "h2") {
			level = 2
		} else if strings.HasPrefix(line, "h3") {
			level = 3
		} else if strings.HasPrefix(line, "li") {
			level = 2
		} else {
			continue
		}

		// Extract text between > and <
		start := strings.Index(line, ">")
		end := strings.Index(line, "</")
		if start < 0 {
			continue
		}

		text := line
		if end > start {
			text = line[start+1 : end]
		} else {
			text = line[start+1:]
		}

		text = stripHTML(text)
		text = strings.TrimSpace(text)

		if text != "" && len(text) < 200 {
			result = append(result, structureLine{Level: level, Text: text})
		}

		if len(result) >= 20 {
			break
		}
	}

	return result
}

// Helper math (Go standard library doesn't have cos/sin for this simple use)
func cos(x float64) float64 {
	// Taylor series approximation (cukup akurat untuk 0-2π)
	x = x - 6.283185307179586*float64(int(x/6.283185307179586))
	if x > 3.14159265358979 {
		x -= 6.283185307179586
	}
	if x < -3.14159265358979 {
		x += 6.283185307179586
	}

	result := 1.0
	term := 1.0
	for i := 1; i < 10; i++ {
		term *= -x * x / float64((2*i-1)*(2*i))
		result += term
	}
	return result
}

func sin(x float64) float64 {
	return cos(x - 1.5707963267948966)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
