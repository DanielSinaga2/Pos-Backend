package handlers

import (
	"errors"
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
	if err := h.db.Preload("Items.Menu").Preload("Items.Options").Preload("Payment").First(&order, orderID).Error; err != nil {
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
	if !models.IsOnlinePaymentMethod(order.Payment.PaymentMethod) {
		return utils.Error(c, fiber.StatusBadRequest, "payment_method must be qris")
	}
	if order.Payment.SnapToken != nil && *order.Payment.SnapToken != "" {
		return utils.Success(c, fiber.StatusOK, "snap token already exists", fiber.Map{
			"order_id":          order.ID,
			"order_code":        order.OrderCode,
			"snap_token":        *order.Payment.SnapToken,
			"redirect_url":      stringValue(order.Payment.SnapRedirectURL),
			"snap_redirect_url": stringValue(order.Payment.SnapRedirectURL),
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
		"payment_type":      midtransPaymentType(order.Payment.PaymentMethod),
	}).Error
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to save snap token")
	}

	return utils.Success(c, fiber.StatusOK, "snap token created", fiber.Map{
		"order_id":          order.ID,
		"order_code":        order.OrderCode,
		"snap_token":        snapResponse.Token,
		"redirect_url":      snapResponse.RedirectURL,
		"snap_redirect_url": snapResponse.RedirectURL,
	})
}

func (h *MidtransPaymentHandler) Notification(c *fiber.Ctx) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("midtrans notification recovered from panic: %v", recovered)
			err = utils.Success(c, fiber.StatusOK, "notification received", nil)
		}
	}()

	var payload services.NotificationPayload
	if err := c.BodyParser(&payload); err != nil {
		log.Printf("midtrans notification invalid payload: %v", err)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"success": false,
			"message": "invalid notification payload",
		})
	}
	if strings.TrimSpace(payload.OrderID) == "" {
		log.Print("midtrans notification ignored: order_id is required")
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"success": false,
			"message": "order_id is required",
		})
	}
	if strings.HasPrefix(payload.OrderID, "payment_notif_test_") {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"success": true,
			"message": "midtrans test notification received",
		})
	}

	var processedOrderCode string
	err = h.db.Transaction(func(tx *gorm.DB) error {
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
			log.Printf("order not found: %s", payload.OrderID)
			return utils.Success(c, fiber.StatusOK, "order not found", nil)
		}
		var serviceError *orderServiceError
		if errors.As(err, &serviceError) {
			log.Printf("midtrans notification ignored for order_id %s: %s", payload.OrderID, serviceError.Message)
			return utils.Success(c, fiber.StatusOK, serviceError.Message, nil)
		}
		log.Printf("midtrans notification failed for order_id %s: %v", payload.OrderID, err)
		return utils.Success(c, fiber.StatusOK, "notification received", nil)
	}

	updatedOrder, err := findOrderByCode(h.db, processedOrderCode)
	if err == nil {
		broadcastMidtransPaymentUpdate(updatedOrder)
	}

	return utils.Success(c, fiber.StatusOK, "notification processed", nil)
}

func (h *MidtransPaymentHandler) SyncStatus(c *fiber.Ctx) error {
	orderCode := strings.TrimSpace(c.Params("order_code"))
	if orderCode == "" {
		return utils.Error(c, fiber.StatusBadRequest, "order_code is required")
	}

	updatedOrder, err := syncMidtransPaymentByOrderCode(h.db, h.midtrans, orderCode)
	if err != nil {
		return orderError(c, err, "failed to sync midtrans status")
	}
	broadcastMidtransPaymentUpdate(updatedOrder)

	return utils.Success(c, fiber.StatusOK, "midtrans status synced", fiber.Map{
		"order":   updatedOrder,
		"payment": updatedOrder.Payment,
	})
}

