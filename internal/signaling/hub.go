package signaling

import (
	"log"
	"sync"
	"time"
)

// Hub maintains the set of active clients and handles registering/unregistering clients
type Hub struct {
	clients     map[*Client]bool
	register    chan *Client
	unregister  chan *Client
	roomManager *RoomManager
	stopGC      chan struct{}
	mu          sync.RWMutex
}

// NewHub initializes and returns a new Hub instance
func NewHub(roomManager *RoomManager) *Hub {
	return &Hub{
		clients:     make(map[*Client]bool),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		roomManager: roomManager,
		stopGC:      make(chan struct{}),
	}
}

// SetRoomManager sets the RoomManager reference for the Hub
func (h *Hub) SetRoomManager(rm *RoomManager) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.roomManager = rm
}

// Register returns the register channel
func (h *Hub) Register() chan<- *Client {
	return h.register
}

// Unregister returns the unregister channel
func (h *Hub) Unregister() chan<- *Client {
	return h.unregister
}

// ClientsCount returns the current count of connected clients (thread-safe)
func (h *Hub) ClientsCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// StartZombiePeerGC starts the background Garbage Collector goroutine that runs every interval (e.g. 10 seconds).
// If a Peer's lastPongReceived is older than timeout (e.g. 15 seconds), aggressively executes cleanup.
func (h *Hub) StartZombiePeerGC(interval, timeout time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				h.cleanupZombiePeers(timeout)
			case <-h.stopGC:
				ticker.Stop()
				return
			}
		}
	}()
	log.Printf("[Zombie GC] Started background peer GC (Interval: %v, Timeout: %v)\n", interval, timeout)
}

// StopZombiePeerGC stops the background garbage collector
func (h *Hub) StopZombiePeerGC() {
	close(h.stopGC)
}

// cleanupZombiePeers checks all connected peers and aggressively removes zombie peers
func (h *Hub) cleanupZombiePeers(timeout time.Duration) {
	h.mu.RLock()
	var zombieClients []*Client
	now := time.Now().Unix()
	timeoutSec := int64(timeout.Seconds())

	for client := range h.clients {
		if client != nil {
			lastPong := client.GetLastPong()
			if lastPong > 0 && (now-lastPong) > timeoutSec {
				zombieClients = append(zombieClients, client)
			}
		}
	}
	h.mu.RUnlock()

	for _, client := range zombieClients {
		log.Printf("[Zombie GC] Peer %s is inactive (last pong > %ds ago). Aggressively executing cleanup...\n", client.ID, timeoutSec)

		// 1. Forcefully close WebRTC PeerConnection
		client.mu.Lock()
		if client.PeerConnection != nil {
			_ = client.PeerConnection.Close()
			client.PeerConnection = nil
		}
		client.mu.Unlock()

		// 2. Forcefully close WebSocket connection
		client.writeMu.Lock()
		if client.Conn != nil {
			_ = client.Conn.Close()
		}
		client.writeMu.Unlock()

		// 3. Aggressively remove zombie peer from Room map and cancel any reconnect timers to free RAM
		if rm := h.roomManager; rm != nil {
			rm.mu.RLock()
			for roomID, room := range rm.activeRooms {
				if room.HostID == client.ID {
					go rm.CloseRoomAndNotifyWithReason(roomID, client.ID, "zombie_timeout")
				} else {
					room.CancelParticipantReconnectTimer(client.ID)
					rm.RemoveViewer(roomID, client.ID)
				}
			}
			rm.mu.RUnlock()
		}

		// 4. Remove client from Hub to free RAM and goroutines
		h.Unregister() <- client
	}
}

// Run listens on channels and manages client connections in an infinite loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("Client registered: %s (Total clients: %d)\n", client.ID, len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				log.Printf("Client unregistered: %s (Total clients: %d)\n", client.ID, len(h.clients))
			}
			rm := h.roomManager
			h.mu.Unlock()

			// Handle Host/Viewer cleanup and room closure first
			if rm != nil {
				rm.HandleClientDisconnect(client)
			}

			// Safely close client send channel after cleanup
			if client != nil {
				client.CloseSend()
			}
		}
	}
}
