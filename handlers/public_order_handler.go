package handlers

import (
	"errors"
	"strings"

	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PublicOrderHandler struct {
	db *gorm.DB
}

type uploadPaymentProofRequest struct {
	ImageURL string `json:"image_url"`
}

func NewPublicOrderHandler(db *gorm.DB) *PublicOrderHandler {
	return &PublicOrderHandler{db: db}
}

func (h *PublicOrderHandler) Create(c *fiber.Ctx) error {
	var request createOrderRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	order, err := createOrder(h.db, request, nil, true)
	if err != nil {
		return orderError(c, err, "failed to create order")
	}
	broadcastOrderCreated(order)
	return utils.Success(c, fiber.StatusCreated, "order created successfully", order)
}

func (h *PublicOrderHandler) Get(c *fiber.Ctx) error {
	orderCode := strings.TrimSpace(c.Params("order_code"))
	if orderCode == "" {
		return utils.Error(c, fiber.StatusBadRequest, "order_code is required")
	}
	var order models.Order
	if err := preloadOrder(h.db).Where("order_code = ?", orderCode).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "order not found")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get order")
	}
	return utils.Success(c, fiber.StatusOK, "order retrieved successfully", order)
}

func (h *PublicOrderHandler) UploadPaymentProof(c *fiber.Ctx) error {
	orderCode := strings.TrimSpace(c.Params("order_code"))
	if orderCode == "" {
		return utils.Error(c, fiber.StatusBadRequest, "order_code is required")
	}
	var request uploadPaymentProofRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	request.ImageURL = strings.TrimSpace(request.ImageURL)
	if request.ImageURL == "" || len(request.ImageURL) > 500 || !isValidHTTPURL(request.ImageURL) {
		return utils.Error(c, fiber.StatusBadRequest, "image_url must be a valid HTTP or HTTPS URL")
	}

	var order models.Order
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Payment").
			Where("order_code = ?", orderCode).
			First(&order).Error; err != nil {
			return err
		}
		if order.Status == models.OrderCancelled || order.Status == models.OrderCompleted {
			return &orderServiceError{Status: 409, Message: "payment proof cannot be uploaded for this order"}
		}
		if order.Payment == nil {
			return errors.New("payment record not found")
		}
		if order.Payment.PaymentMethod == models.PaymentCash {
			return &orderServiceError{Status: 400, Message: "payment proof is not accepted for cash payment"}
		}
		if order.Payment.Status == models.PaymentPaid {
			return &orderServiceError{Status: 409, Message: "payment has already been confirmed"}
		}
		return tx.Model(order.Payment).Updates(map[string]any{
			"proof_image_url": request.ImageURL,
			"status":          models.PaymentWaitingConfirmation,
		}).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "order not found")
		}
		return orderError(c, err, "failed to upload payment proof")
	}
	updatedOrder, err := findOrder(h.db, order.ID)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get updated order")
	}
	broadcastPaymentWaitingConfirmation(updatedOrder)
	return utils.Success(c, fiber.StatusOK, "payment proof uploaded successfully", updatedOrder)
}
