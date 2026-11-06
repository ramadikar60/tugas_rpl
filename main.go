package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"notebook-app/auth"
	"notebook-app/config"
	"notebook-app/handlers"
	"notebook-app/middleware"
)

func main() {
	config.LoadEnv(".env")

	if err := config.InitDB(); err != nil {
		log.Fatal("DB connection failed:", err)
	}
	fmt.Println("Database connected")

	// Cek skema keamanan akun (schema_v10.sql), lalu seed admin bila belum ada
	auth.InitJWTKey()
	handlers.CheckAuthSchema()
	handlers.SeedAdmin()

	// ============================================================
	// SCHEDULER
	// ============================================================
	trashRetentionDays := 30
	go func() {
		handlers.CleanupTrash(trashRetentionDays)
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			handlers.CleanupTrash(trashRetentionDays)
		}
	}()
	fmt.Printf("Auto-cleanup sampah aktif (retensi: %d hari)\n", trashRetentionDays)

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			handlers.CheckDueReminders()
		}
	}()
	fmt.Println("Task reminder aktif (cek tiap 1 menit)")
	fmt.Println(handlers.AIStartupInfo())

	// ============================================================
	// RATE LIMITERS
	// ============================================================
	globalLimiter := middleware.NewRateLimiter(20, 40)
	authLimiter := middleware.NewRateLimiter(0.5, 5)

	mux := http.NewServeMux()

	// ============================================================
	// STATIC FILES
	// ============================================================
	// /uploads/ disajikan dengan nosniff + CSP sandbox; file di luar daftar aman hanya bisa diunduh.
	mux.Handle("/uploads/", http.StripPrefix("/uploads/",
		handlers.SecureUploadHandler(uploadDir())))
	mux.Handle("/static/", http.StripPrefix("/static/",
		http.FileServer(http.Dir("./static"))))

	// Shared public page
	mux.HandleFunc("GET /shared/{token}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/shared.html")
	})

	// Admin page
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/admin.html")
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "./static/index.html")
	})

	// ============================================================
	// AUTH (public)
	// ============================================================
	mux.HandleFunc("POST /api/auth/register", authLimiter.Middleware(handlers.Register))
	mux.HandleFunc("POST /api/auth/login", authLimiter.Middleware(handlers.Login))
	mux.HandleFunc("POST /api/auth/logout", handlers.Logout)
	mux.HandleFunc("POST /api/auth/refresh", handlers.Refresh)
	mux.HandleFunc("POST /api/auth/forgot-password", authLimiter.Middleware(handlers.ForgotPassword))
	mux.HandleFunc("POST /api/auth/reset-password", authLimiter.Middleware(handlers.ResetPassword))

	mux.HandleFunc("GET /api/csrf", middleware.Auth(handlers.CSRFToken))

	// Public shared access
	mux.HandleFunc("GET /api/shared/access", handlers.AccessSharedNote)

	// ============================================================
	// HELPER: register protected route
	// ============================================================
	protected := func(method, path string, h http.HandlerFunc, needCSRF bool) {
		var hh http.HandlerFunc
		if needCSRF {
			hh = middleware.Chain(h, middleware.Auth, middleware.CSRF)
		} else {
			hh = middleware.Auth(h)
		}
		mux.HandleFunc(method+" "+path, hh)
	}

	// ============================================================
	// PROFILE
	// ============================================================
	protected("GET", "/api/profile", handlers.GetProfile, false)
	protected("PUT", "/api/profile", handlers.UpdateProfile, true)
	protected("POST", "/api/profile/change-password", handlers.ChangePassword, true)
	protected("GET", "/api/auth/sessions", handlers.ListSessions, false)
	protected("DELETE", "/api/auth/sessions/{id}", handlers.RevokeSession, true)
	protected("POST", "/api/auth/logout-all", handlers.LogoutAll, true)

	// ============================================================
	// NOTEBOOKS
	// ============================================================
	protected("GET", "/api/notebooks", handlers.ListNotebooks, false)
	protected("POST", "/api/notebooks", handlers.CreateNotebook, true)
	protected("PUT", "/api/notebooks/{id}", handlers.UpdateNotebook, true)
	protected("DELETE", "/api/notebooks/{id}", handlers.DeleteNotebook, true)
	protected("POST", "/api/notebooks/reorder", handlers.ReorderNotebooks, true)
	protected("PUT", "/api/notebooks/{id}/collapse", handlers.ToggleCollapseNotebook, true)

	// ============================================================
	// NOTES
	// ============================================================
	protected("GET", "/api/notes", handlers.ListNotes, false)
	protected("POST", "/api/notes", handlers.Audit("note_create", "note", handlers.CreateNote), true)
	protected("GET", "/api/notes/{id}", handlers.GetNote, false)
	protected("PUT", "/api/notes/{id}", handlers.AuditEdit("note_update", "note", handlers.UpdateNote), true)
	protected("POST", "/api/notes/{id}/trash", handlers.Audit("note_trash", "note", handlers.TrashNote), true)
	protected("POST", "/api/notes/{id}/restore", handlers.Audit("note_restore", "note", handlers.RestoreNote), true)
	protected("DELETE", "/api/notes/{id}", handlers.Audit("note_delete", "note", handlers.DeleteNotePermanently), true)
	protected("POST", "/api/notes/{id}/duplicate", handlers.Audit("note_duplicate", "note", handlers.DuplicateNote), true)

	// Lock / Read-only / Stats
	protected("POST", "/api/notes/{id}/lock", handlers.Audit("note_lock", "note", handlers.LockNote), true)
	protected("POST", "/api/notes/{id}/unlock", handlers.Audit("note_unlock", "note", handlers.UnlockNote), true)
	protected("PUT", "/api/notes/{id}/readonly", handlers.ToggleReadOnly, true)
	protected("POST", "/api/notes/{id}/recount", handlers.RecalcWordCount, false)

	// Links & Mentions
	protected("GET", "/api/notes/{id}/backlinks", handlers.ListBacklinks, false)
	protected("GET", "/api/notes/{id}/outgoing-links", handlers.ListOutgoingLinks, false)
	protected("GET", "/api/notes/{id}/related", handlers.ListRelatedNotes, false)
	protected("GET", "/api/note-titles", handlers.SearchNoteTitles, false)
	protected("GET", "/api/user-mentions", handlers.ListUserMentions, false)
	protected("GET", "/api/users/search", handlers.SearchUsers, false)
	protected("GET", "/api/graph", handlers.GetGraphData, false)

	// Version history
	protected("GET", "/api/notes/{id}/versions", handlers.ListNoteVersions, false)
	protected("GET", "/api/notes/{id}/versions/{version_id}", handlers.GetNoteVersion, false)
	protected("POST", "/api/notes/{id}/versions/{version_id}/restore", handlers.Audit("note_version_restore", "note", handlers.RestoreNoteVersion), true)
	protected("DELETE", "/api/notes/{id}/versions/{version_id}", handlers.DeleteNoteVersion, true)

	// Sync checklist to tasks
	protected("POST", "/api/notes/{note_id}/sync-tasks", handlers.SyncChecklistToTasks, true)

	// ============================================================
	// TAGS
	// ============================================================
	protected("GET", "/api/tags", handlers.ListTags, false)
	protected("GET", "/api/tags/most-used", handlers.MostUsedTags, false)
	protected("POST", "/api/tags", handlers.CreateTag, true)
	protected("PUT", "/api/tags/{id}", handlers.UpdateTag, true)
	protected("DELETE", "/api/tags/{id}", handlers.DeleteTag, true)
	protected("POST", "/api/notes/{note_id}/tags/{tag_id}", handlers.AttachTagToNote, true)
	protected("DELETE", "/api/notes/{note_id}/tags/{tag_id}", handlers.DetachTagFromNote, true)

	// ============================================================
	// ATTACHMENTS
	// ============================================================
	protected("POST", "/api/notes/{note_id}/attachments", handlers.Audit("attachment_upload", "note", handlers.UploadAttachment), true)
	protected("POST", "/api/notes/{note_id}/attachments/clipboard", handlers.Audit("attachment_upload", "note", handlers.UploadClipboardImage), true)
	protected("GET", "/api/notes/{note_id}/attachments/list", handlers.ListNoteAttachments, false)
	protected("GET", "/api/attachments/{id}/download", handlers.Audit("attachment_download", "attachment", handlers.DownloadAttachment), false)
	protected("GET", "/api/attachments/{id}/preview", handlers.PreviewAttachment, false)
	protected("PUT", "/api/attachments/{id}/rename", handlers.Audit("attachment_rename", "attachment", handlers.RenameAttachment), true)
	protected("DELETE", "/api/attachments/{id}", handlers.Audit("attachment_delete", "attachment", handlers.DeleteAttachment), true)

	// ============================================================
	// SEARCH
	// ============================================================
	protected("GET", "/api/search", handlers.SearchNotes, false)
	protected("GET", "/api/search/advanced", handlers.SearchNotes, false)
	protected("GET", "/api/search/history", handlers.ListSearchHistory, false)
	protected("POST", "/api/search/history", handlers.SaveSearchHistory, true)
	protected("DELETE", "/api/search/history", handlers.ClearSearchHistory, true)
	protected("GET", "/api/search/presets", handlers.ListSearchPresets, false)
	protected("POST", "/api/search/presets", handlers.SaveSearchPreset, true)
	protected("DELETE", "/api/search/presets/{id}", handlers.DeleteSearchPreset, true)

	// ============================================================
	// TRASH
	// ============================================================
	protected("DELETE", "/api/trash", handlers.Audit("trash_empty", "note", handlers.EmptyTrash), true)
	mux.HandleFunc("POST /api/trash/cleanup", middleware.Auth(func(w http.ResponseWriter, r *http.Request) {
		handlers.CleanupTrash(trashRetentionDays)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"Cleanup dijalankan"}`))
	}))

	// ============================================================
	// TASKS
	// ============================================================
	protected("GET", "/api/tasks", handlers.ListTasks, false)
	protected("GET", "/api/tasks/stats", handlers.TaskStats, false)
	protected("POST", "/api/tasks", handlers.CreateTask, true)
	protected("GET", "/api/tasks/{id}", handlers.GetTask, false)
	protected("PUT", "/api/tasks/{id}", handlers.UpdateTask, true)
	protected("DELETE", "/api/tasks/{id}", handlers.DeleteTask, true)
	protected("POST", "/api/tasks/{id}/toggle", handlers.ToggleTaskStatus, true)
	protected("GET", "/api/tasks/{id}/history", handlers.ListTaskHistory, false)

	// ============================================================
	// NOTIFICATIONS
	// ============================================================
	protected("GET", "/api/notifications", handlers.ListNotifications, false)
	protected("GET", "/api/notifications/unread-count", handlers.UnreadCount, false)
	protected("PUT", "/api/notifications/read-all", handlers.MarkAllRead, true)
	protected("PUT", "/api/notifications/{id}/read", handlers.MarkNotificationRead, true)
	protected("DELETE", "/api/notifications", handlers.ClearNotifications, true)
	protected("DELETE", "/api/notifications/{id}", handlers.DeleteNotification, true)

	// ============================================================
	// TEMPLATES
	// ============================================================
	protected("GET", "/api/templates", handlers.ListTemplates, false)
	protected("GET", "/api/templates/categories", handlers.ListTemplateCategories, false)
	protected("POST", "/api/templates", handlers.CreateTemplate, true)
	protected("PUT", "/api/templates/{id}", handlers.UpdateTemplate, true)
	protected("DELETE", "/api/templates/{id}", handlers.DeleteTemplate, true)
	protected("POST", "/api/templates/{id}/duplicate", handlers.DuplicateTemplate, true)
	protected("POST", "/api/notes/from-template", handlers.Audit("note_from_template", "note", handlers.CreateNoteFromTemplate), true)

	// ============================================================
	// SHARES
	// ============================================================
	protected("POST", "/api/shares", handlers.Audit("share_create", "share", handlers.CreateShare), true)
	protected("GET", "/api/shares", handlers.ListShares, false)
	protected("PUT", "/api/shares/{id}", handlers.Audit("share_update", "share", handlers.UpdateSharePermission), true)
	protected("DELETE", "/api/shares/{id}", handlers.Audit("share_revoke", "share", handlers.RevokeShare), true)
	protected("GET", "/api/shares/with-me", handlers.ListSharedWithMe, false)

	// ============================================================
	// COMMENTS
	// ============================================================
	protected("GET", "/api/notes/{id}/comments", handlers.ListComments, false)
	protected("POST", "/api/notes/{id}/comments", handlers.Audit("comment_create", "note", handlers.CreateComment), true)
	protected("PUT", "/api/comments/{id}", handlers.UpdateComment, true)
	protected("DELETE", "/api/comments/{id}", handlers.DeleteComment, true)
	protected("PUT", "/api/comments/{id}/resolve", handlers.ResolveComment, true)

	// ============================================================
	// ADMIN (RBAC)
	// ============================================================
	mux.HandleFunc("GET /api/admin/stats", handlers.AdminRoute(handlers.AdminStats))
	mux.HandleFunc("GET /api/admin/users", handlers.AdminRoute(handlers.AdminListUsers))
	mux.HandleFunc("POST /api/admin/users", handlers.AdminRoute(handlers.AdminCreateUser))
	mux.HandleFunc("GET /api/admin/users/{id}", handlers.AdminRoute(handlers.AdminGetUserDetail))
	mux.HandleFunc("PUT /api/admin/users/{id}", handlers.AdminRoute(handlers.AdminUpdateUser))
	mux.HandleFunc("DELETE /api/admin/users/{id}", handlers.AdminRoute(handlers.AdminDeleteUser))

	mux.HandleFunc("GET /api/admin/notes", handlers.AdminRoute(handlers.AdminListNotes))
	mux.HandleFunc("DELETE /api/admin/notes/{id}", handlers.AdminRoute(handlers.AdminDeleteNote))

	mux.HandleFunc("GET /api/admin/tasks", handlers.AdminRoute(handlers.AdminListTasks))
	mux.HandleFunc("GET /api/admin/activity", handlers.AdminRoute(handlers.AdminActivity))
	mux.HandleFunc("GET /api/admin/storage", handlers.AdminRoute(handlers.AdminStorage))

	protected("GET", "/api/activity", handlers.ListActivityLog, false)

	// ============================================================
	// PREFERENCES
	// ============================================================
	protected("GET", "/api/preferences", handlers.GetPreferences, false)
	protected("PUT", "/api/preferences", handlers.SavePreferences, true)

	// ============================================================
	// FASE 6: Quick Capture
	// ============================================================
	protected("POST", "/api/quick/note", handlers.QuickNote, true)
	protected("POST", "/api/quick/task", handlers.QuickTask, true)
	protected("POST", "/api/quick/reminder", handlers.QuickReminder, true)
	protected("POST", "/api/quick/clipboard", handlers.ClipboardCapture, true)

	protected("GET", "/api/draft", handlers.GetDraft, false)
	protected("POST", "/api/draft", handlers.SaveDraft, true)
	protected("DELETE", "/api/draft", handlers.DeleteDraft, true)

	protected("GET", "/api/recent", handlers.RecentItems, false)

	// ============================================================
	// FASE 7: Calendar & Timeline
	// ============================================================
	protected("GET", "/api/calendar/events", handlers.CalendarEvents, false)
	protected("GET", "/api/calendar/stats", handlers.CalendarStats, false)
	protected("GET", "/api/calendar/day", handlers.CalendarDayDetail, false)
	protected("POST", "/api/calendar/note", handlers.CreateNoteOnDate, true)

	protected("GET", "/api/timeline", handlers.Timeline, false)
	protected("GET", "/api/timeline/note/{id}", handlers.NoteTimeline, false)
	protected("GET", "/api/timeline/task/{id}", handlers.TaskTimeline, false)
	protected("GET", "/api/heatmap", handlers.ActivityHeatmap, false)

	// ============================================================
	// FASE 8a: Projects & Kanban
	// ============================================================
	protected("GET", "/api/projects", handlers.ListProjects, false)
	protected("POST", "/api/projects", handlers.CreateProject, true)
	protected("GET", "/api/projects/{id}", handlers.GetProject, false)
	protected("PUT", "/api/projects/{id}", handlers.UpdateProject, true)
	protected("DELETE", "/api/projects/{id}", handlers.DeleteProject, true)
	protected("POST", "/api/projects/{id}/notes", handlers.AddNoteToProject, true)
	protected("DELETE", "/api/projects/{id}/notes/{note_id}", handlers.RemoveNoteFromProject, true)

	protected("GET", "/api/boards", handlers.ListBoards, false)
	protected("POST", "/api/boards", handlers.CreateBoard, true)
	protected("GET", "/api/boards/{id}", handlers.GetBoard, false)
	protected("PUT", "/api/boards/{id}", handlers.UpdateBoard, true)
	protected("DELETE", "/api/boards/{id}", handlers.DeleteBoard, true)

	protected("POST", "/api/boards/{id}/columns", handlers.CreateColumn, true)
	protected("PUT", "/api/columns/{id}", handlers.UpdateColumn, true)
	protected("DELETE", "/api/columns/{id}", handlers.DeleteColumn, true)

	protected("POST", "/api/columns/{id}/cards", handlers.CreateCard, true)
	protected("PUT", "/api/cards/{id}", handlers.UpdateCard, true)
	protected("POST", "/api/cards/{id}/move", handlers.MoveCard, true)
	protected("DELETE", "/api/cards/{id}", handlers.DeleteCard, true)

	// ============================================================
	// FASE 8b: Mind Map & Whiteboard
	// ============================================================
	protected("GET", "/api/mindmaps", handlers.ListMindMaps, false)
	protected("POST", "/api/mindmaps", handlers.CreateMindMap, true)
	protected("GET", "/api/mindmaps/{id}", handlers.GetMindMap, false)
	protected("PUT", "/api/mindmaps/{id}", handlers.UpdateMindMap, true)
	protected("DELETE", "/api/mindmaps/{id}", handlers.DeleteMindMap, true)
	protected("POST", "/api/mindmaps/from-note", handlers.GenerateFromNote, true)

	protected("GET", "/api/whiteboards", handlers.ListWhiteboards, false)
	protected("POST", "/api/whiteboards", handlers.CreateWhiteboard, true)
	protected("GET", "/api/whiteboards/{id}", handlers.GetWhiteboard, false)
	protected("PUT", "/api/whiteboards/{id}", handlers.UpdateWhiteboard, true)
	protected("DELETE", "/api/whiteboards/{id}", handlers.DeleteWhiteboard, true)

	// ============================================================
	// FASE 9: Web Clipper, Research, Media
	// ============================================================
	protected("POST", "/api/clips", handlers.ClipWebPage, true)
	protected("GET", "/api/clips", handlers.ListClips, false)
	protected("GET", "/api/clips/{id}", handlers.GetClip, false)
	protected("PUT", "/api/clips/{id}", handlers.UpdateClip, true)
	protected("DELETE", "/api/clips/{id}", handlers.DeleteClip, true)

	protected("GET", "/api/reading-list", handlers.ListReadingList, false)
	protected("POST", "/api/reading-list", handlers.AddToReadingList, true)
	protected("PUT", "/api/reading-list/{id}", handlers.UpdateReadingList, true)
	protected("DELETE", "/api/reading-list/{id}", handlers.DeleteReadingList, true)

	protected("GET", "/api/sources", handlers.ListSources, false)
	protected("POST", "/api/sources", handlers.CreateSource, true)
	protected("GET", "/api/sources/{id}", handlers.GetSource, false)
	protected("PUT", "/api/sources/{id}", handlers.UpdateSource, true)
	protected("DELETE", "/api/sources/{id}", handlers.DeleteSource, true)
	protected("GET", "/api/sources/{id}/related", handlers.GetRelatedSources, false)

	protected("GET", "/api/highlights", handlers.ListHighlights, false)
	protected("POST", "/api/highlights", handlers.CreateHighlight, true)
	protected("PUT", "/api/highlights/{id}", handlers.UpdateHighlight, true)
	protected("DELETE", "/api/highlights/{id}", handlers.DeleteHighlight, true)

	protected("GET", "/api/media", handlers.ListMedia, false)
	protected("POST", "/api/media", handlers.RegisterMedia, true)
	protected("PUT", "/api/media/{id}", handlers.UpdateMedia, true)
	protected("DELETE", "/api/media/{id}", handlers.DeleteMedia, true)
	protected("GET", "/api/media/stats", handlers.MediaStats, false)
	protected("GET", "/api/media/tags", handlers.ListMediaTags, false)
	protected("POST", "/api/media/backfill", handlers.BackfillMedia, true)

	// ============================================================
	// FASE 10: Productivity & Dashboard
	// ============================================================
	protected("POST", "/api/pomodoro/start", handlers.StartPomodoro, true)
	protected("POST", "/api/pomodoro/{id}/complete", handlers.CompletePomodoro, true)
	protected("GET", "/api/pomodoro/stats", handlers.PomodoroStats, false)

	protected("GET", "/api/daily-plan", handlers.GetDailyPlan, false)
	protected("POST", "/api/daily-plan", handlers.SaveDailyPlan, true)
	protected("GET", "/api/daily-plans", handlers.ListDailyPlans, false)

	protected("GET", "/api/habits", handlers.ListHabits, false)
	protected("POST", "/api/habits", handlers.CreateHabit, true)
	protected("DELETE", "/api/habits/{id}", handlers.DeleteHabit, true)
	protected("POST", "/api/habits/{id}/toggle", handlers.ToggleHabit, true)
	protected("GET", "/api/habits/{id}/heatmap", handlers.HabitHeatmap, false)

	protected("GET", "/api/goals", handlers.ListGoals, false)
	protected("POST", "/api/goals", handlers.CreateGoal, true)
	protected("PUT", "/api/goals/{id}", handlers.UpdateGoalProgress, true)
	protected("DELETE", "/api/goals/{id}", handlers.DeleteGoal, true)

	protected("GET", "/api/dashboard/stats", handlers.DashboardStats, false)
	protected("GET", "/api/dashboard/chart", handlers.ProductivityChart, false)
	protected("GET", "/api/dashboard/top", handlers.TopItems, false)

	protected("POST", "/api/bulk", handlers.BulkAction, true)
	protected("GET", "/api/export/notes", handlers.ExportNotes, false)
	protected("POST", "/api/import/notes", handlers.ImportNotes, true)
	protected("GET", "/api/duplicates", handlers.FindDuplicates, false)
	protected("GET", "/api/storage/breakdown", handlers.StorageBreakdown, false)
	protected("GET", "/api/storage/largest", handlers.LargestAttachments, false)

	// ============================================================
	// ASISTEN AI (Gemini default, gratis)
	// ============================================================
	protected("GET", "/api/ai/status", handlers.AIStatus, false)
	protected("POST", "/api/ai/generate", handlers.AIGenerate, true)

	// ============================================================
	// GLOBAL MIDDLEWARE & START
	// ============================================================
	handler := middleware.SecurityHeaders(corsMiddleware(loggingMiddleware(globalLimiter.MiddlewareHandler(mux))))

	port := config.GetEnv("PORT", "8080")
	srv := &http.Server{
		Handler:      handler,
		Addr:         ":" + port,
		WriteTimeout: 30 * time.Second,
		ReadTimeout:  30 * time.Second,
	}

	fmt.Printf("Server running on http://localhost:%s\n", port)
	log.Fatal(srv.ListenAndServe())
}

func uploadDir() string {
	return config.GetEnv("UPLOAD_DIR", "./uploads")
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-CSRF-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
