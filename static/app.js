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

        pauseBtn.addEventListener('click', () => {
            this.togglePause();
        });

        filterSelect.addEventListener('change', (e) => {
            this.currentFilter = e.target.value;
            this.renderFeed();
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
    }

    getFilteredEvents() {
        if (this.currentFilter === 'all') {
            return this.events;
        }
        return this.events.filter(event => event.type === this.currentFilter);
    }

    renderEvent(event) {
        const timestamp = new Date(event.timestamp).toLocaleTimeString();

        return `
            <div class="feed-entry">
                <div class="entry-header">
                    <span class="entry-type ${event.type}">${event.type}</span>
                    <span class="entry-timestamp">${timestamp}</span>
                </div>
                <div class="entry-content">
                    <div class="intent">${this.escapeHtml(event.intent)}</div>
                    <div class="change">${this.escapeHtml(event.change)}</div>
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
}

document.addEventListener('DOMContentLoaded', () => {
    new UltraThinkApp();
});