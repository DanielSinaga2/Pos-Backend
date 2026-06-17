package handlers

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"pos-backend/models"
	"pos-backend/services"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CashierHandler struct {
	db       *gorm.DB
	midtrans *services.MidtransService
}

func NewCashierHandler(db *gorm.DB, midtrans *services.MidtransService) *CashierHandler {
	return &CashierHandler{db: db, midtrans: midtrans}
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
	return utils.Success(c, fiber.StatusCreated, "order created successfully", cashierOrderResponse(order))
}

func (h *CashierHandler) ListOrders(c *fiber.Ctx) error {
	var orders []models.Order
	if err := preloadOrder(h.db).Order("created_at DESC").Find(&orders).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get orders")
	}

	var response []fiber.Map
	for _, order := range orders {
		response = append(response, cashierOrderResponse(order))
	}

	return utils.Success(c, fiber.StatusOK, "orders retrieved successfully", response)
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
	return utils.Success(c, fiber.StatusOK, "order retrieved successfully", cashierOrderResponse(order))
}

func (h *CashierHandler) CheckPaymentStatus(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	order, err := findOrder(h.db, id)
	if err != nil {
		return orderLookupError(c, err)
	}

	if shouldSyncMidtransPayment(order) {
		order, err = syncMidtransPaymentByOrderCode(h.db, h.midtrans, order.OrderCode)
		if err != nil {
			return orderError(c, err, "failed to sync payment status")
		}
		broadcastMidtransPaymentUpdate(order)
	}

	return utils.Success(c, fiber.StatusOK, "payment status retrieved successfully", cashierOrderResponse(order))
}

func (h *CashierHandler) RetryPayment(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	order, err := findOrder(h.db, id)
	if err != nil {
		return orderLookupError(c, err)
	}
	if order.Payment == nil {
		return utils.Error(c, fiber.StatusNotFound, "payment not found")
	}
	if order.Payment.PaymentMethod == models.PaymentCash {
		return utils.Error(c, fiber.StatusBadRequest, "cash payment does not need online payment retry")
	}
	if !models.IsOnlinePaymentMethod(order.Payment.PaymentMethod) {
		return utils.Error(c, fiber.StatusBadRequest, "payment retry is only available for qris or online payment")
	}
	if order.Payment.Status == models.PaymentPaid {
		return utils.Error(c, fiber.StatusConflict, "payment has already been paid")
	}

	if hasValue(order.Payment.SnapToken) && hasValue(order.Payment.MidtransOrderID) {
		statusResponse, err := h.midtrans.GetTransactionStatus(strings.TrimSpace(*order.Payment.MidtransOrderID))
		if err == nil {
			switch statusResponse.TransactionStatus {
			case "settlement", "capture":
				order, err = syncMidtransPaymentByOrderCode(h.db, h.midtrans, order.OrderCode)
				if err != nil {
					return orderError(c, err, "failed to sync payment status")
				}
				broadcastMidtransPaymentUpdate(order)
				return utils.Success(c, fiber.StatusOK, "payment already paid", paymentRetryResponse(order))
			case "pending":
				return utils.Success(c, fiber.StatusOK, "snap token already exists", paymentRetryResponse(order))
			}
		}
	}

	if hasValue(order.Payment.SnapToken) && !isFinalPaymentStatus(order.Payment.Status) {
		return utils.Success(c, fiber.StatusOK, "snap token already exists", paymentRetryResponse(order))
	}

	retryOrderID := fmt.Sprintf("%s-R%d", order.OrderCode, time.Now().Unix())
	order.Payment.MidtransOrderID = &retryOrderID

	snapResponse, err := h.midtrans.CreateSnapTransaction(order, *order.Payment)
	if err != nil {
		return utils.Error(c, fiber.StatusBadGateway, "failed to create snap token")
	}

	if err := h.db.Model(order.Payment).Updates(map[string]any{
		"midtrans_order_id": retryOrderID,
		"snap_token":        snapResponse.Token,
		"snap_redirect_url": snapResponse.RedirectURL,
		"payment_type":      midtransPaymentType(order.Payment.PaymentMethod),
		"status":            models.PaymentPending,
	}).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to save snap token")
	}

	order, err = findOrder(h.db, id)
	if err != nil {
		return orderLookupError(c, err)
	}

	return utils.Success(c, fiber.StatusOK, "snap token created", paymentRetryResponse(order))
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

func (h *CashierHandler) ConfirmCashPayment(c *fiber.Ctx) error {
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
		if order.Payment == nil {
			return &orderServiceError{Status: fiber.StatusNotFound, Message: "payment not found"}
		}
		if order.Payment.PaymentMethod != models.PaymentCash {
			return &orderServiceError{Status: fiber.StatusBadRequest, Message: "cash payment confirmation is only available for cash payment"}
		}
		if order.Payment.Status == models.PaymentPaid {
			return &orderServiceError{Status: fiber.StatusConflict, Message: "payment has already been paid"}
		}
		if order.Status != models.OrderPendingPayment {
			return &orderServiceError{Status: fiber.StatusConflict, Message: "only waiting payment order can be confirmed"}
		}

		return markPaymentPaidAndSendOrderToKitchen(tx, order.Payment, &order, map[string]any{
			"confirmed_by": userID,
		})
	})
	if err != nil {
		return orderError(c, err, "failed to confirm cash payment")
	}

	order, err := findOrder(h.db, id)
	if err != nil {
		return orderLookupError(c, err)
	}
	broadcastCashPaymentConfirmed(order)

	return utils.Success(c, fiber.StatusOK, "cash payment confirmed and order sent to kitchen", fiber.Map{
		"order":          cashierOrderResponse(order),
		"payment_status": models.PaymentPaid,
		"order_status":   models.OrderSentToKitchen,
	})
}

