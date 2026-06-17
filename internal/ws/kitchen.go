package ws

import (
	"log"
	"time"

	"pos-backend/models"

	fiberws "github.com/gofiber/websocket/v2"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 50 * time.Second
)

const NewOrderEvent = "NEW_ORDER"

var DefaultKitchenManager = NewManager()

type Client struct {
	manager *Manager
	conn    *fiberws.Conn
	send    chan []byte
}

type NewOrderPayload struct {
	Type         string    `json:"type"`
	OrderID      uint      `json:"order_id"`
	OrderCode    string    `json:"order_code"`
	CustomerName string    `json:"customer_name"`
	TableName    string    `json:"table_name"`
	TotalAmount  int64     `json:"total_amount"`
	CreatedAt    time.Time `json:"created_at"`
}

func NewKitchenHandler(manager *Manager) func(*fiberws.Conn) {
	if manager == nil {
		manager = DefaultKitchenManager
	}
	return func(conn *fiberws.Conn) {
		client := &Client{
			manager: manager,
			conn:    conn,
			send:    make(chan []byte, 32),
		}
		client.manager.Register(client)

		done := make(chan struct{})
		go client.writePump(done)
		client.readPump()
		<-done
	}
}

func BroadcastNewOrder(order models.Order) {
	DefaultKitchenManager.BroadcastJSON(newOrderPayload(order))
	log.Printf("Kitchen notification sent: %s", order.OrderCode)
}

func BroadcastKitchenEvent(event string, data any) {
	DefaultKitchenManager.Broadcast(event, data)
}

func newOrderPayload(order models.Order) NewOrderPayload {
	tableName := ""
	if order.Table != nil {
		tableName = order.Table.TableNumber
	}

	return NewOrderPayload{
		Type:         NewOrderEvent,
		OrderID:      order.ID,
		OrderCode:    order.OrderCode,
		CustomerName: order.CustomerName,
		TableName:    tableName,
		TotalAmount:  order.TotalAmount,
		CreatedAt:    order.CreatedAt.UTC(),
	}
}

func (c *Client) readPump() {
	defer c.manager.Unregister(c)
	c.conn.SetReadLimit(1024)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *Client) writePump(done chan<- struct{}) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
		close(done)
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(fiberws.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(fiberws.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(fiberws.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
