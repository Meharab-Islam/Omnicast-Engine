package signaling

import (
	"log"
	"sync"
)

// Hub maintains the set of active clients and handles registering/unregistering clients
type Hub struct {
	clients     map[*Client]bool
	register    chan *Client
	unregister  chan *Client
	roomManager *RoomManager
	mu          sync.RWMutex
}

// NewHub initializes and returns a new Hub instance
func NewHub(roomManager *RoomManager) *Hub {
	return &Hub{
		clients:     make(map[*Client]bool),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		roomManager: roomManager,
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
				close(client.Send)
				log.Printf("Client unregistered: %s (Total clients: %d)\n", client.ID, len(h.clients))
			}
			rm := h.roomManager
			h.mu.Unlock()

			// Handle Host/Viewer cleanup and room closure
			if rm != nil {
				rm.HandleClientDisconnect(client)
			}
		}
	}
}
