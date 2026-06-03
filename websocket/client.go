package websocket

import (
	"time"

	fiberws "github.com/gofiber/websocket/v2"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 50 * time.Second
)

type Client struct {
	hub     *Hub
	conn    *fiberws.Conn
	channel string
	send    chan []byte
}

func NewClientHandler(channel string) func(*fiberws.Conn) {
	return func(conn *fiberws.Conn) {
		client := &Client{
			hub:     DefaultHub,
			conn:    conn,
			channel: channel,
			send:    make(chan []byte, 32),
		}
		client.hub.register(client)

		done := make(chan struct{})
		go client.writePump(done)
		client.readPump()
		<-done
	}
}

func NewCustomerClientHandler() func(*fiberws.Conn) {
	return func(conn *fiberws.Conn) {
		NewClientHandler(CustomerChannel(conn.Params("order_code")))(conn)
	}
}

func (c *Client) readPump() {
	defer c.hub.unregister(c)
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
