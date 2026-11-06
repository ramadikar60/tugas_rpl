// ============================================================
// 4-PRODUCTIVITY.JS — Tasks, Calendar, Timeline, Projects, Kanban
// ============================================================

// ============================================================
// STATE
// ============================================================
let currentTaskView = 'list';
let taskSearchTimer = null;

let calendarYear = new Date().getFullYear();
let calendarMonth = new Date().getMonth() + 1;
let calendarView = 'month';
let selectedCalendarDate = null;

let currentProjectId = null;
let currentProjectTab = 'overview';

let currentBoardId = null;

// ============================================================
// 15. TASKS
// ============================================================
async function openTaskDashboard() {
  showMode('tasksMode');
  setMobileTitle('Tugas');
  setupTasksBackBtn();
  if (window.innerWidth <= 767) closeSidebar();

  await loadTaskStats();
  await loadTasks();
  await populateTaskNotebookDropdown();
}

function closeTasks() {
  navigateTo('list', { filter: currentFilter });
}

function switchTaskView(view) {
  currentTaskView = view;
  document.getElementById('taskListView').style.display = view === 'list' ? 'flex' : 'none';
  document.getElementById('taskKanbanView').style.display = view === 'kanban' ? 'grid' : 'none';
  document.getElementById('viewListBtn').classList.toggle('active', view === 'list');
  document.getElementById('viewKanbanBtn').classList.toggle('active', view === 'kanban');
  loadTasks();
}

async function loadTaskStats() {
  const stats = await api('/tasks/stats');
  if (!stats) return;

  const el = document.getElementById('taskStats');
  el.innerHTML = `
    <div class="stat-card info" onclick="quickFilterTasks('status', 'all')">
      <div class="stat-value">${stats.total || 0}</div>
      <div class="stat-label">Total</div>
    </div>
    <div class="stat-card" onclick="quickFilterTasks('status', 'pending')">
      <div class="stat-value">${stats.pending || 0}</div>
      <div class="stat-label">Pending</div>
    </div>
    <div class="stat-card" onclick="quickFilterTasks('status', 'in_progress')">
      <div class="stat-value">${stats.in_progress || 0}</div>
      <div class="stat-label">Dikerjakan</div>
    </div>
    <div class="stat-card success" onclick="quickFilterTasks('status', 'done')">
      <div class="stat-value">${stats.done || 0}</div>
      <div class="stat-label">Selesai</div>
    </div>
    <div class="stat-card danger" onclick="quickFilterTasks('due', 'overdue')">
      <div class="stat-value">${stats.overdue || 0}</div>
      <div class="stat-label">Terlambat</div>
    </div>
    <div class="stat-card warning" onclick="quickFilterTasks('due', 'today')">
      <div class="stat-value">${stats.today || 0}</div>
      <div class="stat-label">Hari ini</div>
    </div>
    <div class="stat-card warning" onclick="quickFilterTasks('priority', 'urgent')">
      <div class="stat-value">${stats.urgent || 0}</div>
      <div class="stat-label">Urgent</div>
    </div>
    <div class="stat-card" onclick="quickFilterTasks('due', 'week')">
      <div class="stat-value">${stats.week || 0}</div>
      <div class="stat-label">Minggu ini</div>
    </div>
  `;
}

function quickFilterTasks(type, value) {
  if (type === 'status') {
    document.getElementById('taskStatusFilter').value = value;
    document.getElementById('taskPriorityFilter').value = '';
    document.getElementById('taskDueFilter').value = '';
  } else if (type === 'priority') {
    document.getElementById('taskPriorityFilter').value = value;
    document.getElementById('taskStatusFilter').value = 'all';
    document.getElementById('taskDueFilter').value = '';
  } else if (type === 'due') {
    document.getElementById('taskDueFilter').value = value;
    document.getElementById('taskStatusFilter').value = 'all';
    document.getElementById('taskPriorityFilter').value = '';
  }
  loadTasks();
}

async function loadTasks() {
  const status = document.getElementById('taskStatusFilter').value;
  const priority = document.getElementById('taskPriorityFilter').value;
  const due = document.getElementById('taskDueFilter').value;
  const q = document.getElementById('taskSearch').value.trim();

  const params = new URLSearchParams();
  if (status) params.set('status', status);
  if (priority) params.set('priority', priority);
  if (due) params.set('due', due);
  if (q) params.set('q', q);

  const tasks = await api('/tasks?' + params.toString());
  if (!tasks) return;

  if (currentTaskView === 'list') renderTaskList(tasks);
  else renderTaskKanban(tasks);
}

function renderTaskList(tasks) {
  const el = document.getElementById('taskListView');
  el.innerHTML = '';

  if (tasks.length === 0) {
    el.innerHTML = '<div class="task-empty">Belum ada tugas. Klik "+ Tugas Baru" untuk membuat.</div>';
    return;
  }

  tasks.forEach(t => {
    const card = document.createElement('div');
    card.className = `task-card priority-${t.priority} status-${t.status}`;

    const isDone = t.status === 'done';
    const priorityLabel = {
      low: 'Rendah', medium: 'Sedang', high: 'Tinggi', urgent: 'Urgent'
    }[t.priority] || t.priority;

    let dueHtml = '';
    if (t.due_date) {
      dueHtml = `<span style="color:${t.is_overdue ? 'var(--danger)' : 'var(--text-muted)'}">
        ${t.is_overdue ? '⚠ ' : ''}${formatDateTime(t.due_date)}
      </span>`;
    }

    card.innerHTML = `
      <div class="task-checkbox ${isDone ? 'checked' : ''}" onclick="event.stopPropagation(); toggleTask(${t.id})"></div>
      <div class="task-main">
        <div class="task-title">${escapeHtml(t.title)}</div>
        <div class="task-meta">
          <span class="task-badge ${t.priority}">${priorityLabel}</span>
          ${t.is_overdue ? '<span class="task-badge overdue">Terlambat</span>' : ''}
          ${dueHtml}
        </div>
      </div>
      <div class="task-actions">
        <button onclick="event.stopPropagation(); editTask(${t.id})" title="Edit">✎</button>
        <button class="danger" onclick="event.stopPropagation(); deleteTask(${t.id})" title="Hapus">✕</button>
      </div>
    `;
    card.onclick = () => editTask(t.id);
    el.appendChild(card);
  });
}

