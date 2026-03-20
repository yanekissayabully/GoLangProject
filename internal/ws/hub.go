package ws

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/google/uuid"
)

// Event represents a WebSocket event to be sent to clients.
type Event struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
	// TargetUserIDs specifies which users should receive this event.
	// If empty, event is dropped (no broadcast to all).
	TargetUserIDs []uuid.UUID `json:"-"`
}

// Hub manages all active WebSocket connections grouped by user ID.
type Hub struct {
	// connections maps userID → set of active connections
	connections map[uuid.UUID]map[*Conn]bool

	register   chan *Conn
	unregister chan *Conn
	broadcast  chan *Event

	mu     sync.RWMutex
	logger *slog.Logger
}

// NewHub creates a new Hub.
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		connections: make(map[uuid.UUID]map[*Conn]bool),
		register:    make(chan *Conn),
		unregister:  make(chan *Conn),
		broadcast:   make(chan *Event, 256),
		logger:      logger,
	}
}

// Run starts the hub's event loop. Should be called in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			if h.connections[conn.UserID] == nil {
				h.connections[conn.UserID] = make(map[*Conn]bool)
			}
			h.connections[conn.UserID][conn] = true
			h.mu.Unlock()
			h.logger.Debug("ws client registered", "user_id", conn.UserID)

		case conn := <-h.unregister:
			h.mu.Lock()
			if conns, ok := h.connections[conn.UserID]; ok {
				if _, exists := conns[conn]; exists {
					delete(conns, conn)
					close(conn.send)
					if len(conns) == 0 {
						delete(h.connections, conn.UserID)
					}
				}
			}
			h.mu.Unlock()
			h.logger.Debug("ws client unregistered", "user_id", conn.UserID)

		case event := <-h.broadcast:
			data, err := json.Marshal(event)
			if err != nil {
				h.logger.Error("ws marshal event failed", "error", err)
				continue
			}

			h.mu.RLock()
			for _, uid := range event.TargetUserIDs {
				if conns, ok := h.connections[uid]; ok {
					for conn := range conns {
						select {
						case conn.send <- data:
						default:
							// Slow client, skip
							h.logger.Warn("ws slow client, dropping message", "user_id", uid)
						}
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends an event to the target users.
func (h *Hub) Broadcast(event *Event) {
	if len(event.TargetUserIDs) == 0 {
		return
	}
	h.broadcast <- event
}

// Register adds a connection to the hub.
func (h *Hub) Register(conn *Conn) {
	h.register <- conn
}

// Unregister removes a connection from the hub.
func (h *Hub) Unregister(conn *Conn) {
	h.unregister <- conn
}

// IsUserOnline checks if a user has at least one active connection.
func (h *Hub) IsUserOnline(userID uuid.UUID) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conns, ok := h.connections[userID]
	return ok && len(conns) > 0
}