func applyMidtransTransactionStatus(tx *gorm.DB, payment *models.Payment, order *models.Order, transactionStatus, paymentType, transactionID, fraudStatus string) error {
	updates := map[string]any{
		"payment_type":   nullableString(paymentType),
		"transaction_id": nullableString(transactionID),
		"fraud_status":   nullableString(fraudStatus),
	}

	if payment.Status == models.PaymentPaid {
		if err := tx.Model(payment).Updates(updates).Error; err != nil {
			return err
		}
		if order.Status == models.OrderPendingPayment || order.Status == models.OrderPaid {
			if err := tx.Model(order).Update("status", models.OrderSentToKitchen).Error; err != nil {
				return err
			}
			order.Status = models.OrderSentToKitchen
		}
		return nil
	}

	switch transactionStatus {
	case "settlement", "capture":
		return markPaymentPaidAndSendOrderToKitchen(tx, payment, order, updates)
	case "pending":
		updates["status"] = models.PaymentPending
		if err := tx.Model(payment).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Model(order).Update("status", models.OrderPendingPayment).Error; err != nil {
			return err
		}
		payment.Status = models.PaymentPending
		order.Status = models.OrderPendingPayment
		return nil
	case "expire", "deny", "failure":
		updates["status"] = models.PaymentFailed
		if err := tx.Model(payment).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Model(order).Update("status", models.OrderCancelled).Error; err != nil {
			return err
		}
		payment.Status = models.PaymentFailed
		order.Status = models.OrderCancelled
		return nil
	case "cancel":
		updates["status"] = models.PaymentCancelled
		if err := tx.Model(payment).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Model(order).Update("status", models.OrderCancelled).Error; err != nil {
			return err
		}
		payment.Status = models.PaymentCancelled
		order.Status = models.OrderCancelled
		return nil
	default:
		return tx.Model(payment).Updates(updates).Error
	}
}

func markPaymentPaidAndSendOrderToKitchen(tx *gorm.DB, payment *models.Payment, order *models.Order, paymentUpdates map[string]any) error {
	now := time.Now()
	paymentUpdates["status"] = models.PaymentPaid
	paymentUpdates["paid_at"] = &now

	if err := tx.Model(payment).Updates(paymentUpdates).Error; err != nil {
		return err
	}
	if order.Status == models.OrderPendingPayment || order.Status == models.OrderPaid {
		if err := tx.Model(order).Update("status", models.OrderSentToKitchen).Error; err != nil {
			return err
		}
		order.Status = models.OrderSentToKitchen
	}
	payment.Status = models.PaymentPaid
	payment.PaidAt = &now
	return nil
}

