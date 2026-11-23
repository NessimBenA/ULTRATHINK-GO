class UltraThinkApp {
    constructor() {
        this.ws = null;
        this.events = [];
        this.pending = [];
        this.isPaused = false;
        this.currentFilter = 'all';
        this.init();
    }

    init() {
        this.setupWebSocket();
        this.setupEventListeners();
        this.loadCurrentObjective();
        this.updateStatus('connecting');
    }

    setupWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        this.ws = new WebSocket(`${protocol}//${window.location.host}/ws`);

        this.ws.onopen = () => {
            this.events = [];
            this.pending = [];
            this.renderFeed();
            this.updateEventCount();
            this.updatePauseButton();
            this.updateStatus('connected');
        };

        this.ws.onmessage = (message) => {
            this.receive(JSON.parse(message.data));
        };

        this.ws.onclose = () => {
            this.updateStatus('disconnected');
            setTimeout(() => this.reconnect(), 3000);
        };

        this.ws.onerror = () => {
            this.updateStatus('error');
        };
    }

    reconnect() {
        if (this.ws.readyState === WebSocket.CLOSED) {
            this.setupWebSocket();
        }
    }

    setupEventListeners() {
        document.getElementById('pauseBtn').addEventListener('click', () => this.togglePause());
        document.getElementById('filterSelect').addEventListener('change', (e) => {
            this.currentFilter = e.target.value;
            this.renderFeed();
        });
        document.getElementById('closeSidebar').addEventListener('click', () => this.closeSidebar());
        document.getElementById('closeModal').addEventListener('click', () => this.closeModal());
        document.getElementById('fileListModal').addEventListener('click', (e) => {
            if (e.target.id === 'fileListModal') {
                this.closeModal();
            }
        });
    }

    receive(event) {
        if (this.isPaused) {
            this.pending.push(event);
            this.updatePauseButton();
            return;
        }
        this.store(event);
        this.renderFeed();
        this.updateEventCount();
    }

    store(event) {
        this.events.unshift(event);
        if (this.events.length > 100) {
            this.events.pop();
        }
    }

    togglePause() {
        this.isPaused = !this.isPaused;
        if (!this.isPaused) {
            this.pending.forEach((event) => this.store(event));
            this.pending = [];
            this.renderFeed();
            this.updateEventCount();
        }
        this.updatePauseButton();
    }

    updatePauseButton() {
        const pauseBtn = document.getElementById('pauseBtn');
        pauseBtn.classList.toggle('paused', this.isPaused);
        if (!this.isPaused) {
            pauseBtn.textContent = 'Pause';
        } else if (this.pending.length === 0) {
            pauseBtn.textContent = 'Resume';
        } else {
            pauseBtn.textContent = `Resume (${this.pending.length} new)`;
        }
    }

    renderFeed() {
        const feedList = document.getElementById('feedList');
        const filteredEvents = this.getFilteredEvents();
        if (filteredEvents.length === 0) {
            feedList.replaceChildren(this.element('div', 'no-events', 'No events yet...'));
            return;
        }
        feedList.replaceChildren(...filteredEvents.map((event) => this.renderEvent(event)));
    }

    getFilteredEvents() {
        if (this.currentFilter === 'all') {
            return this.events;
        }
        return this.events.filter((event) => event.type === this.currentFilter);
    }

    element(tag, className, text) {
        const node = document.createElement(tag);
        if (className) {
            node.className = className;
        }
        if (text !== undefined && text !== null) {
            node.textContent = text;
        }
        return node;
    }

    tokenClass(base, value) {
        return /^[a-z0-9-]+$/i.test(value || '') ? `${base} ${value}` : base;
    }

    renderEvent(event) {
        const header = this.element('div', 'entry-header');
        header.append(
            this.element('span', this.tokenClass('entry-type', event.type), event.type),
            this.element('span', 'entry-timestamp', new Date(event.timestamp).toLocaleTimeString())
        );

        const change = this.element('div', 'change');
        change.append(this.renderChange(event));

        const content = this.element('div', 'entry-content');
        content.append(
            this.element('div', 'intent', event.intent),
            change,
            this.element('div', this.tokenClass('result', event.result), event.result)
        );

        const entry = this.element('div', 'feed-entry');
        entry.append(header, content);
        return entry;
    }

    renderChange(event) {
        const multiple = Array.isArray(event.files) && event.files.length > 1;
        const target = multiple ? this.element('span', 'change-clickable') : document.createDocumentFragment();
        if (multiple) {
            target.addEventListener('click', () => this.showFileList(event.files));
        }
        (event.change || '').split('TODO.md').forEach((part, index) => {
            if (index > 0) {
                const mention = this.element('span', 'todo-mention', 'TODO.md');
                mention.addEventListener('click', (e) => {
                    e.stopPropagation();
                    this.showTodoSidebar();
                });
                target.append(mention);
            }
            target.append(part);
        });
        return target;
    }

    updateStatus(status) {
        const statusEl = document.getElementById('status');
        statusEl.textContent = status.charAt(0).toUpperCase() + status.slice(1);
        statusEl.className = status;
    }

    updateEventCount() {
        const count = this.events.length;
        document.getElementById('eventCount').textContent = `${count} event${count !== 1 ? 's' : ''}`;
    }

    async loadCurrentObjective() {
        const objective = document.getElementById('currentObjective');
        const response = await fetch('/api/objective');
        if (!response.ok) {
            objective.textContent = 'Failed to load objective';
            return;
        }
        const data = await response.json();
        objective.textContent = data.objective;
    }

    showFileList(files) {
        document.getElementById('fileList').replaceChildren(...files.map((file) => this.element('div', 'file-item', file)));
        document.getElementById('fileListModal').classList.add('show');
    }

    closeModal() {
        document.getElementById('fileListModal').classList.remove('show');
    }

    async showTodoSidebar() {
        const todoContent = document.getElementById('todoContent');
        document.getElementById('todoSidebar').classList.add('open');
        todoContent.textContent = 'Loading...';
        const response = await fetch('/api/todo-content');
        if (!response.ok) {
            todoContent.textContent = 'Failed to load TODO.md content';
            return;
        }
        const data = await response.json();
        todoContent.textContent = data.content || 'No content found';
    }

    closeSidebar() {
        document.getElementById('todoSidebar').classList.remove('open');
    }
}

document.addEventListener('DOMContentLoaded', () => {
    new UltraThinkApp();
});