function renderTaskKanban(tasks) {
  ['kanbanPending', 'kanbanInProgress', 'kanbanDone'].forEach(id => {
    document.getElementById(id).innerHTML = '';
  });

  const counts = { pending: 0, in_progress: 0, done: 0 };

  tasks.forEach(t => {
    let colId = 'kanbanPending';
    if (t.status === 'in_progress') colId = 'kanbanInProgress';
    else if (t.status === 'done') colId = 'kanbanDone';
    else if (t.status === 'cancelled') return;

    const card = document.createElement('div');
    card.className = `kanban-card priority-${t.priority}`;
    card.draggable = true;
    card.dataset.taskId = t.id;

    const priorityLabel = {
      low: 'Rendah', medium: 'Sedang', high: 'Tinggi', urgent: 'Urgent'
    }[t.priority] || t.priority;

    let dueHtml = '';
    if (t.due_date) {
      dueHtml = `<span style="color:${t.is_overdue ? 'var(--danger)' : 'inherit'}">
        ${formatDateTime(t.due_date)}
      </span>`;
    }

    card.innerHTML = `
      <div class="task-title">${escapeHtml(t.title)}</div>
      <div class="task-meta">
        <span class="task-badge ${t.priority}">${priorityLabel}</span>
        ${dueHtml}
      </div>
    `;

    card.addEventListener('dragstart', e => {
      e.dataTransfer.setData('task_id', t.id);
      e.dataTransfer.effectAllowed = 'move';
    });

    card.onclick = () => editTask(t.id);

    document.getElementById(colId).appendChild(card);
    if (counts[t.status] !== undefined) counts[t.status]++;
  });

  document.getElementById('kanbanPendingCount').textContent = counts.pending;
  document.getElementById('kanbanProgressCount').textContent = counts.in_progress;
  document.getElementById('kanbanDoneCount').textContent = counts.done;

  setupKanbanDropzones();
}

function setupKanbanDropzones() {
  const zones = [
    { id: 'kanbanPending', status: 'pending' },
    { id: 'kanbanInProgress', status: 'in_progress' },
    { id: 'kanbanDone', status: 'done' }
  ];

  zones.forEach(z => {
    const el = document.getElementById(z.id);
    if (!el) return;

    el.addEventListener('dragover', e => {
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
      el.classList.add('drag-over');
    });

    el.addEventListener('dragleave', () => el.classList.remove('drag-over'));

    el.addEventListener('drop', async e => {
      e.preventDefault();
      el.classList.remove('drag-over');
      const taskId = e.dataTransfer.getData('task_id');
      if (!taskId) return;
      await api('/tasks/' + taskId, {
        method: 'PUT',
        body: JSON.stringify({ status: z.status })
      });
      loadTasks();
      loadTaskStats();
    });
  });
}

async function toggleTask(id) {
  const res = await api('/tasks/' + id + '/toggle', { method: 'POST' });
  if (res && res.status) {
    loadTasks();
    loadTaskStats();
  }
}

async function deleteTask(id) {
  if (!confirm('Hapus tugas ini?')) return;
  await api('/tasks/' + id, { method: 'DELETE' });
  showToast('Tugas dihapus.', 'success');
  loadTasks();
  loadTaskStats();
}

function showCreateTaskModal() {
  document.getElementById('taskModalTitle').textContent = 'Tugas Baru';
  document.getElementById('taskModalId').value = '';
  document.getElementById('taskModalTitleInput').value = '';
  document.getElementById('taskModalDesc').value = '';
  document.getElementById('taskModalPriority').value = 'medium';
  document.getElementById('taskModalStatus').value = 'pending';
  document.getElementById('taskModalDue').value = '';
  document.getElementById('taskModalReminder').value = '';
  document.getElementById('taskModalNotebook').value = '';
  document.getElementById('createTaskModal').style.display = 'flex';
  setTimeout(() => document.getElementById('taskModalTitleInput').focus(), 100);
}

async function editTask(id) {
  const task = await api('/tasks/' + id);
  if (!task) return;

  document.getElementById('taskModalTitle').textContent = 'Edit Tugas';
  document.getElementById('taskModalId').value = task.id;
  document.getElementById('taskModalTitleInput').value = task.title || '';
  document.getElementById('taskModalDesc').value = task.description || '';
  document.getElementById('taskModalPriority').value = task.priority || 'medium';
  document.getElementById('taskModalStatus').value = task.status || 'pending';
  document.getElementById('taskModalDue').value = toDatetimeLocal(task.due_date);
  document.getElementById('taskModalReminder').value = toDatetimeLocal(task.reminder_at);
  document.getElementById('taskModalNotebook').value = task.notebook_id || '';
  document.getElementById('createTaskModal').style.display = 'flex';
}

function closeCreateTaskModal() {
  document.getElementById('createTaskModal').style.display = 'none';
}

async function saveTaskFromModal() {
  const id = document.getElementById('taskModalId').value;
  const title = document.getElementById('taskModalTitleInput').value.trim();
  if (!title) {
    showToast('Judul tugas wajib diisi.', 'error');
    return;
  }

  const payload = {
    title,
    description: document.getElementById('taskModalDesc').value.trim(),
    priority: document.getElementById('taskModalPriority').value,
    status: document.getElementById('taskModalStatus').value,
    due_date: document.getElementById('taskModalDue').value || null,
    reminder_at: document.getElementById('taskModalReminder').value || null,
    notebook_id: document.getElementById('taskModalNotebook').value
      ? parseInt(document.getElementById('taskModalNotebook').value) : null
  };

  let res;
  if (id) {
    res = await api('/tasks/' + id, {
      method: 'PUT',
      body: JSON.stringify(payload)
    });
  } else {
    res = await api('/tasks', {
      method: 'POST',
      body: JSON.stringify(payload)
    });
  }

  if (res) {
    showToast(id ? 'Tugas diperbarui.' : 'Tugas dibuat.', 'success');
    closeCreateTaskModal();
    loadTasks();
    loadTaskStats();
    if (payload.reminder_at) requestNotifPermissionIfNeeded();
  } else {
    showToast('Gagal menyimpan tugas.', 'error');
  }
}

async function populateTaskNotebookDropdown() {
  const tree = await api('/notebooks');
  const select = document.getElementById('taskModalNotebook');
  select.innerHTML = '<option value="">Tanpa folder</option>';
  if (!tree) return;

  function walk(items, prefix = '') {
    items.forEach(nb => {
      const opt = document.createElement('option');
      opt.value = nb.id;
      opt.textContent = prefix + nb.name;
      select.appendChild(opt);
      if (nb.children && nb.children.length) walk(nb.children, prefix + '— ');
    });
  }
  walk(tree);
}

function debouncedTaskSearch() {
  clearTimeout(taskSearchTimer);
  taskSearchTimer = setTimeout(loadTasks, 300);
}

async function syncChecklistToTasks() {
  if (!currentNoteId) return;
  if (!confirm('Ubah semua checklist di catatan ini menjadi tugas?')) return;
  const res = await api('/notes/' + currentNoteId + '/sync-tasks', { method: 'POST' });
  if (res) {
    showToast(`${res.created} tugas dibuat dari ${res.total} checklist.`, 'success');
  }
}

// ============================================================
// FASE 7: CALENDAR & TIMELINE
// ============================================================

// ============================================================
// CALENDAR
// ============================================================
async function openCalendar() {
  showMode('calendarMode');
  setMobileTitle('Kalender');
  setupEditorBackBtn('.calendar-header .editor-back');
  if (window.innerWidth <= 767) closeSidebar();

  await loadCalendarStats();
  await loadCalendarMonth();
}

function closeCalendar() {
  navigateTo('list', { filter: currentFilter });
}

