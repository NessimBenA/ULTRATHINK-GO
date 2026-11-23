package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
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

//go:embed static
var staticFiles embed.FS

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

const (
	historyLimit = 100
	sendBuffer   = 256
	writeWait    = 10 * time.Second
	todoSettle   = 100 * time.Millisecond
)

type Client struct {
	send chan []byte
}

type Hub struct {
	mu      sync.Mutex
	clients map[*Client]struct{}
	history [][]byte
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*Client]struct{})}
}

func (h *Hub) Register() *Client {
	client := &Client{send: make(chan []byte, sendBuffer)}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, msg := range h.history {
		client.send <- msg
	}
	h.clients[client] = struct{}{}
	return client
}

func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
	}
}

func (h *Hub) Broadcast(event FeedEvent) error {
	msg, err := json.Marshal(event)
	if err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.history = append(h.history, msg)
	if len(h.history) > historyLimit {
		h.history = h.history[1:]
	}
	for client := range h.clients {
		select {
		case client.send <- msg:
		default:
			delete(h.clients, client)
			close(client.send)
		}
	}
	return nil
}

type AgentFeed struct {
	hub  *Hub
	repo *git.Repository
}

var upgrader = websocket.Upgrader{}

func NewAgentFeed() *AgentFeed {
	feed := &AgentFeed{hub: NewHub()}

	repo, err := git.PlainOpen(".")
	if err != nil {
		log.Printf("Warning: Not a git repository: %v", err)
	}
	feed.repo = repo

	go feed.watchCommits()
	go feed.watchTodoFile()

	return feed
}

func (f *AgentFeed) publish(event FeedEvent) {
	if err := f.hub.Broadcast(event); err != nil {
		log.Printf("Broadcast error: %v", err)
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

			f.publish(event)
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
	err = watcher.Add(".")
	if err != nil {
		log.Printf("Error watching the current directory for TODO.md: %v", err)
		return
	}

	var settled <-chan time.Time
	for {
		select {
		case event := <-watcher.Events:
			if filepath.Clean(event.Name) == todoPath && event.Has(fsnotify.Write|fsnotify.Create) {
				settled = time.After(todoSettle)
			}
		case <-settled:
			settled = nil
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

			f.publish(feedEvent)
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

	client := f.hub.Register()
	go writeMessages(conn, client.send)
	defer f.hub.Unregister(client)

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func writeMessages(conn *websocket.Conn, send <-chan []byte) {
	defer conn.Close()
	for msg := range send {
		if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
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

func newHandler(feed *AgentFeed) (http.Handler, error) {
	ui, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServerFS(ui)))
	mux.HandleFunc("/ws", feed.handleWebSocket)
	mux.HandleFunc("/api/objective", feed.handleObjective)
	mux.HandleFunc("/api/todo-content", feed.handleTodoContent)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, ui, "index.html")
	})
	return checkHost(mux), nil
}

func allowedHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = strings.Trim(hostport, "[]")
	}
	return strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil
}

func checkHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHost(r.Host) {
			http.Error(w, "host not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	port := "8080"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}
	addr := flag.String("addr", net.JoinHostPort("127.0.0.1", port), "address to listen on (host:port)")
	flag.Parse()

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}

	feed := NewAgentFeed()

	handler, err := newHandler(feed)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("ULTRATHINK Agent Feed running on http://%s\n", listener.Addr())
	log.Fatal(http.Serve(listener, handler))
}
