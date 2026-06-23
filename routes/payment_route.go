package routes

import (
	"pos-backend/handlers"

	"github.com/gofiber/fiber/v2"
)

func RegisterPaymentRoutes(api fiber.Router, handler *handlers.CorePaymentHandler) {
	payment := api.Group("/payment")
	payment.Post("/create", handler.Create)
	payment.Get("/status/:order_id", handler.Status)
}
