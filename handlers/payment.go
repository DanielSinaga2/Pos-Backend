package handlers

import (
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"pos-backend/config"
	"pos-backend/dto"
	"pos-backend/models"
	"pos-backend/services"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// PaymentHandler handles Snap-based payments (legacy flow)
type PaymentHandler struct {
	midtrans *services.MidtransService
}

// CorePaymentHandler handles Core API QRIS payments
type CorePaymentHandler struct {
	service *services.PaymentService
	db      *gorm.DB
}

// NewPaymentHandler creates a new PaymentHandler
func NewPaymentHandler(midtrans *services.MidtransService) *PaymentHandler {
	return &PaymentHandler{midtrans: midtrans}
}

// NewCorePaymentHandler creates a new CorePaymentHandler
func NewCorePaymentHandler(service *services.PaymentService, db *gorm.DB) *CorePaymentHandler {
	return &CorePaymentHandler{
		service: service,
		db:      db,
	}
}

// Create generates a Core API QRIS transaction
func (h *CorePaymentHandler) Create(c *fiber.Ctx) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in CorePaymentHandler.Create: %v", r)
			_ = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": fmt.Sprintf("Internal Server Error: %v", r),
			})
		}
	}()

	var request dto.CorePaymentCreateRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid JSON request")
	}
	request.OrderID = strings.TrimSpace(request.OrderID)

	if request.OrderID == "" {
		return utils.Error(c, fiber.StatusBadRequest, "order_id is required")
	}
	if request.Amount <= 0 {
		return utils.Error(c, fiber.StatusBadRequest, "amount must be greater than 0")
	}

	response, err := h.service.CreateQRIS(
		request.OrderID,
		request.Amount,
	)
	if err != nil {
		log.Printf("create Core API QRIS payment failed order_id=%s amount=%d error=%v", request.OrderID, request.Amount, err)
		if errors.Is(err, services.ErrMidtransNotConfig) {
			return utils.Error(c, fiber.StatusInternalServerError, "Midtrans is not configured")
		}
		return utils.Error(c, fiber.StatusBadGateway, "failed to create QRIS payment")
	}
	return c.Status(fiber.StatusCreated).JSON(response)
}

// Status handles GET /api/payment/status/:orderId
func (h *CorePaymentHandler) Status(c *fiber.Ctx) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in CorePaymentHandler.Status: %v", r)
			err = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": fmt.Sprintf("Internal Server Error: %v", r),
			})
		}
	}()

	orderID := strings.TrimSpace(c.Params("orderId"))
	if orderID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "orderId parameter is required",
		})
	}

	status, err := h.service.GetStatus(orderID)
	if err != nil {
		log.Printf("GetStatus error order_id=%s: %v", orderID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "failed to get payment status: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success":            true,
		"transaction_status": status.TransactionStatus,
		"order_id":           orderID,
	})
}

// Notification handles POST /api/payment/notification
func (h *CorePaymentHandler) Notification(c *fiber.Ctx) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in CorePaymentHandler.Notification: %v", r)
			err = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": fmt.Sprintf("Internal Server Error: %v", r),
			})
		}
	}()

	var notification struct {
		OrderID           string `json:"order_id"`
		StatusCode        string `json:"status_code"`
		GrossAmount       string `json:"gross_amount"`
		TransactionStatus string `json:"transaction_status"`
		PaymentType       string `json:"payment_type"`
		SignatureKey      string `json:"signature_key"`
	}

	if err := c.BodyParser(&notification); err != nil {
		log.Printf("Error parsing notification body: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "invalid request body: " + err.Error(),
		})
	}

	cfg := config.LoadMidtransConfig()
	if cfg.ServerKey == "" {
		log.Println("Midtrans config error: MIDTRANS_SERVER_KEY is required")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Midtrans configuration error: MIDTRANS_SERVER_KEY is required",
		})
	}

	// Verify signature
	signatureSource := notification.OrderID + notification.StatusCode + notification.GrossAmount + cfg.ServerKey
	hasher := sha512.New()
	hasher.Write([]byte(signatureSource))
	calculatedSignature := hex.EncodeToString(hasher.Sum(nil))

	if !strings.EqualFold(calculatedSignature, notification.SignatureKey) {
		log.Printf("Invalid Midtrans signature: calculated=%s, received=%s", calculatedSignature, notification.SignatureKey)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "invalid signature key",
		})
	}

	// Find the order by order_code (which maps to order_id in Midtrans)
	var order models.Order
	err = h.db.Preload("Payment").Where("order_code = ?", notification.OrderID).First(&order).Error
	if err != nil {
		// Fallback: search by MidtransOrderID if not found directly
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var payment models.Payment
			err = h.db.Where("midtrans_order_id = ?", notification.OrderID).First(&payment).Error
			if err == nil {
				err = h.db.Preload("Payment").First(&order, payment.OrderID).Error
			}
		}

		if err != nil {
			log.Printf("Order not found in database: %s, error: %v", notification.OrderID, err)
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"message": "order not found: " + err.Error(),
			})
		}
	}

	// Map statuses
	var newOrderStatus models.OrderStatus
	var newPaymentStatus models.PaymentStatus

	switch notification.TransactionStatus {
	case "settlement", "capture":
		newOrderStatus = models.OrderPaid
		newPaymentStatus = models.PaymentPaid
	case "pending":
		newOrderStatus = models.OrderPendingPayment
		newPaymentStatus = models.PaymentPending
	case "deny", "cancel", "expire":
		newOrderStatus = models.OrderCancelled
		newPaymentStatus = models.PaymentFailed
	default:
		// Keep unchanged
		newOrderStatus = order.Status
		if order.Payment != nil {
			newPaymentStatus = order.Payment.Status
		}
	}

	// Update order and payment status in database
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&order).Update("status", newOrderStatus).Error; err != nil {
			return err
		}

		if order.Payment != nil {
			updates := map[string]interface{}{
				"status":       newPaymentStatus,
				"payment_type": notification.PaymentType,
			}
			if newPaymentStatus == models.PaymentPaid {
				now := time.Now()
				updates["paid_at"] = &now
			}
			if err := tx.Model(order.Payment).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		log.Printf("Failed to update order status in DB: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "failed to update database: " + err.Error(),
		})
	}

	// Fetch updated order to broadcast realtime websocket updates
	var updatedOrder models.Order
	if err := h.db.Preload("Payment").First(&updatedOrder, order.ID).Error; err == nil {
		broadcastMidtransPaymentUpdate(updatedOrder)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": "notification processed successfully",
	})
}

