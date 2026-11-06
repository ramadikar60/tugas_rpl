package handlers

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"notebook-app/config"
	"notebook-app/middleware"
)

// ============================================================
// KANBAN BOARDS — Section 7
// ============================================================

// ListBoards — daftar board user
func ListBoards(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	rows, err := config.DB.Query(`
		SELECT id, name, COALESCE(description,''), project_id, is_favorite,
		       created_at, updated_at
		FROM kanban_boards WHERE user_id = ?
		ORDER BY is_favorite DESC, updated_at DESC`, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name, description string
		var projectID sql.NullInt64
		var isFavorite bool
		var createdAt, updatedAt time.Time

		_ = rows.Scan(&id, &name, &description, &projectID, &isFavorite,
			&createdAt, &updatedAt)

		item := map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
			"is_favorite": isFavorite,
			"created_at":  createdAt,
			"updated_at":  updatedAt,
		}
		if projectID.Valid {
			item["project_id"] = projectID.Int64
		}

		// Count columns & cards
		var colCount, cardCount int
		_ = config.DB.QueryRow(
			"SELECT COUNT(*) FROM kanban_columns WHERE board_id=?", id).Scan(&colCount)
		_ = config.DB.QueryRow(`
			SELECT COUNT(*) FROM kanban_cards c
			JOIN kanban_columns col ON col.id = c.column_id
			WHERE col.board_id = ?`, id).Scan(&cardCount)

		item["column_count"] = colCount
		item["card_count"] = cardCount

		result = append(result, item)
	}

	writeJSON(w, 200, result)
}