function calendarPrevMonth() {
  calendarMonth--;
  if (calendarMonth < 1) {
    calendarMonth = 12;
    calendarYear--;
  }
  loadCalendarMonth();
  loadCalendarStats();
}

function calendarNextMonth() {
  calendarMonth++;
  if (calendarMonth > 12) {
    calendarMonth = 1;
    calendarYear++;
  }
  loadCalendarMonth();
  loadCalendarStats();
}

function calendarGoToday() {
  const now = new Date();
  calendarYear = now.getFullYear();
  calendarMonth = now.getMonth() + 1;
  loadCalendarMonth();
  loadCalendarStats();
}

function switchCalendarView(view) {
  calendarView = view;
  document.querySelectorAll('.calendar-view-switch button').forEach(b => {
    b.classList.toggle('active', b.dataset.view === view);
  });
  loadCalendarMonth();
}

async function loadCalendarStats() {
  const stats = await api(`/calendar/stats?year=${calendarYear}&month=${calendarMonth}`);
  if (!stats) return;

  const el = document.getElementById('calendarStats');
  el.innerHTML = `
    <div class="stat-card info">
      <div class="stat-value">${stats.notes || 0}</div>
      <div class="stat-label">Catatan</div>
    </div>
    <div class="stat-card warning">
      <div class="stat-value">${stats.tasks || 0}</div>
      <div class="stat-label">Tugas</div>
    </div>
    <div class="stat-card danger">
      <div class="stat-value">${stats.reminders || 0}</div>
      <div class="stat-label">Pengingat</div>
    </div>
    <div class="stat-card success">
      <div class="stat-value">${stats.completed || 0}</div>
      <div class="stat-label">Selesai</div>
    </div>
  `;
}

