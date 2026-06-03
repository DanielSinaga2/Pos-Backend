package websocket

import (
	"encoding/json"
	"testing"
)

func TestHubBroadcastAndUnregister(t *testing.T) {
	hub := NewHub()
	client := &Client{
		hub:     hub,
		channel: CashierChannel,
		send:    make(chan []byte, 1),
	}
	hub.register(client)

	hub.Broadcast(CashierChannel, "order_created", map[string]any{"id": float64(1)})

	var event Event
	if err := json.Unmarshal(<-client.send, &event); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if event.Event != "order_created" {
		t.Fatalf("expected order_created event, got %s", event.Event)
	}

	hub.unregister(client)
	hub.unregister(client)
	hub.Broadcast(CashierChannel, "order_created", nil)
}
