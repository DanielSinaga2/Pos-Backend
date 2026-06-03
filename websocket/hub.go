package websocket

import (
	"encoding/json"
	"sync"
)

type Event struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

type Hub struct {
	mu       sync.RWMutex
	channels map[string]map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{
		channels: make(map[string]map[*Client]struct{}),
	}
}

func (h *Hub) register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.channels[client.channel] == nil {
		h.channels[client.channel] = make(map[*Client]struct{})
	}
	h.channels[client.channel][client] = struct{}{}
}

func (h *Hub) unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	clients := h.channels[client.channel]
	if _, exists := clients[client]; !exists {
		return
	}
	delete(clients, client)
	close(client.send)
	if len(clients) == 0 {
		delete(h.channels, client.channel)
	}
}

func (h *Hub) Broadcast(channel, event string, data any) {
	message, err := json.Marshal(Event{Event: event, Data: data})
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.channels[channel] {
		select {
		case client.send <- message:
		default:
			// A slow client is disconnected so it cannot block realtime updates.
			go h.unregister(client)
		}
	}
}
