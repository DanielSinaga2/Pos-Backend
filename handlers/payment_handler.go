package handlers

import (
	"errors"
	"log"
	"strings"

	"pos-backend/dto"
	"pos-backend/services"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
)

type PaymentHandler struct {
	midtrans *services.MidtransService
}

type CorePaymentHandler struct {
	service *services.PaymentService
}

func NewPaymentHandler(midtrans *services.MidtransService) *PaymentHandler {
	return &PaymentHandler{midtrans: midtrans}
}

func NewCorePaymentHandler(service *services.PaymentService) *CorePaymentHandler {
	return &CorePaymentHandler{service: service}
}

func (h *CorePaymentHandler) Create(c *fiber.Ctx) error {
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

func (h *CorePaymentHandler) Status(c *fiber.Ctx) error {
	orderID := strings.TrimSpace(c.Params("order_id"))
	if orderID == "" {
		return utils.Error(c, fiber.StatusBadRequest, "order_id is required")
	}

	response, err := h.service.GetStatus(orderID)
	if err != nil {
		log.Printf("get Core API QRIS status failed order_id=%s error=%v", orderID, err)
		if errors.Is(err, services.ErrMidtransNotConfig) {
			return utils.Error(c, fiber.StatusInternalServerError, "Midtrans is not configured")
		}
		return utils.Error(c, fiber.StatusBadGateway, "failed to get payment status")
	}
	return c.JSON(response)
}

func (h *PaymentHandler) Create(c *fiber.Ctx) error {
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

func (h *PaymentHandler) Webhook(c *fiber.Ctx) error {
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

	// Hook update status order bisa ditempatkan di sini jika endpoint ini dipakai
	// tanpa database flow existing. Endpoint /api/payments/midtrans/notification
	// tetap tersedia untuk update Payment/Order yang sudah tersimpan di DB.
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