// GetBoard — detail board dengan columns & cards
func GetBoard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var name, description string
	var projectID sql.NullInt64
	var isFavorite bool
	var createdAt, updatedAt time.Time

	err := config.DB.QueryRow(`
		SELECT name, COALESCE(description,''), project_id, is_favorite,
		       created_at, updated_at
		FROM kanban_boards WHERE id = ? AND user_id = ?`,
		id, userID).
		Scan(&name, &description, &projectID, &isFavorite, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		writeJSON(w, 404, map[string]string{"error": "Board tidak ditemukan"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	board := map[string]interface{}{
		"id":          id,
		"name":        name,
		"description": description,
		"is_favorite": isFavorite,
		"created_at":  createdAt,
		"updated_at":  updatedAt,
	}
	if projectID.Valid {
		board["project_id"] = projectID.Int64
	}

	// Columns
	colRows, _ := config.DB.Query(`
		SELECT id, name, color, position
		FROM kanban_columns WHERE board_id = ?
		ORDER BY position ASC, id ASC`, id)
	columns := []map[string]interface{}{}
	if colRows != nil {
		defer colRows.Close()
		for colRows.Next() {
			var colID int64
			var colName, color string
			var position int
			_ = colRows.Scan(&colID, &colName, &color, &position)

			col := map[string]interface{}{
				"id":       colID,
				"name":     colName,
				"color":    color,
				"position": position,
			}

			// Cards in this column
			cardRows, _ := config.DB.Query(`
				SELECT c.id, c.title, COALESCE(c.description,''), c.card_type,
				       c.task_id, c.note_id, COALESCE(c.labels,''),
				       c.assignee_id, c.priority, c.due_date, c.position,
				       COALESCE(u.username,'') AS assignee_name
				FROM kanban_cards c
				LEFT JOIN users u ON u.id = c.assignee_id
				WHERE c.column_id = ?
				ORDER BY c.position ASC, c.id ASC`, colID)
			cards := []map[string]interface{}{}
			if cardRows != nil {
				for cardRows.Next() {
					var cardID int64
					var title, description, cardType, labels, priority, assigneeName string
					var taskID, noteID, assigneeID sql.NullInt64
					var dueDate sql.NullTime
					var position int

					_ = cardRows.Scan(&cardID, &title, &description, &cardType,
						&taskID, &noteID, &labels, &assigneeID, &priority,
						&dueDate, &position, &assigneeName)

					card := map[string]interface{}{
						"id":            cardID,
						"title":         title,
						"description":   description,
						"card_type":     cardType,
						"labels":        labels,
						"priority":      priority,
						"position":      position,
						"assignee_name": assigneeName,
					}
					if taskID.Valid {
						card["task_id"] = taskID.Int64
					}
					if noteID.Valid {
						card["note_id"] = noteID.Int64
					}
					if assigneeID.Valid {
						card["assignee_id"] = assigneeID.Int64
					}
					if dueDate.Valid {
						card["due_date"] = dueDate.Time
					}
					cards = append(cards, card)
				}
				cardRows.Close()
			}

			col["cards"] = cards
			columns = append(columns, col)
		}
	}
	board["columns"] = columns

	writeJSON(w, 200, board)
}

// CreateBoard
func CreateBoard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		ProjectID   *int64 `json:"project_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "Nama board wajib diisi"})
		return
	}

	res, err := config.DB.Exec(`
		INSERT INTO kanban_boards (user_id, name, description, project_id)
		VALUES (?, ?, ?, ?)`,
		userID, req.Name, req.Description, req.ProjectID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	boardID, _ := res.LastInsertId()

	// Buat 3 kolom default
	defaultCols := []struct {
		Name  string
		Color string
	}{
		{"To Do", "#9ca3af"},
		{"In Progress", "#0ea5e9"},
		{"Done", "#16a34a"},
	}

	for i, col := range defaultCols {
		_, _ = config.DB.Exec(`
			INSERT INTO kanban_columns (board_id, name, color, position)
			VALUES (?, ?, ?, ?)`,
			boardID, col.Name, col.Color, i)
	}

	writeJSON(w, 201, map[string]interface{}{
		"id":   boardID,
		"name": req.Name,
	})
}

// UpdateBoard
func UpdateBoard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		IsFavorite  *bool   `json:"is_favorite"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE kanban_boards SET "
	args := []interface{}{}
	if req.Name != nil {
		query += "name=?, "
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		query += "description=?, "
		args = append(args, *req.Description)
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

	_, _ = config.DB.Exec(query, args...)
	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

// DeleteBoard
func DeleteBoard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	_, _ = config.DB.Exec("DELETE FROM kanban_boards WHERE id=? AND user_id=?", id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// ============================================================
// COLUMNS
// ============================================================

// CreateColumn
func CreateColumn(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	boardID, _ := pathInt(r, "id")

	// Verifikasi ownership
	var count int
	_ = config.DB.QueryRow(
		"SELECT COUNT(*) FROM kanban_boards WHERE id=? AND user_id=?",
		boardID, userID).Scan(&count)
	if count == 0 {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}

	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "Nama kolom wajib diisi"})
		return
	}
	if req.Color == "" {
		req.Color = "#6366f1"
	}

	// Get next position
	var maxPos sql.NullInt64
	_ = config.DB.QueryRow(
		"SELECT MAX(position) FROM kanban_columns WHERE board_id=?",
		boardID).Scan(&maxPos)
	pos := 0
	if maxPos.Valid {
		pos = int(maxPos.Int64) + 1
	}

	res, err := config.DB.Exec(`
		INSERT INTO kanban_columns (board_id, name, color, position)
		VALUES (?, ?, ?, ?)`,
		boardID, req.Name, req.Color, pos)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"id": id, "name": req.Name})
}

// UpdateColumn
func UpdateColumn(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Name     *string `json:"name"`
		Color    *string `json:"color"`
		Position *int    `json:"position"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	// Verifikasi ownership via join
	var count int
	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM kanban_columns c
		JOIN kanban_boards b ON b.id = c.board_id
		WHERE c.id = ? AND b.user_id = ?`,
		id, userID).Scan(&count)
	if count == 0 {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}

	query := "UPDATE kanban_columns SET "
	args := []interface{}{}
	if req.Name != nil {
		query += "name=?, "
		args = append(args, *req.Name)
	}
	if req.Color != nil {
		query += "color=?, "
		args = append(args, *req.Color)
	}
	if req.Position != nil {
		query += "position=?, "
		args = append(args, *req.Position)
	}

	if len(args) == 0 {
		writeJSON(w, 400, map[string]string{"error": "Tidak ada field"})
		return
	}
	query = query[:len(query)-2] + " WHERE id=?"
	args = append(args, id)

	_, _ = config.DB.Exec(query, args...)
	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

