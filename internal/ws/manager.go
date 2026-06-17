package ws

import (
	"encoding/json"
	"sync"
)

type Event struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

type Manager struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

func NewManager() *Manager {
	return &Manager{
		clients: make(map[*Client]struct{}),
	}
}

func (m *Manager) Register(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clients[client] = struct{}{}
}

func (m *Manager) Unregister(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clients[client]; !exists {
		return
	}
	delete(m.clients, client)
	close(client.send)
}

func (m *Manager) BroadcastJSON(payload any) {
	message, err := json.Marshal(payload)
	if err != nil {
		return
	}
	m.broadcast(message)
}

func (m *Manager) Broadcast(event string, data any) {
	m.BroadcastJSON(Event{Event: event, Data: data})
}

func (m *Manager) broadcast(message []byte) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for client := range m.clients {
		select {
		case client.send <- message:
		default:
			go m.Unregister(client)
		}
	}
}