func syncMidtransPaymentByOrderCode(db *gorm.DB, midtrans *services.MidtransService, orderCode string) (models.Order, error) {
	var order models.Order
	if err := db.Preload("Payment").Where("order_code = ?", orderCode).First(&order).Error; err != nil {
		return models.Order{}, err
	}
	if order.Payment == nil {
		return models.Order{}, &orderServiceError{Status: fiber.StatusNotFound, Message: "payment not found"}
	}
	if order.Payment.PaymentMethod == models.PaymentCash {
		return models.Order{}, &orderServiceError{Status: fiber.StatusBadRequest, Message: "cash payment does not need Midtrans"}
	}
	if !models.IsOnlinePaymentMethod(order.Payment.PaymentMethod) {
		return models.Order{}, &orderServiceError{Status: fiber.StatusBadRequest, Message: "payment_method must be qris"}
	}
	if order.Payment.Status == models.PaymentPaid {
		return findOrder(db, order.ID)
	}

	midtransOrderID := order.OrderCode
	if order.Payment.MidtransOrderID != nil && strings.TrimSpace(*order.Payment.MidtransOrderID) != "" {
		midtransOrderID = strings.TrimSpace(*order.Payment.MidtransOrderID)
	}

	statusResponse, err := midtrans.GetTransactionStatus(midtransOrderID)
	if err != nil {
		return models.Order{}, &orderServiceError{Status: fiber.StatusBadGateway, Message: "failed to get midtrans status"}
	}
	log.Printf(
		"midtrans sync status order_code=%s midtrans_order_id=%s transaction_status=%s payment_type=%s transaction_id=%s fraud_status=%s",
		order.OrderCode,
		midtransOrderID,
		statusResponse.TransactionStatus,
		statusResponse.PaymentType,
		statusResponse.TransactionID,
		statusResponse.FraudStatus,
	)

	var latestOrder models.Order
	if err := db.Preload("Payment").Where("order_code = ?", orderCode).First(&latestOrder).Error; err != nil {
		return models.Order{}, err
	}
	if latestOrder.Payment == nil {
		return models.Order{}, &orderServiceError{Status: fiber.StatusNotFound, Message: "payment not found"}
	}
	if latestOrder.Payment.Status == models.PaymentPaid {
		return findOrder(db, latestOrder.ID)
	}

	if !midtransStatusUpdatesPaymentStatus(statusResponse.TransactionStatus) ||
		!midtransStatusChangeNeeded(latestOrder, statusResponse.TransactionStatus) {
		if err := updateMidtransPaymentMetadata(db, latestOrder.Payment, statusResponse); err != nil {
			return models.Order{}, err
		}
		return findOrder(db, latestOrder.ID)
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		var lockedOrder models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Payment").
			Where("order_code = ?", orderCode).
			First(&lockedOrder).Error; err != nil {
			return err
		}
		if lockedOrder.Payment == nil {
			return &orderServiceError{Status: fiber.StatusNotFound, Message: "payment not found"}
		}
		if lockedOrder.Payment.Status == models.PaymentPaid {
			return nil
		}
		if !midtransStatusChangeNeeded(lockedOrder, statusResponse.TransactionStatus) {
			return updateMidtransPaymentMetadata(tx, lockedOrder.Payment, statusResponse)
		}
		return applyMidtransTransactionStatus(
			tx,
			lockedOrder.Payment,
			&lockedOrder,
			statusResponse.TransactionStatus,
			statusResponse.PaymentType,
			statusResponse.TransactionID,
			statusResponse.FraudStatus,
		)
	})
	if err != nil {
		return models.Order{}, err
	}

	return findOrder(db, order.ID)
}

func midtransStatusUpdatesPaymentStatus(transactionStatus string) bool {
	switch transactionStatus {
	case "settlement", "capture", "pending", "expire", "deny", "failure", "cancel":
		return true
	default:
		return false
	}
}

func midtransStatusChangeNeeded(order models.Order, transactionStatus string) bool {
	if order.Payment == nil {
		return false
	}

	switch transactionStatus {
	case "settlement", "capture":
		return order.Payment.Status != models.PaymentPaid
	case "pending":
		return order.Payment.Status != models.PaymentPending || order.Status != models.OrderPendingPayment
	case "expire", "deny", "failure":
		return order.Payment.Status != models.PaymentFailed || order.Status != models.OrderCancelled
	case "cancel":
		return order.Payment.Status != models.PaymentCancelled || order.Status != models.OrderCancelled
	default:
		return false
	}
}

func updateMidtransPaymentMetadata(db *gorm.DB, payment *models.Payment, statusResponse services.TransactionStatusResponse) error {
	return db.Model(payment).Updates(map[string]any{
		"payment_type":   nullableString(statusResponse.PaymentType),
		"transaction_id": nullableString(statusResponse.TransactionID),
		"fraud_status":   nullableString(statusResponse.FraudStatus),
	}).Error
}

func broadcastMidtransPaymentUpdate(order models.Order) {
	if order.Payment == nil {
		broadcastOrderStatus("order_updated", order)
		return
	}

	switch order.Payment.Status {
	case models.PaymentPaid:
		broadcastPaymentConfirmed(order)
	case models.PaymentPending:
		broadcastPaymentWaitingConfirmation(order)
	case models.PaymentFailed, models.PaymentCancelled:
		broadcastOrderStatus("order_cancelled", order)
	default:
		broadcastOrderStatus("order_updated", order)
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

	order, err := syncMidtransPaymentByOrderCode(h.db, h.midtrans, orderCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "order not found")
		}
		return orderError(c, err, "failed to get payment status")
	}
	broadcastMidtransPaymentUpdate(order)

	return utils.Success(c, fiber.StatusOK, "payment status retrieved", fiber.Map{
		"order":   order,
		"payment": order.Payment,
	})
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

func midtransPaymentType(method models.PaymentMethod) string {
	return "qris"
}
