package routes

import (
	"pos-backend/handlers"

	"github.com/gofiber/fiber/v2"
)

// RegisterPaymentRoutes registers routes under /api/payment
func RegisterPaymentRoutes(api fiber.Router, handler *handlers.CorePaymentHandler) {
	payment := api.Group("/payment")
	payment.Post("/create", handler.Create)
	payment.Get("/status/:orderId", handler.Status)
	payment.Post("/notification", handler.Notification)
}
