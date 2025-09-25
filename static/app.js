class UltraThinkApp {
    constructor() {
        this.ws = null;
        this.events = [];
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
        const wsUrl = `${protocol}//${window.location.host}/ws`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            this.updateStatus('connected');
            console.log('WebSocket connected');
        };

        this.ws.onmessage = (event) => {
            const feedEvent = JSON.parse(event.data);
            this.addEvent(feedEvent);
        };

        this.ws.onclose = () => {
            this.updateStatus('disconnected');
            console.log('WebSocket disconnected');
            setTimeout(() => this.reconnect(), 3000);
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            this.updateStatus('error');
        };
    }

    reconnect() {
        if (this.ws.readyState === WebSocket.CLOSED) {
            this.setupWebSocket();
        }
    }

    setupEventListeners() {
        const pauseBtn = document.getElementById('pauseBtn');
        const filterSelect = document.getElementById('filterSelect');
        const closeSidebar = document.getElementById('closeSidebar');
        const closeModal = document.getElementById('closeModal');

        pauseBtn.addEventListener('click', () => {
            this.togglePause();
        });

        filterSelect.addEventListener('change', (e) => {
            this.currentFilter = e.target.value;
            this.renderFeed();
        });

        closeSidebar.addEventListener('click', () => {
            this.closeSidebar();
        });

        closeModal.addEventListener('click', () => {
            this.closeModal();
        });

        // Close modal when clicking outside
        document.getElementById('fileListModal').addEventListener('click', (e) => {
            if (e.target.id === 'fileListModal') {
                this.closeModal();
            }
        });
    }

    togglePause() {
        this.isPaused = !this.isPaused;
        const pauseBtn = document.getElementById('pauseBtn');

        if (this.isPaused) {
            pauseBtn.textContent = 'Resume';
            pauseBtn.classList.add('paused');
            this.sendMessage({ action: 'pause' });
        } else {
            pauseBtn.textContent = 'Pause';
            pauseBtn.classList.remove('paused');
            this.sendMessage({ action: 'resume' });
        }
    }

    sendMessage(message) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify(message));
        }
    }

    addEvent(event) {
        this.events.unshift(event);
        if (this.events.length > 100) {
            this.events.pop();
        }
        this.renderFeed();
        this.updateEventCount();
    }

    renderFeed() {
        const feedList = document.getElementById('feedList');
        const filteredEvents = this.getFilteredEvents();

        if (filteredEvents.length === 0) {
            feedList.innerHTML = '<div class="no-events">No events yet...</div>';
            return;
        }

        feedList.innerHTML = filteredEvents.map(event => this.renderEvent(event)).join('');

        // Add event listeners to clickable elements
        this.setupClickableElements();
    }

    setupClickableElements() {
        // Handle file list clicks
        document.querySelectorAll('.change-clickable').forEach(element => {
            element.addEventListener('click', (e) => {
                const files = JSON.parse(e.target.getAttribute('data-files'));
                this.showFileList(files);
            });
        });

        // Handle TODO.md mentions
        document.querySelectorAll('.todo-mention').forEach(element => {
            element.addEventListener('click', () => {
                this.showTodoSidebar();
            });
        });
    }

    getFilteredEvents() {
        if (this.currentFilter === 'all') {
            return this.events;
        }
        return this.events.filter(event => event.type === this.currentFilter);
    }

    renderEvent(event) {
        const timestamp = new Date(event.timestamp).toLocaleTimeString();

        // Make change clickable if there are multiple files
        let changeElement = this.escapeHtml(event.change);
        const hasMultipleFiles = event.files && event.files.length > 1;

        if (hasMultipleFiles) {
            changeElement = `<span class="change-clickable" data-files='${JSON.stringify(event.files)}'>${changeElement}</span>`;
        }

        // Make TODO.md mentions clickable
        if (event.change && event.change.includes('TODO.md')) {
            changeElement = changeElement.replace(/TODO\.md/g, '<span class="todo-mention">TODO.md</span>');
        }

        return `
            <div class="feed-entry">
                <div class="entry-header">
                    <span class="entry-type ${event.type}">${event.type}</span>
                    <span class="entry-timestamp">${timestamp}</span>
                </div>
                <div class="entry-content">
                    <div class="intent">${this.escapeHtml(event.intent)}</div>
                    <div class="change">${changeElement}</div>
                    <div class="result ${event.result}">${event.result}</div>
                </div>
            </div>
        `;
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    updateStatus(status) {
        const statusEl = document.getElementById('status');
        statusEl.textContent = status.charAt(0).toUpperCase() + status.slice(1);
        statusEl.className = status;
    }

    updateEventCount() {
        const eventCountEl = document.getElementById('eventCount');
        const count = this.events.length;
        eventCountEl.textContent = `${count} event${count !== 1 ? 's' : ''}`;
    }

    async loadCurrentObjective() {
        try {
            const response = await fetch('/api/objective');
            const data = await response.json();
            document.getElementById('currentObjective').textContent = data.objective;
        } catch (error) {
            console.error('Failed to load objective:', error);
            document.getElementById('currentObjective').textContent = 'Failed to load objective';
        }
    }

    showFileList(files) {
        const fileList = document.getElementById('fileList');
        fileList.innerHTML = files.map(file =>
            `<div class="file-item">${this.escapeHtml(file)}</div>`
        ).join('');

        document.getElementById('fileListModal').classList.add('show');
    }

    closeModal() {
        document.getElementById('fileListModal').classList.remove('show');
    }

    async showTodoSidebar() {
        const sidebar = document.getElementById('todoSidebar');
        const todoContent = document.getElementById('todoContent');

        sidebar.classList.add('open');
        todoContent.textContent = 'Loading...';

        try {
            const response = await fetch('/api/todo-content');
            const data = await response.json();
            todoContent.textContent = data.content || 'No content found';
        } catch (error) {
            console.error('Failed to load TODO.md content:', error);
            todoContent.textContent = 'Failed to load TODO.md content';
        }
    }

    closeSidebar() {
        document.getElementById('todoSidebar').classList.remove('open');
    }
}

document.addEventListener('DOMContentLoaded', () => {
    new UltraThinkApp();
});