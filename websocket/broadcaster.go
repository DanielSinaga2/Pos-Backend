package websocket

import (
	"fmt"

	kitchenws "pos-backend/internal/ws"
)

const (
	CashierChannel = "cashier"
	KitchenChannel = "kitchen"
)

var DefaultHub = NewHub()

func CustomerChannel(orderCode string) string {
	return fmt.Sprintf("customer:%s", orderCode)
}

func BroadcastToCashier(event string, data any) {
	DefaultHub.Broadcast(CashierChannel, event, data)
}

func BroadcastToKitchen(event string, data any) {
	DefaultHub.Broadcast(KitchenChannel, event, data)
	kitchenws.BroadcastKitchenEvent(event, data)
}

func BroadcastToCustomer(orderCode, event string, data any) {
	DefaultHub.Broadcast(CustomerChannel(orderCode), event, data)
}
