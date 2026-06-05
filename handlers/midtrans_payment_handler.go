package handlers

import (
	"errors"
	"strings"
	"time"

	"pos-backend/models"
	"pos-backend/services"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MidtransPaymentHandler struct {
	db       *gorm.DB
	midtrans *services.MidtransService
}

var errMidtransOrderNotFound = errors.New("midtrans notification order not found")

func NewMidtransPaymentHandler(db *gorm.DB, midtrans *services.MidtransService) *MidtransPaymentHandler {
	return &MidtransPaymentHandler{db: db, midtrans: midtrans}
}

func (h *MidtransPaymentHandler) CreateSnap(c *fiber.Ctx) error {
	orderID, err := parseID(c, "order_id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	var order models.Order
	if err := h.db.Preload("Items.Menu").Preload("Payment").First(&order, orderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "order not found")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get order")
	}
	if order.Payment == nil {
		return utils.Error(c, fiber.StatusNotFound, "payment not found")
	}
	if order.Payment.Status == models.PaymentPaid {
		return utils.Error(c, fiber.StatusConflict, "payment has already been paid")
	}
	if order.Payment.PaymentMethod == models.PaymentCash {
		return utils.Error(c, fiber.StatusBadRequest, "cash payment does not need Midtrans")
	}
	if order.Payment.SnapToken != nil && *order.Payment.SnapToken != "" {
		return utils.Success(c, fiber.StatusOK, "snap token already exists", fiber.Map{
			"order_id":     order.ID,
			"order_code":   order.OrderCode,
			"snap_token":   *order.Payment.SnapToken,
			"redirect_url": stringValue(order.Payment.SnapRedirectURL),
		})
	}

	snapResponse, err := h.midtrans.CreateSnapTransaction(order, *order.Payment)
	if err != nil {
		return utils.Error(c, fiber.StatusBadGateway, "failed to create snap token")
	}

	err = h.db.Model(order.Payment).Updates(map[string]any{
		"midtrans_order_id": order.OrderCode,
		"snap_token":        snapResponse.Token,
		"snap_redirect_url": snapResponse.RedirectURL,
	}).Error
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to save snap token")
	}

	return utils.Success(c, fiber.StatusOK, "snap token created", fiber.Map{
		"order_id":     order.ID,
		"order_code":   order.OrderCode,
		"snap_token":   snapResponse.Token,
		"redirect_url": snapResponse.RedirectURL,
	})
}

func (h *MidtransPaymentHandler) Notification(c *fiber.Ctx) error {
	var payload services.NotificationPayload
	if err := c.BodyParser(&payload); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid notification payload")
	}
	if strings.TrimSpace(payload.OrderID) == "" {
		return utils.Error(c, fiber.StatusBadRequest, "order_id is required")
	}
	if strings.HasPrefix(payload.OrderID, "payment_notif_test_") {
		return utils.Success(c, fiber.StatusOK, "midtrans test notification accepted", nil)
	}

	var processedOrderCode string
	err := h.db.Transaction(func(tx *gorm.DB) error {
		payment, order, err := findMidtransPaymentAndOrder(tx, payload.OrderID)
		if err != nil {
			return err
		}
		if !h.midtrans.VerifyNotification(payload) {
			return &orderServiceError{Status: fiber.StatusUnauthorized, Message: "invalid signature"}
		}
		processedOrderCode = order.OrderCode

		return applyMidtransTransactionStatus(
			tx,
			&payment,
			&order,
			payload.TransactionStatus,
			payload.PaymentType,
			payload.TransactionID,
			payload.FraudStatus,
		)
	})
	if err != nil {
		if errors.Is(err, errMidtransOrderNotFound) {
			return utils.Success(c, fiber.StatusOK, "notification ignored: order not found", nil)
		}
		var serviceError *orderServiceError
		if errors.As(err, &serviceError) {
			return utils.Error(c, serviceError.Status, serviceError.Message)
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to process notification")
	}

	updatedOrder, err := findOrderByCode(h.db, processedOrderCode)
	if err == nil {
		switch payload.TransactionStatus {
		case "settlement", "capture":
			broadcastPaymentConfirmed(updatedOrder)
		case "pending":
			broadcastPaymentWaitingConfirmation(updatedOrder)
		case "deny", "expire", "cancel":
			broadcastOrderStatus("order_cancelled", updatedOrder)
		}
	}

	return utils.Success(c, fiber.StatusOK, "notification processed", nil)
}