async function loadCalendarMonth() {
  const monthNames = ['Januari', 'Februari', 'Maret', 'April', 'Mei', 'Juni',
    'Juli', 'Agustus', 'September', 'Oktober', 'November', 'Desember'];
  document.getElementById('calendarMonthTitle').textContent =
    `${monthNames[calendarMonth - 1]} ${calendarYear}`;

  const data = await api(`/calendar/events?year=${calendarYear}&month=${calendarMonth}`);
  const byDate = data && data.by_date ? data.by_date : {};

  const grid = document.getElementById('calendarGrid');
  grid.innerHTML = '';

  const firstDay = new Date(calendarYear, calendarMonth - 1, 1);
  const lastDay = new Date(calendarYear, calendarMonth, 0);

  let startWeekday = firstDay.getDay() - 1;
  if (startWeekday < 0) startWeekday = 6;

  const prevMonthLastDay = new Date(calendarYear, calendarMonth - 1, 0);

  for (let i = 0; i < startWeekday; i++) {
    const dayNum = prevMonthLastDay.getDate() - startWeekday + i + 1;
    const dayEl = createCalendarDay(dayNum, true, null, {});
    grid.appendChild(dayEl);
  }

  const today = new Date();
  const todayStr = today.toISOString().split('T')[0];

  for (let day = 1; day <= lastDay.getDate(); day++) {
    const dateStr = `${calendarYear}-${String(calendarMonth).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
    const isToday = dateStr === todayStr;
    const events = byDate[dateStr] || [];
    const dayEl = createCalendarDay(day, false, dateStr, events, isToday);
    grid.appendChild(dayEl);
  }

  const totalCells = grid.children.length;
  const remainingCells = (7 - (totalCells % 7)) % 7;
  for (let i = 1; i <= remainingCells; i++) {
    const dayEl = createCalendarDay(i, true, null, {});
    grid.appendChild(dayEl);
  }
}

function createCalendarDay(dayNum, isOtherMonth, dateStr, events, isToday = false) {
  const dayEl = document.createElement('div');
  dayEl.className = 'calendar-day';
  if (isOtherMonth) dayEl.classList.add('other-month');
  if (isToday) dayEl.classList.add('today');

  let eventsHtml = '';

  if (events && Array.isArray(events) && events.length > 0) {
    const shownEvents = events.slice(0, 3);
    shownEvents.forEach(e => {
      let cls = e.type;
      if (e.type === 'task' && e.status === 'done') cls += ' completed';

      const title = (e.title || 'Tanpa judul').substring(0, 24);
      eventsHtml += `<div class="calendar-event ${cls}" title="${escapeHtml(e.title || '')}">${escapeHtml(title)}</div>`;
    });

    if (events.length > 3) {
      eventsHtml += `<div class="calendar-more">+${events.length - 3} lagi</div>`;
    }
  }

  dayEl.innerHTML = `
    <div class="calendar-day-number">${dayNum}</div>
    <div class="calendar-day-events">${eventsHtml}</div>
  `;

  if (dateStr) {
    dayEl.onclick = () => openCalendarDay(dateStr);
  }

  return dayEl;
}

async function openCalendarDay(dateStr) {
  selectedCalendarDate = dateStr;

  const panel = document.getElementById('calendarDayPanel');
  panel.style.display = 'flex';

  const [y, m, d] = dateStr.split('-');
  const monthNames = ['Januari', 'Februari', 'Maret', 'April', 'Mei', 'Juni',
    'Juli', 'Agustus', 'September', 'Oktober', 'November', 'Desember'];
  document.getElementById('calendarDayTitle').textContent =
    `${parseInt(d)} ${monthNames[parseInt(m) - 1]} ${y}`;

  const data = await api(`/calendar/day?date=${dateStr}`);
  const el = document.getElementById('calendarDayContent');

  if (!data || !data.events || data.events.length === 0) {
    el.innerHTML = `
      <div class="panel-empty" style="padding: 40px 20px;">
        Belum ada aktivitas di hari ini.<br>
        <span style="font-size:12px;color:var(--text-faint);">Klik tombol di bawah untuk membuat catatan.</span>
      </div>
    `;
    return;
  }

  let html = '';
  data.events.forEach(e => {
    const typeLabel = {
      note: '📄 Catatan',
      task: '✓ Tugas',
      reminder: '⏰ Pengingat'
    }[e.type] || e.type;

    html += `
      <div class="calendar-day-event" onclick="handleCalendarEventClick('${e.type}', ${e.id})">
        <div class="calendar-day-event-time">${e.time || '--:--'}</div>
        <div class="calendar-day-event-info">
          <div class="calendar-day-event-title">${escapeHtml(e.title || 'Tanpa judul')}</div>
          <div class="calendar-day-event-type">${typeLabel}</div>
        </div>
      </div>
    `;
  });

  el.innerHTML = html;
}

function handleCalendarEventClick(type, id) {
  closeCalendarDay();

  if (type === 'note') {
    openNote(id);
  } else if (type === 'task' || type === 'reminder') {
    navigateTo('tasks', {
      callback: () => setTimeout(() => editTask(id), 400)
    });
  }
}

function closeCalendarDay() {
  document.getElementById('calendarDayPanel').style.display = 'none';
  selectedCalendarDate = null;
}

function createNoteOnSelectedDate() {
  if (!selectedCalendarDate) return;

  document.getElementById('createNoteDateInput').value = selectedCalendarDate;

  const [y, m, d] = selectedCalendarDate.split('-');
  const monthNames = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun',
    'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des'];
  document.getElementById('createNoteDateLabel').textContent =
    `${parseInt(d)} ${monthNames[parseInt(m) - 1]} ${y}`;

  document.getElementById('createNoteTitleInput').value = '';
  document.getElementById('createNoteContentInput').value = '';

  document.getElementById('createNoteOnDateModal').style.display = 'flex';
  setTimeout(() => document.getElementById('createNoteTitleInput').focus(), 100);
}

function closeCreateNoteOnDate() {
  document.getElementById('createNoteOnDateModal').style.display = 'none';
}

async function submitCreateNoteOnDate() {
  const date = document.getElementById('createNoteDateInput').value;
  const title = document.getElementById('createNoteTitleInput').value.trim();
  const content = document.getElementById('createNoteContentInput').value.trim();

  if (!content) {
    showToast('Isi catatan wajib diisi.', 'error');
    return;
  }

  const htmlContent = content
    .split('\n')
    .filter(p => p.trim())
    .map(p => '<p>' + escapeHtml(p) + '</p>')
    .join('');

  const res = await api('/calendar/note', {
    method: 'POST',
    body: JSON.stringify({
      date,
      title: title || 'Catatan ' + date,
      content: htmlContent
    })
  });

  if (res && res.id) {
    showToast('Catatan dibuat.', 'success');
    closeCreateNoteOnDate();
    closeCalendarDay();
    loadCalendarMonth();
    loadCalendarStats();
    setTimeout(() => openNote(res.id), 400);
  } else {
    showToast('Gagal membuat catatan.', 'error');
  }
}

// ============================================================
// TIMELINE
// ============================================================
async function openTimeline() {
  showMode('timelineMode');
  setMobileTitle('Timeline');
  setupEditorBackBtn('.timeline-header .editor-back');
  if (window.innerWidth <= 767) closeSidebar();

  await loadHeatmap();
  await loadTimeline();
}

function closeTimeline() {
  navigateTo('list', { filter: currentFilter });
}

async function loadTimeline() {
  const type = document.getElementById('timelineTypeFilter').value;
  const from = document.getElementById('timelineFromDate').value;
  const to = document.getElementById('timelineToDate').value;

  const params = new URLSearchParams();
  params.set('type', type);
  params.set('limit', '100');
  if (from) params.set('from', from);
  if (to) params.set('to', to);

  const data = await api('/timeline?' + params.toString());
  const el = document.getElementById('timelineList');

  if (!data || !data.events || data.events.length === 0) {
    el.innerHTML = '<div class="timeline-empty">Belum ada aktivitas untuk ditampilkan</div>';
    return;
  }

  let html = '';

  data.events.forEach(e => {
    const date = new Date(e.timestamp);
    const dateStr = date.toLocaleDateString('id-ID', { day: 'numeric', month: 'short' });
    const timeStr = date.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' });

    let category = 'activity';
    if (e.category) category = e.category;
    else if (e.type.startsWith('note')) category = 'note';
    else if (e.type.startsWith('task')) category = 'task';
    else if (e.type === 'version') category = 'version';

    const onClick = getTimelineOnClick(e);

    html += `
      <div class="timeline-event" onclick="${onClick}">
        <div class="timeline-event-time">
          <div class="timeline-event-date">${dateStr}</div>
          <div class="timeline-event-hour">${timeStr}</div>
        </div>
        <div class="timeline-event-dot ${category}"></div>
        <div class="timeline-event-content">
          <div class="timeline-event-title">${escapeHtml(e.title || '-')}</div>
          <div class="timeline-event-desc">${escapeHtml(e.description || '')}</div>
        </div>
      </div>
    `;
  });

  el.innerHTML = html;
}

function getTimelineOnClick(e) {
  if (e.category === 'note' && e.id) {
    return `openNote(${e.id})`;
  }
  if (e.category === 'task' && e.id) {
    return `navigateTo('tasks', {callback: () => setTimeout(() => editTask(${e.id}), 400)})`;
  }
  return '';
}

async function loadHeatmap() {
  const year = new Date().getFullYear();
  document.getElementById('heatmapYear').textContent = year;

  const data = await api(`/heatmap?year=${year}`);
  const grid = document.getElementById('heatmapGrid');
  grid.innerHTML = '';

  if (!data || !data.days) return;

  const dayMap = {};
  data.days.forEach(d => {
    dayMap[d.date] = d.count;
  });

  const startDate = new Date(year, 0, 1);
  const endDate = new Date(year, 11, 31);

  let current = new Date(startDate);
  const startWeekday = (current.getDay() + 6) % 7;
  current.setDate(current.getDate() - startWeekday);

  while (current <= endDate) {
    const dateStr = current.toISOString().split('T')[0];
    const count = dayMap[dateStr] || 0;

    let level = 0;
    if (count >= 1) level = 1;
    if (count >= 3) level = 2;
    if (count >= 6) level = 3;
    if (count >= 10) level = 4;

    const dayEl = document.createElement('div');
    dayEl.className = `heatmap-day level-${level}`;
    dayEl.title = `${dateStr}: ${count} aktivitas`;

    if (current >= startDate && current <= endDate) {
      dayEl.onclick = () => {
        calendarYear = current.getFullYear();
        calendarMonth = current.getMonth() + 1;
        navigateTo('calendar', {
          callback: () => {
            setTimeout(() => openCalendarDay(dateStr), 500);
          }
        });
      };
    }

    grid.appendChild(dayEl);
    current.setDate(current.getDate() + 1);
  }
}

async function showNoteTimeline(noteId) {
  const data = await api(`/timeline/note/${noteId}`);
  if (!data) return;

  document.getElementById('timelineDetailTitle').textContent = 'Riwayat Catatan';

  let html = '';
  if (data.length === 0) {
    html = '<div class="panel-empty">Belum ada riwayat.</div>';
  } else {
    data.forEach(e => {
      const t = new Date(e.timestamp);
      const timeStr = t.toLocaleString('id-ID', {
        day: 'numeric', month: 'short', year: 'numeric',
        hour: '2-digit', minute: '2-digit'
      });

      html += `
        <div class="timeline-detail-event">
          <div class="timeline-detail-dot"></div>
          <div class="timeline-detail-info">
            <div class="timeline-detail-title">${escapeHtml(e.title || '-')}</div>
            <div class="timeline-detail-desc">${escapeHtml(e.description || '')}</div>
            <div class="timeline-detail-time">${timeStr}</div>
          </div>
        </div>
      `;
    });
  }

  document.getElementById('timelineDetailContent').innerHTML = html;
  document.getElementById('timelineDetailModal').style.display = 'flex';
}

async function showTaskTimeline(taskId) {
  const data = await api(`/timeline/task/${taskId}`);
  if (!data) return;

  document.getElementById('timelineDetailTitle').textContent = 'Riwayat Tugas';

  let html = '';
  if (data.length === 0) {
    html = '<div class="panel-empty">Belum ada riwayat.</div>';
  } else {
    data.forEach(e => {
      const t = new Date(e.timestamp);
      const timeStr = t.toLocaleString('id-ID', {
        day: 'numeric', month: 'short', year: 'numeric',
        hour: '2-digit', minute: '2-digit'
      });

      html += `
        <div class="timeline-detail-event">
          <div class="timeline-detail-dot"></div>
          <div class="timeline-detail-info">
            <div class="timeline-detail-title">${escapeHtml(e.title || '-')}</div>
            <div class="timeline-detail-desc">${escapeHtml(e.description || '')}</div>
            <div class="timeline-detail-time">${timeStr}</div>
          </div>
        </div>
      `;
    });
  }

  document.getElementById('timelineDetailContent').innerHTML = html;
  document.getElementById('timelineDetailModal').style.display = 'flex';
}

function closeTimelineDetail() {
  document.getElementById('timelineDetailModal').style.display = 'none';
}

// ============================================================
// FASE 8a: PROJECTS & KANBAN
// ============================================================

// ============================================================
// PROJECTS
// ============================================================
async function openProjects() {
  showMode('projectsMode');
  setMobileTitle('Projects');
  setupEditorBackBtn('.projects-header .editor-back');
  if (window.innerWidth <= 767) closeSidebar();

  await loadProjects();
}

function closeProjects() {
  navigateTo('list', { filter: currentFilter });
}

async function loadProjects() {
  const status = document.getElementById('projectStatusFilter').value;
  const params = new URLSearchParams();
  if (status && status !== 'all') params.set('status', status);

  const projects = await api('/projects?' + params.toString());
  const el = document.getElementById('projectsList');
  el.innerHTML = '';

  if (!projects || projects.length === 0) {
    el.innerHTML = `
      <div class="panel-empty" style="grid-column: 1/-1; padding: 60px 24px;">
        Belum ada project.<br>
        <span style="font-size:12px;color:var(--text-faint);">Klik "+ Project Baru" untuk mulai.</span>
      </div>
    `;
    return;
  }

  projects.forEach(p => {
    const card = document.createElement('div');
    card.className = 'project-card';
    card.style.borderLeftColor = p.color || '#6366f1';

    const statusLabel = {
      planning: 'Planning',
      active: 'Active',
      on_hold: 'On Hold',
      completed: 'Completed',
      cancelled: 'Cancelled'
    }[p.status] || p.status;

    const progress = p.progress || 0;

    card.innerHTML = `
      <div class="project-status-badge ${p.status}">${statusLabel}</div>
      <div class="project-card-header">
        <div class="project-card-icon" style="background:${p.color || '#6366f1'}">📁</div>
        <div class="project-card-title-group">
          <div class="project-card-name">${escapeHtml(p.name)}</div>
          <div class="project-card-desc">${escapeHtml(p.description || 'Tanpa deskripsi')}</div>
        </div>
      </div>
      <div class="project-card-progress">
        <div class="project-progress-bar">
          <div class="project-progress-fill" style="width:${progress}%"></div>
        </div>
        <div class="project-card-stats">
          <span>📊 ${progress}% selesai</span>
          <span>✓ ${p.task_done || 0}/${p.task_count || 0} tugas</span>
          <span>📄 ${p.note_count || 0} catatan</span>
        </div>
      </div>
    `;

    card.onclick = () => openProjectDetail(p.id);
    el.appendChild(card);
  });
}

async function openProjectDetail(id) {
  currentProjectId = id;

  document.getElementById('projectsMode').style.display = 'none';
  document.getElementById('projectDetailMode').style.display = 'flex';

  await loadProjectDetail();
}

async function loadProjectDetail() {
  const project = await api('/projects/' + currentProjectId);
  if (!project) return;

  document.getElementById('projectDetailIcon').textContent = '📁';
  document.getElementById('projectDetailIcon').style.background = project.color || '#6366f1';
  document.getElementById('projectDetailName').textContent = project.name;
  document.getElementById('projectDetailDesc').textContent = project.description || 'Tanpa deskripsi';

  switchProjectTab(currentProjectTab);
}

function switchProjectTab(tab) {
  currentProjectTab = tab;

  document.querySelectorAll('.project-tab').forEach(t => {
    t.classList.toggle('active', t.dataset.tab === tab);
  });

  ['overview', 'tasks', 'notes', 'activity'].forEach(t => {
    const panel = document.getElementById('projectPanel' + t.charAt(0).toUpperCase() + t.slice(1));
    if (panel) panel.style.display = (t === tab) ? 'block' : 'none';
  });

  if (tab === 'overview') loadProjectOverview();
  if (tab === 'tasks') loadProjectTasks();
  if (tab === 'notes') loadProjectNotes();
  if (tab === 'activity') loadProjectActivity();
}

async function loadProjectOverview() {
  const panel = document.getElementById('projectPanelOverview');
  const project = await api('/projects/' + currentProjectId);
  if (!project) return;

  const progress = project.progress || 0;
  const autoProgress = project.auto_progress || 0;

  const statusLabel = {
    planning: 'Planning',
    active: 'Active',
    on_hold: 'On Hold',
    completed: 'Completed',
    cancelled: 'Cancelled'
  }[project.status] || project.status;

  let html = '<div class="project-overview">';

  html += `
    <div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:var(--radius-lg);padding:24px;margin-bottom:20px;">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:12px;">
        <h3 style="font-size:16px;font-weight:700;color:var(--text-strong);">Progress</h3>
        <span class="project-status-badge ${project.status}" style="position:static;">${statusLabel}</span>
      </div>
      <div class="project-progress-bar" style="height:10px;margin-bottom:12px;">
        <div class="project-progress-fill" style="width:${progress}%"></div>
      </div>
      <div style="display:flex;justify-content:space-between;font-size:12.5px;color:var(--text-muted);">
        <span><strong style="color:var(--text-strong);">${progress}%</strong> manual</span>
        <span><strong style="color:var(--text-strong);">${autoProgress}%</strong> dari tugas</span>
      </div>
    </div>
  `;

  html += `
    <div class="tasks-stats" style="padding:0;background:transparent;border:none;margin-bottom:20px;">
      <div class="stat-card info">
        <div class="stat-value">${project.note_count || 0}</div>
        <div class="stat-label">Catatan</div>
      </div>
      <div class="stat-card warning">
        <div class="stat-value">${project.task_count || 0}</div>
        <div class="stat-label">Tugas</div>
      </div>
      <div class="stat-card success">
        <div class="stat-value">${project.task_done || 0}</div>
        <div class="stat-label">Selesai</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">${project.task_count - project.task_done || 0}</div>
        <div class="stat-label">Tersisa</div>
      </div>
    </div>
  `;

  if (project.start_date || project.due_date) {
    html += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:var(--radius-lg);padding:18px 20px;margin-bottom:20px;display:flex;gap:24px;flex-wrap:wrap;">';
    if (project.start_date) {
      html += `<div><div style="font-size:11px;color:var(--text-muted);text-transform:uppercase;font-weight:600;margin-bottom:4px;">Mulai</div><div style="font-size:14px;color:var(--text-strong);font-weight:600;">${formatDate(project.start_date)}</div></div>`;
    }
    if (project.due_date) {
      html += `<div><div style="font-size:11px;color:var(--text-muted);text-transform:uppercase;font-weight:600;margin-bottom:4px;">Target Selesai</div><div style="font-size:14px;color:var(--text-strong);font-weight:600;">${formatDate(project.due_date)}</div></div>`;
    }
    html += '</div>';
  }

  html += `
    <div style="display:flex;gap:8px;flex-wrap:wrap;">
      <button class="btn-ghost-sm" onclick="editCurrentProject()">✎ Edit Project</button>
      <button class="btn-ghost-sm" onclick="quickAddTaskToProject()">+ Tugas Baru</button>
      <button class="btn-ghost-sm" onclick="quickAddNoteToProject()">+ Catatan Baru</button>
    </div>
  `;

  html += '</div>';
  panel.innerHTML = html;
}

async function loadProjectTasks() {
  const panel = document.getElementById('projectPanelTasks');
  const project = await api('/projects/' + currentProjectId);
  if (!project || !project.tasks || project.tasks.length === 0) {
    panel.innerHTML = `
      <div class="panel-empty" style="padding:60px 24px;">
        Belum ada tugas di project ini.<br>
        <button class="btn-primary" style="margin-top:12px;" onclick="quickAddTaskToProject()">+ Tugas Baru</button>
      </div>
    `;
    return;
  }

  let html = '<div style="display:flex;flex-direction:column;gap:8px;">';

  project.tasks.forEach(t => {
    const priorityLabel = {
      low: 'Rendah', medium: 'Sedang', high: 'Tinggi', urgent: 'Urgent'
    }[t.priority] || t.priority;

    html += `
      <div class="task-card priority-${t.priority} status-${t.status}" onclick="editTask(${t.id})">
        <div class="task-checkbox ${t.status === 'done' ? 'checked' : ''}" onclick="event.stopPropagation(); toggleTask(${t.id}).then(() => loadProjectTasks())"></div>
        <div class="task-main">
          <div class="task-title">${escapeHtml(t.title)}</div>
          <div class="task-meta">
            <span class="task-badge ${t.priority}">${priorityLabel}</span>
            ${t.status === 'done' ? '<span class="task-badge" style="background:rgba(22,163,74,0.15);color:#16a34a;">Selesai</span>' : ''}
          </div>
        </div>
      </div>
    `;
  });

  html += '</div>';
  panel.innerHTML = html;
}

async function loadProjectNotes() {
  const panel = document.getElementById('projectPanelNotes');
  const project = await api('/projects/' + currentProjectId);
  if (!project || !project.notes || project.notes.length === 0) {
    panel.innerHTML = `
      <div class="panel-empty" style="padding:60px 24px;">
        Belum ada catatan di project ini.<br>
        <button class="btn-primary" style="margin-top:12px;" onclick="quickAddNoteToProject()">+ Catatan Baru</button>
      </div>
    `;
    return;
  }

  let html = '<div style="display:flex;flex-direction:column;gap:8px;">';

  project.notes.forEach(n => {
    html += `
      <div class="backlink-item" onclick="openNote(${n.id})">
        <div class="backlink-icon">📄</div>
        <div class="backlink-info">
          <div class="backlink-title">${escapeHtml(n.title)}</div>
          <div class="backlink-meta">${formatDate(n.updated_at)}</div>
        </div>
      </div>
    `;
  });

  html += '</div>';
  panel.innerHTML = html;
}

async function loadProjectActivity() {
  const panel = document.getElementById('projectPanelActivity');
  const project = await api('/projects/' + currentProjectId);
  if (!project || !project.activities || project.activities.length === 0) {
    panel.innerHTML = '<div class="panel-empty" style="padding:60px 24px;">Belum ada aktivitas.</div>';
    return;
  }

  let html = '<div class="activity-list" style="padding:0;">';

  project.activities.forEach(a => {
    const initials = a.action.substring(0, 2).toUpperCase();
    html += `
      <div class="activity-item">
        <div class="activity-icon">${initials}</div>
        <div class="activity-info">
          <div class="activity-desc">${escapeHtml(a.description || a.action)}</div>
          <div class="activity-time">${formatDate(a.created_at)} · @${escapeHtml(a.username)}</div>
        </div>
      </div>
    `;
  });

  html += '</div>';
  panel.innerHTML = html;
}

function closeProjectDetail() {
  currentProjectId = null;
  navigateTo('projects');
}

// ---------- Project CRUD ----------
function showCreateProjectModal() {
  document.getElementById('projectModalTitle').textContent = 'Project Baru';
  document.getElementById('projectModalId').value = '';
  document.getElementById('projectModalName').value = '';
  document.getElementById('projectModalDesc').value = '';
  document.getElementById('projectModalStart').value = '';
  document.getElementById('projectModalDue').value = '';

  document.querySelectorAll('#projectColorPicker .accent-dot').forEach((d, i) => {
    d.classList.toggle('active', i === 0);
  });

  document.getElementById('projectModal').style.display = 'flex';
}

function closeProjectModal() {
  document.getElementById('projectModal').style.display = 'none';
}

async function saveProject() {
  const id = document.getElementById('projectModalId').value;
  const name = document.getElementById('projectModalName').value.trim();
  if (!name) {
    showToast('Nama project wajib diisi.', 'error');
    return;
  }

  const color = document.querySelector('#projectColorPicker .accent-dot.active')?.dataset.color || '#6366f1';

  const payload = {
    name,
    description: document.getElementById('projectModalDesc').value.trim(),
    color,
    start_date: document.getElementById('projectModalStart').value || null,
    due_date: document.getElementById('projectModalDue').value || null
  };

  let res;
  if (id) {
    res = await api('/projects/' + id, { method: 'PUT', body: JSON.stringify(payload) });
  } else {
    res = await api('/projects', { method: 'POST', body: JSON.stringify(payload) });
  }

  if (res) {
    showToast(id ? 'Project diperbarui.' : 'Project dibuat.', 'success');
    closeProjectModal();
    if (id && currentProjectId == id) {
      loadProjectDetail();
    } else {
      loadProjects();
    }
  } else {
    showToast('Gagal menyimpan project.', 'error');
  }
}

function editCurrentProject() {
  api('/projects/' + currentProjectId).then(p => {
    if (!p) return;
    document.getElementById('projectModalTitle').textContent = 'Edit Project';
    document.getElementById('projectModalId').value = p.id;
    document.getElementById('projectModalName').value = p.name || '';
    document.getElementById('projectModalDesc').value = p.description || '';
    document.getElementById('projectModalStart').value = p.start_date || '';
    document.getElementById('projectModalDue').value = p.due_date || '';

    document.querySelectorAll('#projectColorPicker .accent-dot').forEach(d => {
      d.classList.toggle('active', d.dataset.color === p.color);
    });

    document.getElementById('projectModal').style.display = 'flex';
  });
}

async function deleteCurrentProject() {
  if (!confirm('Hapus project ini?\n\nCatatan & tugas akan tetap ada, tapi tidak terkait project lagi.')) return;

  await api('/projects/' + currentProjectId, { method: 'DELETE' });
  showToast('Project dihapus.', 'success');
  closeProjectDetail();
}

function quickAddTaskToProject() {
  showCreateTaskModal();
  setTimeout(() => {
    showToast('Buat task dan pilih project "' + document.getElementById('projectDetailName').textContent + '"', 'info');
  }, 200);
}

function quickAddNoteToProject() {
  createNote();
}

// ============================================================
// KANBAN BOARDS
// ============================================================
async function openBoards() {
  showMode('boardsMode');
  setMobileTitle('Kanban');
  setupEditorBackBtn('.boards-header .editor-back');
  if (window.innerWidth <= 767) closeSidebar();

  await loadBoards();
  await populateBoardProjectDropdown();
}

function closeBoards() {
  navigateTo('list', { filter: currentFilter });
}

async function loadBoards() {
  const boards = await api('/boards');
  const el = document.getElementById('boardsList');
  el.innerHTML = '';

  if (!boards || boards.length === 0) {
    el.innerHTML = `
      <div class="panel-empty" style="grid-column:1/-1;padding:60px 24px;">
        Belum ada board.<br>
        <span style="font-size:12px;color:var(--text-faint);">Klik "+ Board Baru" untuk mulai.</span>
      </div>
    `;
    return;
  }

  boards.forEach(b => {
    const card = document.createElement('div');
    card.className = 'board-card';
    card.innerHTML = `
      <button class="item-delete-btn" title="Hapus board">✕</button>
      <div class="board-card-name">${escapeHtml(b.name)}</div>
      ${b.description ? `<div class="board-card-desc">${escapeHtml(b.description)}</div>` : ''}
      <div class="board-card-stats">
        <span>📋 ${b.column_count || 0} kolom</span>
        <span>🎴 ${b.card_count || 0} kartu</span>
      </div>
    `;

    card.querySelector('.item-delete-btn').onclick = (e) => {
      e.stopPropagation();
      deleteBoard(b.id, b.name);
    };

    card.onclick = () => openBoardDetail(b.id);
    el.appendChild(card);
  });
}

async function deleteBoard(id, name) {
  if (!confirm(`Hapus board "${name}"?\n\nSemua kolom & kartu di dalamnya akan terhapus permanen.`)) return;

  const res = await api('/boards/' + id, { method: 'DELETE' });
  if (res) {
    showToast('Board dihapus.', 'success');
    loadBoards();
  } else {
    showToast('Gagal menghapus board.', 'error');
  }
}

async function openBoardDetail(id) {
  currentBoardId = id;

  document.getElementById('boardsMode').style.display = 'none';
  document.getElementById('boardDetailMode').style.display = 'flex';

  await loadBoardDetail();
}

async function loadBoardDetail() {
  const board = await api('/boards/' + currentBoardId);
  if (!board) return;

  document.getElementById('boardDetailName').textContent = board.name;
  document.getElementById('boardDetailDesc').textContent = board.description || '';

  const canvas = document.getElementById('boardCanvas');
  canvas.innerHTML = '';

  (board.columns || []).forEach(col => {
    const colEl = document.createElement('div');
    colEl.className = 'kanban-column-v2';
    colEl.style.borderTopColor = col.color || '#6366f1';
    colEl.dataset.columnId = col.id;

    let cardsHtml = '';
    (col.cards || []).forEach(card => {
      cardsHtml += renderKanbanCard(card);
    });

    colEl.innerHTML = `
      <div class="kanban-column-header-v2">
        <div class="kanban-column-name-v2">
          <span class="kanban-column-dot" style="background:${col.color || '#6366f1'}"></span>
          ${escapeHtml(col.name)}
          <span class="kanban-column-count-v2">${(col.cards || []).length}</span>
        </div>
        <div class="kanban-column-actions">
          <button onclick="event.stopPropagation(); editColumn(${col.id})" title="Edit">✎</button>
          <button onclick="event.stopPropagation(); deleteColumn(${col.id})" title="Hapus">🗑</button>
        </div>
      </div>
      <div class="kanban-column-body-v2" data-column-id="${col.id}">
        ${cardsHtml}
      </div>
      <button class="kanban-add-card" onclick="openAddCard(${col.id})">+ Tambah kartu</button>
    `;

    canvas.appendChild(colEl);
  });

  const addCol = document.createElement('button');
  addCol.className = 'kanban-add-column';
  addCol.innerHTML = '+ Tambah kolom';
  addCol.onclick = () => showCreateColumnModal();
  canvas.appendChild(addCol);

  setupKanbanDragDrop();
}

function renderKanbanCard(card) {
  const labels = (card.labels || '').split(',').filter(l => l.trim());

  let labelsHtml = '';
  if (labels.length > 0) {
    labelsHtml = '<div class="kanban-card-labels">' +
      labels.slice(0, 3).map(l => `<span class="kanban-card-label">${escapeHtml(l.trim())}</span>`).join('') +
      '</div>';
  }

  let dueHtml = '';
  if (card.due_date) {
    const d = new Date(card.due_date);
    const isOverdue = d < new Date();
    dueHtml = `<span class="kanban-card-due ${isOverdue ? 'overdue' : ''}">${d.toLocaleDateString('id-ID', { day: 'numeric', month: 'short' })}</span>`;
  }

  return `
    <div class="kanban-card-v2 priority-${card.priority || 'medium'}" 
         draggable="true" 
         data-card-id="${card.id}"
         onclick="editCard(${card.id})">
      <button class="kanban-card-delete" onclick="event.stopPropagation(); deleteCard(${card.id})">✕</button>
      <div class="kanban-card-title-v2">${escapeHtml(card.title)}</div>
      ${card.description ? `<div class="kanban-card-desc-v2">${escapeHtml(card.description)}</div>` : ''}
      ${labelsHtml}
      ${dueHtml ? `<div class="kanban-card-footer">${dueHtml}</div>` : ''}
    </div>
  `;
}

function setupKanbanDragDrop() {
  const cards = document.querySelectorAll('.kanban-card-v2');
  const columns = document.querySelectorAll('.kanban-column-body-v2');

  cards.forEach(card => {
    card.addEventListener('dragstart', e => {
      e.dataTransfer.setData('card_id', card.dataset.cardId);
      card.classList.add('dragging');
    });

    card.addEventListener('dragend', () => {
      card.classList.remove('dragging');
    });
  });

  columns.forEach(col => {
    col.addEventListener('dragover', e => {
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
      col.closest('.kanban-column-v2').classList.add('drag-over');
    });

    col.addEventListener('dragleave', e => {
      if (!col.contains(e.relatedTarget)) {
        col.closest('.kanban-column-v2').classList.remove('drag-over');
      }
    });

    col.addEventListener('drop', async e => {
      e.preventDefault();
      col.closest('.kanban-column-v2').classList.remove('drag-over');

      const cardId = e.dataTransfer.getData('card_id');
      const columnId = col.dataset.columnId;

      if (!cardId || !columnId) return;

      await api('/cards/' + cardId + '/move', {
        method: 'POST',
        body: JSON.stringify({
          column_id: parseInt(columnId),
          position: 0
        })
      });

      await loadBoardDetail();
    });
  });
}

function closeBoardDetail() {
  currentBoardId = null;
  navigateTo('boards');
}

// ---------- Board CRUD ----------
function showCreateBoardModal() {
  document.getElementById('boardModalTitle').textContent = 'Board Baru';
  document.getElementById('boardModalId').value = '';
  document.getElementById('boardModalName').value = '';
  document.getElementById('boardModalDesc').value = '';
  document.getElementById('boardModalProject').value = '';

  document.getElementById('boardModal').style.display = 'flex';
}

function closeBoardModal() {
  document.getElementById('boardModal').style.display = 'none';
}

async function populateBoardProjectDropdown() {
  const projects = await api('/projects');
  const select = document.getElementById('boardModalProject');
  if (!select) return;
  select.innerHTML = '<option value="">— Tidak ada —</option>';
  if (!projects) return;
  projects.forEach(p => {
    const opt = document.createElement('option');
    opt.value = p.id;
    opt.textContent = p.name;
    select.appendChild(opt);
  });
}

async function saveBoard() {
  const id = document.getElementById('boardModalId').value;
  const name = document.getElementById('boardModalName').value.trim();
  if (!name) {
    showToast('Nama board wajib diisi.', 'error');
    return;
  }

  const projectVal = document.getElementById('boardModalProject').value;

  const payload = {
    name,
    description: document.getElementById('boardModalDesc').value.trim()
  };
  if (projectVal) payload.project_id = parseInt(projectVal);

  let res;
  if (id) {
    res = await api('/boards/' + id, { method: 'PUT', body: JSON.stringify(payload) });
  } else {
    res = await api('/boards', { method: 'POST', body: JSON.stringify(payload) });
  }

  if (res) {
    showToast(id ? 'Board diperbarui.' : 'Board dibuat.', 'success');
    closeBoardModal();
    loadBoards();
  }
}

function editCurrentBoard() {
  api('/boards/' + currentBoardId).then(b => {
    if (!b) return;
    document.getElementById('boardModalTitle').textContent = 'Edit Board';
    document.getElementById('boardModalId').value = b.id;
    document.getElementById('boardModalName').value = b.name;
    document.getElementById('boardModalDesc').value = b.description || '';
    document.getElementById('boardModalProject').value = b.project_id || '';
    document.getElementById('boardModal').style.display = 'flex';
  });
}

// ---------- Column CRUD ----------
function showCreateColumnModal() {
  document.getElementById('columnModalName').value = '';
  document.querySelectorAll('#columnColorPicker .accent-dot').forEach((d, i) => {
    d.classList.toggle('active', i === 0);
  });
  document.getElementById('columnModal').style.display = 'flex';
  setTimeout(() => document.getElementById('columnModalName').focus(), 100);
}

function closeColumnModal() {
  document.getElementById('columnModal').style.display = 'none';
}

async function saveColumn() {
  const name = document.getElementById('columnModalName').value.trim();
  if (!name) {
    showToast('Nama kolom wajib diisi.', 'error');
    return;
  }

  const color = document.querySelector('#columnColorPicker .accent-dot.active')?.dataset.color || '#6366f1';

  const res = await api('/boards/' + currentBoardId + '/columns', {
    method: 'POST',
    body: JSON.stringify({ name, color })
  });

  if (res) {
    showToast('Kolom dibuat.', 'success');
    closeColumnModal();
    loadBoardDetail();
  }
}

async function editColumn(id) {
  const board = await api('/boards/' + currentBoardId);
  if (!board) return;
  const col = (board.columns || []).find(c => c.id === id);
  if (!col) return;

  const newName = prompt('Nama kolom:', col.name);
  if (!newName) return;

  await api('/columns/' + id, {
    method: 'PUT',
    body: JSON.stringify({ name: newName })
  });
  loadBoardDetail();
}

async function deleteColumn(id) {
  if (!confirm('Hapus kolom ini? Semua kartu di dalamnya akan terhapus.')) return;
  await api('/columns/' + id, { method: 'DELETE' });
  showToast('Kolom dihapus.', 'success');
  loadBoardDetail();
}

// ---------- Card CRUD ----------
function openAddCard(columnId) {
  document.getElementById('cardModalTitle').textContent = 'Card Baru';
  document.getElementById('cardModalId').value = '';
  document.getElementById('cardModalColumnId').value = columnId;
  document.getElementById('cardModalCardTitle').value = '';
  document.getElementById('cardModalCardDesc').value = '';
  document.getElementById('cardModalPriority').value = 'medium';
  document.getElementById('cardModalDue').value = '';
  document.getElementById('cardModalLabels').value = '';

  document.getElementById('cardModal').style.display = 'flex';
  setTimeout(() => document.getElementById('cardModalCardTitle').focus(), 100);
}

async function editCard(id) {
  const board = await api('/boards/' + currentBoardId);
  if (!board) return;

  let card = null;
  for (const col of (board.columns || [])) {
    const found = (col.cards || []).find(c => c.id === id);
    if (found) { card = found; break; }
  }
  if (!card) return;

  document.getElementById('cardModalTitle').textContent = 'Edit Card';
  document.getElementById('cardModalId').value = card.id;
  document.getElementById('cardModalColumnId').value = '';
  document.getElementById('cardModalCardTitle').value = card.title || '';
  document.getElementById('cardModalCardDesc').value = card.description || '';
  document.getElementById('cardModalPriority').value = card.priority || 'medium';
  document.getElementById('cardModalDue').value = toDatetimeLocal(card.due_date);
  document.getElementById('cardModalLabels').value = card.labels || '';

  document.getElementById('cardModal').style.display = 'flex';
}

function closeCardModal() {
  document.getElementById('cardModal').style.display = 'none';
}

async function saveCard() {
  const id = document.getElementById('cardModalId').value;
  const columnId = document.getElementById('cardModalColumnId').value;
  const title = document.getElementById('cardModalCardTitle').value.trim();

  if (!title) {
    showToast('Judul card wajib diisi.', 'error');
    return;
  }

  const payload = {
    title,
    description: document.getElementById('cardModalCardDesc').value.trim(),
    priority: document.getElementById('cardModalPriority').value,
    due_date: document.getElementById('cardModalDue').value || null,
    labels: document.getElementById('cardModalLabels').value.trim()
  };

  let res;
  if (id) {
    res = await api('/cards/' + id, { method: 'PUT', body: JSON.stringify(payload) });
  } else {
    res = await api('/columns/' + columnId + '/cards', { method: 'POST', body: JSON.stringify(payload) });
  }

  if (res) {
    showToast(id ? 'Card diperbarui.' : 'Card dibuat.', 'success');
    closeCardModal();
    loadBoardDetail();
  }
}

async function deleteCard(id) {
  if (!confirm('Hapus card ini?')) return;
  await api('/cards/' + id, { method: 'DELETE' });
  showToast('Card dihapus.', 'success');
  loadBoardDetail();
}

console.log('✅ 4-productivity.js loaded');