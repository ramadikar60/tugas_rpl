package models

import "time"

type User struct {
	ID          int64      `json:"id"`
	Email       string     `json:"email"`
	Username    string     `json:"username"`
	Avatar      string     `json:"avatar,omitempty"`
	Role        string     `json:"role,omitempty"`
	IsActive    bool       `json:"is_active,omitempty"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Notebook struct {
	ID          int64       `json:"id"`
	UserID      int64       `json:"user_id"`
	ParentID    *int64      `json:"parent_id"`
	Name        string      `json:"name"`
	Icon        string      `json:"icon"`
	Color       string      `json:"color"`
	IsPinned    bool        `json:"is_pinned"`
	IsFavorite  bool        `json:"is_favorite"`
	IsCollapsed bool        `json:"is_collapsed"`
	Position    int         `json:"position"`
	Children    []*Notebook `json:"children,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type Note struct {
	ID             int64         `json:"id"`
	UserID         int64         `json:"user_id"`
	NotebookID     *int64        `json:"notebook_id"`
	Title          string        `json:"title"`
	Content        string        `json:"content"`
	ContentText    string        `json:"-"`
	IsPinned       bool          `json:"is_pinned"`
	IsFavorite     bool          `json:"is_favorite"`
	IsArchived     bool          `json:"is_archived"`
	IsTrashed      bool          `json:"is_trashed"`
	IsLocked       bool          `json:"is_locked"`
	IsReadonly     bool          `json:"is_readonly"`
	LockedAt       *time.Time    `json:"locked_at,omitempty"`
	ReadOnlyAt     *time.Time    `json:"read_only_at,omitempty"`
	TrashedAt      *time.Time    `json:"trashed_at,omitempty"`
	HasChecklist   bool          `json:"has_checklist"`
	ChecklistTotal int           `json:"checklist_total"`
	ChecklistDone  int           `json:"checklist_done"`
	WordCount      int           `json:"word_count"`
	CharCount      int           `json:"char_count"`
	SortOrder      int           `json:"sort_order"`
	LastViewedAt   *time.Time    `json:"last_viewed_at,omitempty"`
	Tags           []*Tag        `json:"tags,omitempty"`
	Attachments    []*Attachment `json:"attachments,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type Tag struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Attachment struct {
	ID         int64     `json:"id"`
	NoteID     int64     `json:"note_id"`
	Filename   string    `json:"filename"`
	StoredName string    `json:"stored_name"`
	MimeType   string    `json:"mime_type"`
	Size       int64     `json:"size"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"created_at"`
}

type Task struct {
	ID           int64      `json:"id"`
	UserID       int64      `json:"user_id"`
	NoteID       *int64     `json:"note_id"`
	NotebookID   *int64     `json:"notebook_id"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Status       string     `json:"status"`
	Priority     string     `json:"priority"`
	DueDate      *time.Time `json:"due_date"`
	ReminderAt   *time.Time `json:"reminder_at"`
	ReminderSent bool       `json:"reminder_sent"`
	CompletedAt  *time.Time `json:"completed_at"`
	SortOrder    int        `json:"sort_order"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	NoteTitle    string `json:"note_title,omitempty"`
	NotebookName string `json:"notebook_name,omitempty"`
	IsOverdue    bool   `json:"is_overdue"`
	DaysUntilDue int    `json:"days_until_due"`
}

type Notification struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Link      string    `json:"link"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

type Template struct {
	ID          int64     `json:"id"`
	UserID      *int64    `json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Icon        string    `json:"icon"`
	IconColor   string    `json:"icon_color"`
	Content     string    `json:"content"`
	IsSystem    bool      `json:"is_system"`
	IsFavorite  bool      `json:"is_favorite"`
	UsageCount  int       `json:"usage_count"`
	Tags        string    `json:"tags"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type NoteLink struct {
	ID           int64     `json:"id"`
	SourceNoteID int64     `json:"source_note_id"`
	TargetNoteID int64     `json:"target_note_id"`
	LinkText     string    `json:"link_text"`
	CreatedAt    time.Time `json:"created_at"`
	TargetTitle  string    `json:"target_title,omitempty"`
	SourceTitle  string    `json:"source_title,omitempty"`
}

type Share struct {
	ID               int64      `json:"id"`
	UserID           int64      `json:"user_id"`
	ItemType         string     `json:"item_type"`
	ItemID           int64      `json:"item_id"`
	ShareMode        string     `json:"share_mode"`
	Permission       string     `json:"permission"`
	Token            string     `json:"token,omitempty"`
	SharedWithUserID *int64     `json:"shared_with_user_id"`
	Title            string     `json:"title"`
	ExpiresAt        *time.Time `json:"expires_at"`
	IsActive         bool       `json:"is_active"`
	ViewCount        int        `json:"view_count"`
	CreatedAt        time.Time  `json:"created_at"`

	SharedWithName  string `json:"shared_with_name,omitempty"`
	SharedWithEmail string `json:"shared_with_email,omitempty"`
}

type Comment struct {
	ID         int64     `json:"id"`
	NoteID     int64     `json:"note_id"`
	UserID     int64     `json:"user_id"`
	ParentID   *int64    `json:"parent_id"`
	Content    string    `json:"content"`
	IsResolved bool      `json:"is_resolved"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	Username string     `json:"username,omitempty"`
	Avatar   string     `json:"avatar,omitempty"`
	Replies  []*Comment `json:"replies,omitempty"`
}

type NoteVersion struct {
	ID            int64     `json:"id"`
	NoteID        int64     `json:"note_id"`
	UserID        int64     `json:"user_id"`
	VersionNumber int       `json:"version_number"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	WordCount     int       `json:"word_count"`
	CharCount     int       `json:"char_count"`
	ChangeSummary string    `json:"change_summary"`
	IsAuto        bool      `json:"is_auto"`
	CreatedAt     time.Time `json:"created_at"`
	Username      string    `json:"username,omitempty"`
}