func (h *CashierHandler) CancelOrder(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		log.Printf("CANCEL ORDER FAILED order_id=%s error=%v", c.Params("id"), err)
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Payment").First(&order, id).Error; err != nil {
			return err
		}
		if order.Status == models.OrderCancelled {
			return &orderServiceError{Status: fiber.StatusBadRequest, Message: "Pesanan sudah dibatalkan"}
		}
		if !canCancelOrder(order) {
			return &orderServiceError{Status: fiber.StatusBadRequest, Message: "Pesanan sudah diproses dan tidak dapat dibatalkan"}
		}
		if order.Payment != nil && canCancelPayment(order.Payment.Status) {
			if err := tx.Model(order.Payment).Update("status", models.PaymentCancelled).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&order).Update("status", models.OrderCancelled).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		log.Printf("CANCEL ORDER FAILED order_id=%d error=%v", id, err)
		return orderError(c, err, "failed to cancel order")
	}
	order, err := findOrder(h.db, id)
	if err != nil {
		log.Printf("CANCEL ORDER FAILED order_id=%d error=%v", id, err)
		return orderLookupError(c, err)
	}
	broadcastOrderStatus("order_cancelled", order)
	log.Printf("CANCEL ORDER SUCCESS order_id=%d order_code=%s", order.ID, order.OrderCode)
	return utils.Success(c, fiber.StatusOK, "Pesanan berhasil dibatalkan", cashierOrderResponse(order))
}

func canCancelOrder(order models.Order) bool {
	if order.Status != models.OrderPendingPayment {
		return false
	}
	if order.Payment != nil && order.Payment.Status == models.PaymentPaid {
		return false
	}
	return true
}

func canCancelPayment(status models.PaymentStatus) bool {
	return status == models.PaymentUnpaid ||
		status == models.PaymentPending ||
		status == models.PaymentWaitingConfirmation ||
		status == models.PaymentRejected
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
		return nil
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

func cashierOrderResponse(order models.Order) fiber.Map {
	var paymentMethod models.PaymentMethod
	var paymentStatus models.PaymentStatus
	if order.Payment != nil {
		paymentMethod = order.Payment.PaymentMethod
		paymentStatus = order.Payment.Status
	}

	return fiber.Map{
		"id":                order.ID,
		"order_code":        order.OrderCode,
		"customer_name":     order.CustomerName,
		"customer_phone":    order.CustomerPhone,
		"order_type":        order.OrderType,
		"table_id":          order.TableID,
		"table":             order.Table,
		"items":             order.Items,
		"total_amount":      order.TotalAmount,
		"payment_method":    paymentMethod,
		"payment_status":    paymentStatus,
		"order_status":      order.Status,
		"status":            order.Status,
		"paid_at":           paymentPaidAt(order.Payment),
		"snap_token":        paymentSnapToken(order.Payment),
		"payment_url":       paymentURL(order.Payment),
		"redirect_url":      paymentURL(order.Payment),
		"snap_redirect_url": paymentURL(order.Payment),
		"transaction_id":    paymentTransactionID(order.Payment),
		"payment_reference": paymentTransactionID(order.Payment),
		"payment":           order.Payment,
		"created_at":        order.CreatedAt,
		"updated_at":        order.UpdatedAt,
	}
}

func shouldSyncMidtransPayment(order models.Order) bool {
	if order.Payment == nil {
		return false
	}
	if order.Payment.PaymentMethod == models.PaymentCash {
		return false
	}
	if !models.IsOnlinePaymentMethod(order.Payment.PaymentMethod) {
		return false
	}
	if order.Payment.Status == models.PaymentPaid ||
		order.Payment.Status == models.PaymentFailed ||
		order.Payment.Status == models.PaymentCancelled {
		return false
	}
	return hasValue(order.Payment.MidtransOrderID) || hasValue(order.Payment.SnapToken)
}

func paymentPaidAt(payment *models.Payment) any {
	if payment == nil {
		return nil
	}
	return payment.PaidAt
}

func paymentTransactionID(payment *models.Payment) any {
	if payment == nil {
		return nil
	}
	return payment.TransactionID
}

func paymentSnapToken(payment *models.Payment) any {
	if payment == nil {
		return nil
	}
	return payment.SnapToken
}

func paymentURL(payment *models.Payment) any {
	if payment == nil {
		return nil
	}
	return payment.SnapRedirectURL
}

func paymentRetryResponse(order models.Order) fiber.Map {
	return fiber.Map{
		"order":             cashierOrderResponse(order),
		"snap_token":        paymentSnapToken(order.Payment),
		"payment_url":       paymentURL(order.Payment),
		"redirect_url":      paymentURL(order.Payment),
		"snap_redirect_url": paymentURL(order.Payment),
	}
}

func isFinalPaymentStatus(status models.PaymentStatus) bool {
	return status == models.PaymentPaid ||
		status == models.PaymentFailed ||
		status == models.PaymentCancelled ||
		status == models.PaymentRejected
}

func hasValue(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
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