func (h *MidtransPaymentHandler) SyncStatus(c *fiber.Ctx) error {
	orderCode := strings.TrimSpace(c.Params("order_code"))
	if orderCode == "" {
		return utils.Error(c, fiber.StatusBadRequest, "order_code is required")
	}

	var order models.Order
	if err := h.db.Where("order_code = ?", orderCode).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "order not found")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get order")
	}

	var payment models.Payment
	if err := h.db.Where("order_id = ?", order.ID).First(&payment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "payment not found")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get payment")
	}
	if payment.MidtransOrderID == nil || strings.TrimSpace(*payment.MidtransOrderID) == "" {
		return utils.Error(c, fiber.StatusBadRequest, "midtrans_order_id is required")
	}

	statusResponse, err := h.midtrans.GetTransactionStatus(*payment.MidtransOrderID)
	if err != nil {
		return utils.Error(c, fiber.StatusBadGateway, "failed to get midtrans status")
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, order.ID).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ?", order.ID).First(&payment).Error; err != nil {
			return err
		}
		return applyMidtransTransactionStatus(
			tx,
			&payment,
			&order,
			statusResponse.TransactionStatus,
			statusResponse.PaymentType,
			statusResponse.TransactionID,
			statusResponse.FraudStatus,
		)
	})
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to sync midtrans status")
	}

	updatedOrder, err := findOrderByCode(h.db, orderCode)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get updated order")
	}
	var updatedPayment models.Payment
	if err := h.db.Where("order_id = ?", updatedOrder.ID).First(&updatedPayment).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get updated payment")
	}

	switch statusResponse.TransactionStatus {
	case "settlement", "capture":
		broadcastPaymentConfirmed(updatedOrder)
	case "pending":
		broadcastPaymentWaitingConfirmation(updatedOrder)
	case "deny", "expire", "cancel":
		broadcastOrderStatus("order_cancelled", updatedOrder)
	}

	return utils.Success(c, fiber.StatusOK, "midtrans status synced", fiber.Map{
		"order":   updatedOrder,
		"payment": updatedPayment,
	})
}

func applyMidtransTransactionStatus(tx *gorm.DB, payment *models.Payment, order *models.Order, transactionStatus, paymentType, transactionID, fraudStatus string) error {
	updates := map[string]any{
		"payment_type":   nullableString(paymentType),
		"transaction_id": nullableString(transactionID),
		"fraud_status":   nullableString(fraudStatus),
	}

	switch transactionStatus {
	case "settlement", "capture":
		now := time.Now()
		updates["status"] = models.PaymentPaid
		updates["paid_at"] = &now
		if err := tx.Model(payment).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Model(order).Update("status", models.OrderSentToKitchen).Error
	case "pending":
		updates["status"] = models.PaymentWaitingConfirmation
		if err := tx.Model(payment).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Model(order).Update("status", models.OrderPendingPayment).Error
	case "deny", "expire", "cancel":
		updates["status"] = models.PaymentRejected
		if err := tx.Model(payment).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Model(order).Update("status", models.OrderCancelled).Error; err != nil {
			return err
		}
		return releaseOrderTable(tx, *order)
	default:
		return tx.Model(payment).Updates(updates).Error
	}
}

func findMidtransPaymentAndOrder(tx *gorm.DB, orderID string) (models.Payment, models.Order, error) {
	var payment models.Payment
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("midtrans_order_id = ?", orderID).
		First(&payment).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.Payment{}, models.Order{}, err
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		var order models.Order
		if err := tx.Where("order_code = ?", orderID).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return models.Payment{}, models.Order{}, errMidtransOrderNotFound
			}
			return models.Payment{}, models.Order{}, err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_id = ?", order.ID).
			First(&payment).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return models.Payment{}, models.Order{}, errMidtransOrderNotFound
			}
			return models.Payment{}, models.Order{}, err
		}
	}

	var order models.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&order, payment.OrderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Payment{}, models.Order{}, errMidtransOrderNotFound
		}
		return models.Payment{}, models.Order{}, err
	}

	return payment, order, nil
}

func (h *MidtransPaymentHandler) Status(c *fiber.Ctx) error {
	orderCode := strings.TrimSpace(c.Params("order_code"))
	if orderCode == "" {
		return utils.Error(c, fiber.StatusBadRequest, "order_code is required")
	}

	order, err := findOrderByCode(h.db, orderCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "order not found")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get payment status")
	}

	return utils.Success(c, fiber.StatusOK, "payment status retrieved", order)
}

func findOrderByCode(db *gorm.DB, orderCode string) (models.Order, error) {
	var order models.Order
	err := preloadOrder(db).Where("order_code = ?", orderCode).First(&order).Error
	return order, err
}

func nullableString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
