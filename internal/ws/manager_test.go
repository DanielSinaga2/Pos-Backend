package ws

import (
	"encoding/json"
	"testing"
	"time"

	"pos-backend/models"
)

func TestManagerBroadcastAndUnregister(t *testing.T) {
	manager := NewManager()
	client := &Client{
		manager: manager,
		send:    make(chan []byte, 1),
	}
	manager.Register(client)

	manager.BroadcastJSON(map[string]any{"type": NewOrderEvent})

	var payload map[string]string
	if err := json.Unmarshal(<-client.send, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["type"] != NewOrderEvent {
		t.Fatalf("expected %s event, got %s", NewOrderEvent, payload["type"])
	}

	manager.Unregister(client)
	manager.Unregister(client)
	manager.BroadcastJSON(map[string]any{"type": NewOrderEvent})
}

func TestNewOrderPayload(t *testing.T) {
	createdAt := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	order := models.Order{
		ID:           123,
		OrderCode:    "ORD-20260617-0001",
		CustomerName: "Daniel",
		Table:        &models.Table{TableNumber: "Meja 17"},
		TotalAmount:  53000,
		CreatedAt:    createdAt,
	}

	payload := newOrderPayload(order)

	if payload.Type != NewOrderEvent {
		t.Fatalf("expected %s type, got %s", NewOrderEvent, payload.Type)
	}
	if payload.OrderID != order.ID {
		t.Fatalf("expected order_id %d, got %d", order.ID, payload.OrderID)
	}
	if payload.OrderCode != order.OrderCode {
		t.Fatalf("expected order_code %s, got %s", order.OrderCode, payload.OrderCode)
	}
	if payload.CustomerName != order.CustomerName {
		t.Fatalf("expected customer_name %s, got %s", order.CustomerName, payload.CustomerName)
	}
	if payload.TableName != order.Table.TableNumber {
		t.Fatalf("expected table_name %s, got %s", order.Table.TableNumber, payload.TableName)
	}
	if payload.TotalAmount != order.TotalAmount {
		t.Fatalf("expected total_amount %d, got %d", order.TotalAmount, payload.TotalAmount)
	}
	if !payload.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected created_at %s, got %s", createdAt, payload.CreatedAt)
	}
}