// DeleteColumn
func DeleteColumn(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	_, _ = config.DB.Exec(`
		DELETE c FROM kanban_columns c
		JOIN kanban_boards b ON b.id = c.board_id
		WHERE c.id = ? AND b.user_id = ?`,
		id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}

// ============================================================
// CARDS
// ============================================================

// CreateCard
func CreateCard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	columnID, _ := pathInt(r, "id")

	// Verifikasi ownership
	var count int
	_ = config.DB.QueryRow(`
		SELECT COUNT(*) FROM kanban_columns c
		JOIN kanban_boards b ON b.id = c.board_id
		WHERE c.id = ? AND b.user_id = ?`,
		columnID, userID).Scan(&count)
	if count == 0 {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}

	var req struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		CardType    string  `json:"card_type"`
		TaskID      *int64  `json:"task_id"`
		NoteID      *int64  `json:"note_id"`
		Labels      string  `json:"labels"`
		AssigneeID  *int64  `json:"assignee_id"`
		Priority    string  `json:"priority"`
		DueDate     *string `json:"due_date"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Title) == "" {
		writeJSON(w, 400, map[string]string{"error": "Judul card wajib diisi"})
		return
	}

	if req.CardType == "" {
		req.CardType = "custom"
	}
	if req.Priority == "" {
		req.Priority = "medium"
	}

	// Get next position
	var maxPos sql.NullInt64
	_ = config.DB.QueryRow(
		"SELECT MAX(position) FROM kanban_cards WHERE column_id=?",
		columnID).Scan(&maxPos)
	pos := 0
	if maxPos.Valid {
		pos = int(maxPos.Int64) + 1
	}

	var dueArg interface{}
	if req.DueDate != nil && *req.DueDate != "" {
		if t, err := parseDateTimeInput(*req.DueDate); err == nil {
			dueArg = t
		}
	}

	res, err := config.DB.Exec(`
		INSERT INTO kanban_cards 
		    (column_id, user_id, title, description, card_type, task_id, note_id,
		     labels, assignee_id, priority, due_date, position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		columnID, userID, req.Title, req.Description, req.CardType,
		req.TaskID, req.NoteID, req.Labels, req.AssigneeID, req.Priority,
		dueArg, pos)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	writeJSON(w, 201, map[string]interface{}{"id": id, "title": req.Title})
}

// UpdateCard
func UpdateCard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Labels      *string `json:"labels"`
		AssigneeID  *int64  `json:"assignee_id"`
		Priority    *string `json:"priority"`
		DueDate     *string `json:"due_date"`
		Position    *int    `json:"position"`
		ColumnID    *int64  `json:"column_id"` // untuk move antar kolom
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	query := "UPDATE kanban_cards SET "
	args := []interface{}{}

	if req.Title != nil {
		query += "title=?, "
		args = append(args, *req.Title)
	}
	if req.Description != nil {
		query += "description=?, "
		args = append(args, *req.Description)
	}
	if req.Labels != nil {
		query += "labels=?, "
		args = append(args, *req.Labels)
	}
	if req.AssigneeID != nil {
		query += "assignee_id=?, "
		args = append(args, *req.AssigneeID)
	}
	if req.Priority != nil {
		query += "priority=?, "
		args = append(args, *req.Priority)
	}
	if req.DueDate != nil {
		if *req.DueDate == "" {
			query += "due_date=NULL, "
		} else {
			if t, err := parseDateTimeInput(*req.DueDate); err == nil {
				query += "due_date=?, "
				args = append(args, t)
			}
		}
	}
	if req.Position != nil {
		query += "position=?, "
		args = append(args, *req.Position)
	}
	if req.ColumnID != nil {
		query += "column_id=?, "
		args = append(args, *req.ColumnID)
	}

	if len(args) == 0 {
		writeJSON(w, 400, map[string]string{"error": "Tidak ada field"})
		return
	}
	query = strings.TrimSuffix(query, ", ")
	query += " WHERE id=? AND user_id=?"
	args = append(args, id, userID)

	_, err := config.DB.Exec(query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Updated"})
}

// MoveCard — drag & drop card ke kolom lain dengan posisi baru
func MoveCard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")

	var req struct {
		ColumnID int64 `json:"column_id"`
		Position int   `json:"position"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request"})
		return
	}

	_, err := config.DB.Exec(`
		UPDATE kanban_cards SET column_id = ?, position = ?
		WHERE id = ? AND user_id = ?`,
		req.ColumnID, req.Position, id, userID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Moved"})
}

// DeleteCard
func DeleteCard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id, _ := pathInt(r, "id")
	_, _ = config.DB.Exec("DELETE FROM kanban_cards WHERE id=? AND user_id=?", id, userID)
	writeJSON(w, 200, map[string]string{"message": "Deleted"})
}
