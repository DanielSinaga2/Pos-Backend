package handlers

import (
	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type KitchenHandler struct {
	db *gorm.DB
}

func NewKitchenHandler(db *gorm.DB) *KitchenHandler {
	return &KitchenHandler{db: db}
}

func (h *KitchenHandler) ListOrders(c *fiber.Ctx) error {
	statuses := []models.OrderStatus{
		models.OrderSentToKitchen,
		models.OrderCooking,
		models.OrderReady,
	}
	var orders []models.Order
	if err := preloadOrder(h.db).Where("status IN ?", statuses).Order("created_at ASC").Find(&orders).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get kitchen orders")
	}
	return utils.Success(c, fiber.StatusOK, "kitchen orders retrieved successfully", orders)
}

func (h *KitchenHandler) StartCooking(c *fiber.Ctx) error {
	return h.updateStatus(c, models.OrderSentToKitchen, models.OrderCooking, "order is now cooking")
}

func (h *KitchenHandler) MarkReady(c *fiber.Ctx) error {
	return h.updateStatus(c, models.OrderCooking, models.OrderReady, "order is ready")
}

func (h *KitchenHandler) updateStatus(c *fiber.Ctx, expected, next models.OrderStatus, message string) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, id).Error; err != nil {
			return err
		}
		if order.Status != expected {
			return &orderServiceError{Status: 409, Message: "invalid order status transition"}
		}
		return tx.Model(&order).Update("status", next).Error
	})
	if err != nil {
		return orderError(c, err, "failed to update order status")
	}
	order, err := findOrder(h.db, id)
	if err != nil {
		return orderLookupError(c, err)
	}
	event := "order_cooking"
	if next == models.OrderReady {
		event = "order_ready"
	}
	broadcastOrderStatus(event, order)
	return utils.Success(c, fiber.StatusOK, message, order)
}
