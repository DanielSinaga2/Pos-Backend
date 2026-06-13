package handlers

import (
	"pos-backend/models"
	realtime "pos-backend/websocket"
)

func broadcastOrderCreated(order models.Order) {
	// Selalu broadcast ke cashier agar kasir bisa lihat semua order baru.
	realtime.BroadcastToCashier("order_created", order)
	realtime.BroadcastToCustomer(order.OrderCode, "order_created", order)

	if order.Payment == nil {
		return
	}

	if order.Payment.Status == models.PaymentPaid && order.Status == models.OrderSentToKitchen {
		// Cashier manual order + CASH langsung paid dan masuk kitchen.
		realtime.BroadcastToCashier("payment_confirmed", order)
		realtime.BroadcastToKitchen("payment_confirmed", order)
		realtime.BroadcastToCustomer(order.OrderCode, "payment_confirmed", order)

		realtime.BroadcastToCashier("order_sent_to_kitchen", order)
		realtime.BroadcastToKitchen("order_sent_to_kitchen", order)
		realtime.BroadcastToCustomer(order.OrderCode, "order_sent_to_kitchen", order)
		return
	}

	switch order.Payment.Status {
	case models.PaymentWaitingConfirmation:
		// QRIS manual / transfer: muncul di antrian konfirmasi kasir.
		broadcastPaymentWaitingConfirmation(order)
	case models.PaymentUnpaid:
		// Customer QR + CASH: menunggu validasi kasir, JANGAN ke kitchen.
		realtime.BroadcastToCashier("cash_payment_pending", order)
		realtime.BroadcastToCustomer(order.OrderCode, "cash_payment_pending", order)
	}
}

func broadcastPaymentWaitingConfirmation(order models.Order) {
	realtime.BroadcastToCashier("payment_waiting_confirmation", order)
	realtime.BroadcastToCustomer(order.OrderCode, "payment_waiting_confirmation", order)
}

func broadcastCashPaymentConfirmed(order models.Order) {
	realtime.BroadcastToCashier("payment_confirmed", order)
	realtime.BroadcastToKitchen("payment_confirmed", order)
	realtime.BroadcastToCustomer(order.OrderCode, "payment_confirmed", order)

	realtime.BroadcastToCashier("order_sent_to_kitchen", order)
	realtime.BroadcastToKitchen("order_sent_to_kitchen", order)
	realtime.BroadcastToCustomer(order.OrderCode, "order_sent_to_kitchen", order)
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
