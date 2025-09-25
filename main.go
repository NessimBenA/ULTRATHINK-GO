package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-git/go-git/v5"
	"github.com/gorilla/websocket"
)

type EventType string

const (
	EventTypeCommit EventType = "commit"
	EventTypeTodo   EventType = "todo"
)

type FeedEvent struct {
	ID        string    `json:"id"`
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Intent    string    `json:"intent"`
	Change    string    `json:"change"`
	Result    string    `json:"result"`
	Files     []string  `json:"files,omitempty"`
}

type AgentFeed struct {
	events    []FeedEvent
	clients   map[*websocket.Conn]bool
	broadcast chan FeedEvent
	mutex     sync.RWMutex
	paused    bool
	repo      *git.Repository
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func NewAgentFeed() *AgentFeed {
	feed := &AgentFeed{
		events:    make([]FeedEvent, 0, 100),
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan FeedEvent),
		paused:    false,
	}

	repo, err := git.PlainOpen(".")
	if err != nil {
		log.Printf("Warning: Not a git repository: %v", err)
	}
	feed.repo = repo

	go feed.handleBroadcast()
	go feed.watchCommits()
	go feed.watchTodoFile()

	return feed
}

func (f *AgentFeed) handleBroadcast() {
	for event := range f.broadcast {
		f.mutex.Lock()
		if !f.paused {
			f.events = append(f.events, event)
			if len(f.events) > 100 {
				f.events = f.events[1:]
			}
		}
		f.mutex.Unlock()

		eventJSON, _ := json.Marshal(event)
		for client := range f.clients {
			err := client.WriteMessage(websocket.TextMessage, eventJSON)
			if err != nil {
				client.Close()
				delete(f.clients, client)
			}
		}
	}
}

func (f *AgentFeed) watchCommits() {
	if f.repo == nil {
		return
	}

	lastCommitHash := ""
	for {
		time.Sleep(1 * time.Second)

		ref, err := f.repo.Head()
		if err != nil {
			continue
		}

		commit, err := f.repo.CommitObject(ref.Hash())
		if err != nil {
			continue
		}

		if commit.Hash.String() != lastCommitHash {
			lastCommitHash = commit.Hash.String()

			intent := strings.TrimSpace(commit.Message)
			if len(intent) > 80 {
				intent = intent[:80] + "..."
			}

			stats, err := commit.Stats()
			var change string
			var files []string
			if err == nil && len(stats) > 0 {
				fileCount := len(stats)
				files = make([]string, fileCount)
				for i, stat := range stats {
					files[i] = stat.Name
				}
				if fileCount == 1 {
					change = fmt.Sprintf("Modified %s", stats[0].Name)
				} else {
					change = fmt.Sprintf("Modified %d files", fileCount)
				}
			} else {
				change = "Repository updated"
			}

			event := FeedEvent{
				ID:        commit.Hash.String()[:8],
				Type:      EventTypeCommit,
				Timestamp: commit.Author.When,
				Intent:    intent,
				Change:    change,
				Result:    "success",
				Files:     files,
			}

			f.broadcast <- event
		}
	}
}

func (f *AgentFeed) watchTodoFile() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Error creating file watcher: %v", err)
		return
	}
	defer watcher.Close()

	todoPath := "TODO.md"
	if _, err := os.Stat(todoPath); os.IsNotExist(err) {
		os.WriteFile(todoPath, []byte("# TODO\n\n"), 0644)
	}

	err = watcher.Add(todoPath)
	if err != nil {
		log.Printf("Error watching TODO.md: %v", err)
		return
	}

	for {
		select {
		case event := <-watcher.Events:
			if event.Op&fsnotify.Write == fsnotify.Write {
				content, err := os.ReadFile(todoPath)
				if err != nil {
					continue
				}

				lines := strings.Split(string(content), "\n")
				intent := "TODO.md updated"
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line != "" && !strings.HasPrefix(line, "#") {
						intent = line
						if len(intent) > 80 {
							intent = intent[:80] + "..."
						}
						break
					}
				}

				feedEvent := FeedEvent{
					ID:        fmt.Sprintf("todo-%d", time.Now().Unix()),
					Type:      EventTypeTodo,
					Timestamp: time.Now(),
					Intent:    intent,
					Change:    "TODO.md modified",
					Result:    "updated",
				}

				f.broadcast <- feedEvent
			}
		case err := <-watcher.Errors:
			log.Printf("File watcher error: %v", err)
		}
	}
}

func (f *AgentFeed) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	f.mutex.Lock()
	f.clients[conn] = true
	events := make([]FeedEvent, len(f.events))
	copy(events, f.events)
	f.mutex.Unlock()

	for _, event := range events {
		eventJSON, _ := json.Marshal(event)
		conn.WriteMessage(websocket.TextMessage, eventJSON)
	}

	for {
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		if err != nil {
			f.mutex.Lock()
			delete(f.clients, conn)
			f.mutex.Unlock()
			break
		}

		if action, ok := msg["action"].(string); ok {
			switch action {
			case "pause":
				f.mutex.Lock()
				f.paused = true
				f.mutex.Unlock()
			case "resume":
				f.mutex.Lock()
				f.paused = false
				f.mutex.Unlock()
			}
		}
	}
}

func (f *AgentFeed) getCurrentObjective() string {
	content, err := os.ReadFile("TODO.md")
	if err != nil {
		return "No current objective"
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return "No current objective"
}

func (f *AgentFeed) handleObjective(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	objective := f.getCurrentObjective()
	json.NewEncoder(w).Encode(map[string]string{"objective": objective})
}

func (f *AgentFeed) handleTodoContent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	content, err := os.ReadFile("TODO.md")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"content": "Failed to read TODO.md", "error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"content": string(content)})
}

func main() {
	feed := NewAgentFeed()

	staticDir := "./static"
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		os.Mkdir(staticDir, 0755)
	}

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	http.HandleFunc("/ws", feed.handleWebSocket)
	http.HandleFunc("/api/objective", feed.handleObjective)
	http.HandleFunc("/api/todo-content", feed.handleTodoContent)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
	})

	port := "8080"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	fmt.Printf("ULTRATHINK Agent Feed running on http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}