package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func testEvent(id string) FeedEvent {
	return FeedEvent{
		ID:        id,
		Type:      EventTypeTodo,
		Timestamp: time.Now(),
		Intent:    id,
		Change:    "TODO.md modified",
		Result:    "updated",
	}
}

func pending(client *Client) [][]byte {
	var msgs [][]byte
	for {
		select {
		case msg, ok := <-client.send:
			if !ok {
				return msgs
			}
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
}

func pendingIDs(t *testing.T, client *Client) []string {
	t.Helper()
	var ids []string
	for _, msg := range pending(client) {
		var event FeedEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, event.ID)
	}
	return ids
}

func clientCount(hub *Hub) int {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	return len(hub.clients)
}

func TestHubConcurrentRegisterUnregisterBroadcast(t *testing.T) {
	hub := NewHub()
	const listeners = 4
	const broadcasters = 8
	const perBroadcaster = 25
	const churners = 8
	const rounds = 200

	stay := make([]*Client, listeners)
	for i := range stay {
		stay[i] = hub.Register()
	}

	var wg sync.WaitGroup
	for b := 0; b < broadcasters; b++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perBroadcaster; i++ {
				if err := hub.Broadcast(testEvent(fmt.Sprintf("%d-%d", b, i))); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	for c := 0; c < churners; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				client := hub.Register()
				pending(client)
				hub.Unregister(client)
				hub.Unregister(client)
			}
		}()
	}
	wg.Wait()

	total := broadcasters * perBroadcaster
	first := pendingIDs(t, stay[0])
	for i, client := range stay {
		ids := first
		if i > 0 {
			ids = pendingIDs(t, client)
		}
		if len(ids) != total {
			t.Fatalf("listener %d got %d events, want %d", i, len(ids), total)
		}
		last := make(map[string]int)
		for j, id := range ids {
			if id != first[j] {
				t.Fatalf("listener %d event %d is %s, listener 0 got %s", i, j, id, first[j])
			}
			source, seq, found := strings.Cut(id, "-")
			if !found {
				t.Fatalf("bad event id %q", id)
			}
			n, err := strconv.Atoi(seq)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if prev, ok := last[source]; ok {
				want = prev + 1
			}
			if n != want {
				t.Fatalf("listener %d got %s, want %s-%d", i, id, source, want)
			}
			last[source] = n
		}
	}

	if got := clientCount(hub); got != listeners {
		t.Fatalf("%d clients registered, want %d", got, listeners)
	}
	if got := len(hub.history); got != historyLimit {
		t.Fatalf("history holds %d events, want %d", got, historyLimit)
	}
}

func TestHubReplaysHistoryThenLiveEvents(t *testing.T) {
	hub := NewHub()
	for i := 0; i < historyLimit+5; i++ {
		if err := hub.Broadcast(testEvent(strconv.Itoa(i))); err != nil {
			t.Fatal(err)
		}
	}

	client := hub.Register()
	if err := hub.Broadcast(testEvent("live")); err != nil {
		t.Fatal(err)
	}

	ids := pendingIDs(t, client)
	if len(ids) != historyLimit+1 {
		t.Fatalf("got %d events, want %d", len(ids), historyLimit+1)
	}
	for i := 0; i < historyLimit; i++ {
		if want := strconv.Itoa(i + 5); ids[i] != want {
			t.Fatalf("event %d is %s, want %s", i, ids[i], want)
		}
	}
	if ids[historyLimit] != "live" {
		t.Fatalf("last event is %s, want live", ids[historyLimit])
	}
}

func TestHubDropsClientThatStopsReading(t *testing.T) {
	hub := NewHub()
	slow := hub.Register()

	for i := 0; i <= sendBuffer; i++ {
		if err := hub.Broadcast(testEvent(strconv.Itoa(i))); err != nil {
			t.Fatal(err)
		}
	}

	if got := clientCount(hub); got != 0 {
		t.Fatalf("%d clients registered, want the slow one dropped", got)
	}
	received := 0
	for range slow.send {
		received++
	}
	if received != sendBuffer {
		t.Fatalf("slow client got %d events before being dropped, want %d", received, sendBuffer)
	}
	hub.Unregister(slow)
}

func newTestServer(t *testing.T) (*Hub, *httptest.Server) {
	t.Helper()
	feed := &AgentFeed{hub: NewHub()}
	handler, err := newHandler(feed)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return feed.hub, server
}

func wsURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
}

func TestWebSocketClientsReceiveConcurrentBroadcasts(t *testing.T) {
	hub, server := newTestServer(t)
	const clients = 5
	const churners = 4
	const rounds = 20
	const broadcasters = 4
	const perBroadcaster = 50

	conns := make([]*websocket.Conn, clients)
	for i := range conns {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL(server), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		conns[i] = conn
	}
	deadline := time.Now().Add(5 * time.Second)
	for clientCount(hub) != clients {
		if time.Now().After(deadline) {
			t.Fatalf("%d clients registered, want %d", clientCount(hub), clients)
		}
		time.Sleep(10 * time.Millisecond)
	}

	var wg sync.WaitGroup
	for c := 0; c < churners; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				conn, _, err := websocket.DefaultDialer.Dial(wsURL(server), nil)
				if err != nil {
					t.Error(err)
					return
				}
				conn.Close()
			}
		}()
	}
	for b := 0; b < broadcasters; b++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perBroadcaster; i++ {
				if err := hub.Broadcast(testEvent(fmt.Sprintf("%d-%d", b, i))); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()

	total := broadcasters * perBroadcaster
	for i, conn := range conns {
		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatal(err)
		}
		seen := make(map[string]bool)
		for len(seen) < total {
			var event FeedEvent
			if err := conn.ReadJSON(&event); err != nil {
				t.Fatalf("client %d: %v after %d events", i, err, len(seen))
			}
			if seen[event.ID] {
				t.Fatalf("client %d got %s twice", i, event.ID)
			}
			seen[event.ID] = true
		}
	}
}

func TestRejectsCrossOriginUpgradesAndForeignHosts(t *testing.T) {
	_, server := newTestServer(t)

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL(server), http.Header{"Origin": {"http://evil.example"}})
	if err == nil {
		conn.Close()
		t.Fatal("cross-origin upgrade was accepted")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin upgrade: got %v, want status 403", resp)
	}

	conn, _, err = websocket.DefaultDialer.Dial(wsURL(server), http.Header{"Origin": {server.URL}})
	if err != nil {
		t.Fatalf("same-origin upgrade: %v", err)
	}
	conn.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/objective", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "evil.example"
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Host evil.example: got status %d, want 403", resp.StatusCode)
	}

	resp, err = http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "<title>ULTRATHINK") {
		t.Fatalf("GET /: status %d, body does not hold the embedded index.html", resp.StatusCode)
	}
}
