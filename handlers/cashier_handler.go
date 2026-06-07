package handlers

import (
	"errors"

	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CashierHandler struct {
	db *gorm.DB
}

func NewCashierHandler(db *gorm.DB) *CashierHandler {
	return &CashierHandler{db: db}
}

func (h *CashierHandler) CreateOrder(c *fiber.Ctx) error {
	var request createOrderRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return utils.Error(c, fiber.StatusUnauthorized, "authentication is required")
	}
	order, err := createOrder(h.db, request, &userID, false)
	if err != nil {
		return orderError(c, err, "failed to create order")
	}
	broadcastOrderCreated(order)
	return utils.Success(c, fiber.StatusCreated, "order created successfully", order)
}

func (h *CashierHandler) ListOrders(c *fiber.Ctx) error {
	var orders []models.Order
	if err := preloadOrder(h.db).Order("created_at DESC").Find(&orders).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get orders")
	}
	return utils.Success(c, fiber.StatusOK, "orders retrieved successfully", orders)
}

func (h *CashierHandler) GetOrder(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	order, err := findOrder(h.db, id)
	if err != nil {
		return orderLookupError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "order retrieved successfully", order)
}

func (h *CashierHandler) ConfirmPayment(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return utils.Error(c, fiber.StatusUnauthorized, "authentication is required")
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Payment").First(&order, id).Error; err != nil {
			return err
		}
		if order.Status != models.OrderPendingPayment {
			return &orderServiceError{Status: 409, Message: "only pending payment order can be confirmed"}
		}
		if order.Payment == nil {
			return errors.New("payment record not found")
		}
		if order.Payment.Status == models.PaymentPaid {
			return &orderServiceError{Status: 409, Message: "payment has already been confirmed"}
		}
		return markPaymentPaidAndSendOrderToKitchen(tx, order.Payment, &order, map[string]any{
			"confirmed_by": userID,
		})
	})
	if err != nil {
		return orderError(c, err, "failed to confirm payment")
	}
	order, err := findOrder(h.db, id)
	if err != nil {
		return orderLookupError(c, err)
	}
	broadcastPaymentConfirmed(order)
	return utils.Success(c, fiber.StatusOK, "payment confirmed and order sent to kitchen", order)
}

func (h *CashierHandler) CancelOrder(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Payment").First(&order, id).Error; err != nil {
			return err
		}
		if order.Status == models.OrderCancelled {
			return &orderServiceError{Status: 409, Message: "order has already been cancelled"}
		}
		if order.Status == models.OrderCompleted {
			return &orderServiceError{Status: 409, Message: "completed order cannot be cancelled"}
		}
		if order.Payment != nil && order.Payment.Status == models.PaymentPaid {
			return &orderServiceError{Status: 409, Message: "paid order cannot be cancelled without refund"}
		}
		if order.Payment != nil {
			if err := tx.Model(order.Payment).Update("status", models.PaymentRejected).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&order).Update("status", models.OrderCancelled).Error; err != nil {
			return err
		}
		return releaseOrderTable(tx, order)
	})
	if err != nil {
		return orderError(c, err, "failed to cancel order")
	}
	order, err := findOrder(h.db, id)
	if err != nil {
		return orderLookupError(c, err)
	}
	broadcastOrderStatus("order_cancelled", order)
	return utils.Success(c, fiber.StatusOK, "order cancelled successfully", order)
}

func (h *CashierHandler) CompleteOrder(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, id).Error; err != nil {
			return err
		}
		if order.Status != models.OrderReady {
			return &orderServiceError{Status: 409, Message: "only ready order can be completed"}
		}
		if err := tx.Model(&order).Update("status", models.OrderCompleted).Error; err != nil {
			return err
		}
		return releaseOrderTable(tx, order)
	})
	if err != nil {
		return orderError(c, err, "failed to complete order")
	}
	order, err := findOrder(h.db, id)
	if err != nil {
		return orderLookupError(c, err)
	}
	broadcastOrderStatus("order_completed", order)
	return utils.Success(c, fiber.StatusOK, "order completed successfully", order)
}

func (h *CashierHandler) ListWaitingPayments(c *fiber.Ctx) error {
	var payments []models.Payment
	if err := h.db.
		Preload("Order").
		Preload("Order.Table").
		Preload("Order.Items.Menu").
		Preload("Confirmer").
		Where("status = ?", models.PaymentWaitingConfirmation).
		Order("created_at ASC").
		Find(&payments).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get waiting payments")
	}
	return utils.Success(c, fiber.StatusOK, "waiting payments retrieved successfully", payments)
}

func releaseOrderTable(tx *gorm.DB, order models.Order) error {
	if order.OrderType != models.OrderDineIn || order.TableID == nil {
		return nil
	}
	return tx.Model(&models.Table{}).Where("id = ?", *order.TableID).Update("status", models.TableAvailable).Error
}
