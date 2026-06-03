package handlers

import (
	"pos-backend/models"
	realtime "pos-backend/websocket"
)

func broadcastOrderCreated(order models.Order) {
	realtime.BroadcastToCashier("order_created", order)
	realtime.BroadcastToKitchen("order_created", order)
	realtime.BroadcastToCustomer(order.OrderCode, "order_created", order)
	if order.Payment != nil && order.Payment.Status == models.PaymentWaitingConfirmation {
		broadcastPaymentWaitingConfirmation(order)
	}
}

func broadcastPaymentWaitingConfirmation(order models.Order) {
	realtime.BroadcastToCashier("payment_waiting_confirmation", order)
	realtime.BroadcastToCustomer(order.OrderCode, "payment_waiting_confirmation", order)
}

func broadcastPaymentConfirmed(order models.Order) {
	realtime.BroadcastToCashier("payment_confirmed", order)
	realtime.BroadcastToKitchen("payment_confirmed", order)
	realtime.BroadcastToCustomer(order.OrderCode, "payment_confirmed", order)

	realtime.BroadcastToCashier("order_sent_to_kitchen", order)
	realtime.BroadcastToKitchen("order_sent_to_kitchen", order)
	realtime.BroadcastToCustomer(order.OrderCode, "order_sent_to_kitchen", order)
}

func broadcastOrderStatus(event string, order models.Order) {
	realtime.BroadcastToCashier(event, order)
	realtime.BroadcastToKitchen(event, order)
	realtime.BroadcastToCustomer(order.OrderCode, event, order)
}