// Create (legacy snap flow)
func (h *PaymentHandler) Create(c *fiber.Ctx) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in PaymentHandler.Create: %v", r)
			_ = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": fmt.Sprintf("Internal Server Error: %v", r),
			})
		}
	}()

	var request dto.CreatePaymentRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid JSON request")
	}

	request.OrderID = strings.TrimSpace(request.OrderID)
	request.PaymentMethod = strings.ToLower(strings.TrimSpace(request.PaymentMethod))
	request.Customer.FirstName = strings.TrimSpace(request.Customer.FirstName)
	request.Customer.Email = strings.TrimSpace(request.Customer.Email)
	request.Customer.Phone = strings.TrimSpace(request.Customer.Phone)

	if request.OrderID == "" {
		return utils.Error(c, fiber.StatusBadRequest, "order_id is required")
	}
	if request.GrossAmount <= 0 {
		return utils.Error(c, fiber.StatusBadRequest, "gross_amount must be greater than 0")
	}
	enabledPayments, err := services.EnabledPaymentsForMethod(request.PaymentMethod)
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "payment_method must be qris")
	}

	log.Printf(
		"payment create request order_id=%s payment_method=%s gross_amount=%d enabled_payments=%v",
		request.OrderID,
		request.PaymentMethod,
		request.GrossAmount,
		enabledPayments,
	)

	result, err := h.midtrans.CreatePaymentTransaction(services.CreatePaymentTransactionInput{
		OrderID:     request.OrderID,
		GrossAmount: request.GrossAmount,
		Customer: services.PaymentCustomer{
			FirstName: request.Customer.FirstName,
			Email:     request.Customer.Email,
			Phone:     request.Customer.Phone,
		},
		PaymentMethod: request.PaymentMethod,
	})
	if err != nil {
		if strings.Contains(err.Error(), "MIDTRANS_SERVER_KEY") {
			return utils.Error(c, fiber.StatusInternalServerError, "MIDTRANS_SERVER_KEY is required")
		}
		log.Printf("payment create failed order_id=%s payment_method=%s error=%v", request.OrderID, request.PaymentMethod, err)
		return utils.Error(c, fiber.StatusBadGateway, "failed to create payment transaction")
	}

	return utils.Success(c, fiber.StatusOK, "QRIS payment transaction created", dto.CreatePaymentResponse{
		OrderID:       result.OrderID,
		PaymentMethod: result.PaymentMethod,
		SnapToken:     result.SnapToken,
		RedirectURL:   result.RedirectURL,
		MidtransPayloadSummary: dto.MidtransPayloadSummary{
			EnabledPayments: result.PayloadSummary.EnabledPayments,
			GrossAmount:     result.PayloadSummary.GrossAmount,
		},
	})
}

// Webhook (legacy webhook handler)
func (h *PaymentHandler) Webhook(c *fiber.Ctx) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in PaymentHandler.Webhook: %v", r)
			_ = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": fmt.Sprintf("Internal Server Error: %v", r),
			})
		}
	}()

	var payload services.NotificationPayload
	if err := c.BodyParser(&payload); err != nil {
		log.Printf("payment webhook invalid payload: %v", err)
		return utils.Error(c, fiber.StatusBadRequest, "invalid notification payload")
	}

	log.Printf(
		"midtrans webhook order_id=%s transaction_status=%s fraud_status=%s payment_type=%s transaction_id=%s",
		payload.OrderID,
		payload.TransactionStatus,
		payload.FraudStatus,
		payload.PaymentType,
		payload.TransactionID,
	)

	nextStatus := "unchanged"
	switch payload.TransactionStatus {
	case "settlement", "capture":
		nextStatus = "paid"
	case "pending":
		nextStatus = "pending"
	case "expire", "deny", "failure":
		nextStatus = "failed"
	case "cancel":
		nextStatus = "cancelled"
	}

	log.Printf("midtrans webhook mapped order_id=%s next_payment_status=%s", payload.OrderID, nextStatus)

	return utils.Success(c, fiber.StatusOK, "notification received", fiber.Map{
		"order_id":            payload.OrderID,
		"transaction_status":  payload.TransactionStatus,
		"fraud_status":        payload.FraudStatus,
		"payment_type":        payload.PaymentType,
		"next_payment_status": nextStatus,
	})
}

