<div align="center">

# 📓 Notebook

### Catatan modern untuk pikiran modern.

**An all-in-one, self-hosted workspace** that combines note-taking, task & project management, Kanban boards, mind maps, whiteboards, a web clipper, research tools, and productivity tracking — in one fast, keyboard-driven app.

Backend in **Go** · Frontend in plain **HTML / CSS / JavaScript** · No build step, no framework bloat.

![Go](https://img.shields.io/badge/backend-Go-00ADD8?logo=go&logoColor=white)
![JavaScript](https://img.shields.io/badge/frontend-Vanilla%20JS-F7DF1E?logo=javascript&logoColor=black)
![HTML5](https://img.shields.io/badge/-HTML5-E34F26?logo=html5&logoColor=white)
![CSS3](https://img.shields.io/badge/-CSS3-1572B6?logo=css3&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-green)
![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen)

</div>

---

## 📖 Table of Contents

- [Why Notebook?](#-why-notebook)
- [Feature Overview](#-feature-overview)
- [Modules in Detail](#-modules-in-detail)
- [Tech Stack](#-tech-stack)
- [Getting Started](#-getting-started)
- [Keyboard-First Workflow](#-keyboard-first-workflow)
- [Roles & Permissions](#-roles--permissions)
- [Project Structure](#-project-structure)
- [Roadmap](#-roadmap)
- [Contributing](#-contributing)
- [License](#-license)

---

## 💡 Why Notebook?

Most note apps make you choose: a writing tool, a task manager, a project board, or a research clipper — each living in a different tab. **Notebook** merges all of that into a single, self-hosted app with one login, one search bar, and one command palette (`Ctrl/Cmd + K`) to jump anywhere instantly.

---

## 🧭 Feature Overview

| | |
|---|---|
| 📝 **Rich Notes** | Notion-style editor with slash commands, tables, code blocks, checklists & more |
| 📂 **Notebooks & Folders** | Nested folders, favorites, drag & drop organization |
| ✅ **Tasks** | List & Kanban views, priorities, due dates, reminders |
| 📌 **Projects & Boards** | Trello-style Kanban boards linked to projects, notes & tasks |
| 🗓️ **Calendar & Timeline** | Month view of notes/tasks, chronological activity timeline |
| 🧠 **Mind Maps & Whiteboards** | Visual, freeform canvases for brainstorming |
| 🕸️ **Knowledge Graph** | Interactive graph of how your notes link together |
| ✂️ **Web Clipper** | Save web pages, build a reading list, and collect highlights |
| 🔬 **Research** | Manage sources/references for research-heavy work |
| 🖼️ **Media Library** | Central view of every image, video, audio & document you've uploaded |
| ⏱️ **Productivity Suite** | Built-in Pomodoro timer, daily planner, habit tracker & goals |
| 🤝 **Sharing & Collaboration** | Share notes/folders with granular Viewer/Editor permissions |
| 🕓 **Version History** | Every note change is versioned and restorable |
| 🔔 **Notifications & Reminders** | In-app + browser notifications for tasks and due dates |
| 🛡️ **Admin & Audit Log** | User management, RBAC, and a full activity trail |
| ⌨️ **Command Palette & Quick Capture** | Capture a note, task, or reminder from anywhere in one keystroke |

---

## 🧩 Modules in Detail

### 📝 Notes & Editor
- Full rich-text editing: bold/italic/underline/strikethrough, H1–H6, bullet & numbered lists, checklists, blockquotes, tables, code blocks with syntax highlighting, links, text/highlight colors, alignment, and horizontal rules
- `/` slash commands and Markdown shortcuts for fast writing
- Auto-save, pin/favorite, archive, lock, read-only mode, duplicate, and Trash with restore
- Full **version history** — view, compare, and restore any previous version
- Live word & character counter

### 🗂️ Notebooks, Folders & Tags
- Unlimited nested folders with custom colors and icons
- Drag & drop reordering, collapse/expand tree
- Custom tags with colors, tag suggestions, and "most used tags"

### ✅ Tasks, Projects & Boards
- Tasks with checklists, due dates, priorities, and status, viewable as a **List** or **Kanban board**
- **Projects** group related notes, tasks, and boards together with an activity feed
- **Boards** support custom columns and drag-and-drop cards — a full Trello-style workflow

### 🗓️ Calendar & Timeline
- **Calendar view** showing notes and tasks scheduled by date, with day drill-down
- **Timeline view** giving a chronological feed of everything you've created or completed

### 🧠 Knowledge Tools
- **Mind Maps** — freeform visual maps you can build from scratch or generate from a note
- **Whiteboards** — open canvases for sketching and diagramming
- **Knowledge Graph** — see backlinks and relationships between notes as an interactive graph

### ✂️ Web Clipper & Research
- **Clips** — save articles/pages from the web directly into your notebook
- **Reading List** — a queue of saved pages to get through later
- **Highlights** — key excerpts pulled from your clips
- **Research / Sources** — a lightweight reference manager for citations and source material

### 🖼️ Media Library
- Every attachment (image, video, audio, PDF, document) in one searchable library with usage stats and storage breakdown

### ⏱️ Productivity Suite
- **Pomodoro timer** with session presets and daily/weekly/total stats
- **Daily Planner** for laying out today's priorities
- **Habit tracker** for recurring routines
- **Goals** for longer-term tracking

### 📊 Dashboard
- Recent & recently-edited notes, favorites, recent files
- Pending/upcoming tasks and a 30-day activity chart (notes created, tasks completed, pomodoros run)
- "Top" widgets highlighting your most active notebooks/tags

### 🤝 Sharing & Collaboration
- Share notes or folders via public, private, or read-only links
- Per-user Viewer/Editor permissions, comments, and mentions
- "Shared with me" view for everything others have shared with you

### 🔔 Notifications, Search & Quick Capture
- In-app notification center + optional browser notifications
- Full-text search with filters (folder, tag, date, favorite, archive, checklist) and saved search presets
- **Quick Capture** modal (⚡ note / ✓ task / ⏰ reminder) and a **Command Palette** (`Ctrl/Cmd + K`) for jumping to any action instantly
- Local draft auto-save so you never lose a half-written quick capture

### 🛡️ Admin, Security & Audit
- Role-based access control: **Admin / User / Viewer**
- Admin panel for user management (create, disable, reset password)
- Full activity/audit log: creates, edits, deletes, restores, logins, uploads, shares, and permission changes
- JWT authentication with CSRF protection, bcrypt/argon2 password hashing, rate limiting, and account lockout

### 🔄 Data Management
- Import notes from Markdown, TXT, HTML, DOCX, or PDF
- Export notes to PDF, DOCX, Markdown, HTML, or TXT
- Manual & scheduled backups with restore

---

## 🛠️ Tech Stack

| Layer | Technology |
|---|---|
| Backend | **Go** — REST API, JWT auth, CSRF protection |
| Frontend | **HTML5, CSS3, vanilla JavaScript** (no framework, no build step) |
| Rich text editor | [Quill.js](https://quilljs.com) + `quill-better-table` |
| Diagrams | [Mermaid.js](https://mermaid.js.org) |
| Math rendering | [KaTeX](https://katex.org) |
| Code highlighting | [highlight.js](https://highlightjs.org) |
| Database | PostgreSQL / MySQL *(configurable)* |
| Auth | JWT (Bearer token) + CSRF token, stored client-side for API calls |

---

## 🚀 Getting Started

### Prerequisites
- [Go](https://go.dev/dl/) 1.21+
- A relational database (PostgreSQL or MySQL)
- A modern browser (Chrome, Firefox, Edge, Safari)

### Installation

```bash
# 1. Clone the repository
git clone https://github.com/<your-username>/notebook.git
cd notebook

# 2. Configure environment variables
cp .env.example .env
# then edit .env with your DB credentials, JWT secret, upload limits, etc.

# 3. Install Go dependencies
go mod download

# 4. Run database migrations (if applicable)
go run ./cmd/migrate

# 5. Start the server
go run ./cmd/server
```

The Go server serves the static frontend (`index.html`, `app.js`, `style.css`) and exposes the REST API from the same process. By default:

```
http://localhost:8080
```

> 🔧 Adjust the commands above to match your actual `cmd/` entrypoints and config setup.

### Example environment variables

| Variable | Description | Example |
|---|---|---|
| `PORT` | Port the server listens on | `8080` |
| `DATABASE_URL` | Database connection string | `postgres://user:pass@localhost:5432/notebook` |
| `JWT_SECRET` | Secret used to sign JWT tokens | `change-me` |
| `MAX_UPLOAD_SIZE_MB` | Max attachment size in MB | `25` |
| `STORAGE_PATH` | Local path (or bucket) for uploaded files | `./uploads` |

---

## ⌨️ Keyboard-First Workflow

Notebook is built to be used without touching the mouse:

- **`Ctrl/Cmd + K`** — open the Command Palette to jump to any note, action, or module
- **`/`** — open slash commands inside the editor
- **`Ctrl + /`** — open the full keyboard shortcuts reference
- Quick Capture shortcuts for instantly jotting a note, task, or reminder from anywhere in the app

---

## 🔐 Roles & Permissions

| Role | Access |
|---|---|
| **Admin** | Full access — manage users, view the audit log, configure the system |
| **User** | Create and manage their own notebooks, notes, tasks, projects, and boards; can share content with others |
| **Viewer** | Read-only access limited to notes/folders explicitly shared with them |

---

## 📁 Project Structure

```
notebook/
├── cmd/
│   └── server/            # Application entrypoint
├── internal/
│   ├── handlers/          # HTTP handlers (notes, tasks, boards, media, etc.)
│   ├── models/            # Database models
│   ├── middleware/        # Auth, CSRF, rate limiting, logging
│   └── storage/           # File storage logic
├── static/
│   ├── index.html         # Frontend entry
│   ├── app.js             # Frontend application logic
│   └── style.css          # Styles
├── go.mod
└── README.md
```

> Update this tree to match your actual repository layout.

---

## 🗺️ Roadmap

- [ ] Real-time collaborative editing (multi-cursor)
- [ ] Native mobile apps (iOS/Android)
- [ ] Third-party OAuth login (Google/Microsoft)
- [ ] Web push notifications
- [ ] Configurable file storage backend (local disk vs. S3-compatible)

---

## 🤝 Contributing

Contributions are welcome!

1. Fork this repository
2. Create a feature branch — `git checkout -b feature/amazing-feature`
3. Commit your changes — `git commit -m 'Add amazing feature'`
4. Push the branch — `git push origin feature/amazing-feature`
5. Open a Pull Request

---

## 📄 License

Licensed under the [MIT License](LICENSE).

---

<div align="center">

Built with ❤️ using Go and vanilla JavaScript — proof that you don't need a heavy framework to build a fast, feature-rich app.

</div>
