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

type PublicOrderHandler struct {
	db       *gorm.DB
	midtrans *services.MidtransService
}

type uploadPaymentProofRequest struct {
	ImageURL string `json:"image_url"`
}

type publicCustomerResponse struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email,omitempty"`
}

type customerHistoryItemResponse struct {
	MenuName  string                          `json:"menu_name"`
	Quantity  int                             `json:"quantity"`
	Price     int64                           `json:"price"`
	UnitPrice int64                           `json:"unit_price"`
	Subtotal  int64                           `json:"subtotal"`
	Notes     string                          `json:"notes"`
	Options   []customerHistoryOptionResponse `json:"options"`
}

type customerHistoryOptionResponse struct {
	GroupName       string `json:"group_name"`
	OptionName      string `json:"option_name"`
	AdditionalPrice int64  `json:"additional_price"`
}

type customerHistoryOrderResponse struct {
	ID            uint                          `json:"id"`
	OrderCode     string                        `json:"order_code"`
	OrderType     models.OrderType              `json:"order_type"`
	TableName     string                        `json:"table_name"`
	TableNumber   string                        `json:"table_number"`
	TotalAmount   int64                         `json:"total_amount"`
	PaymentMethod models.PaymentMethod          `json:"payment_method"`
	PaymentStatus models.PaymentStatus          `json:"payment_status"`
	OrderStatus   models.OrderStatus            `json:"order_status"`
	CreatedAt     time.Time                     `json:"created_at"`
	Items         []customerHistoryItemResponse `json:"items"`
}

type publicOrderPaymentSyncResponse struct {
	models.Order
	PaymentStatus models.PaymentStatus `json:"payment_status"`
	OrderStatus   models.OrderStatus   `json:"order_status"`
}

func NewPublicOrderHandler(db *gorm.DB, midtrans *services.MidtransService) *PublicOrderHandler {
	return &PublicOrderHandler{db: db, midtrans: midtrans}
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

func (h *PublicOrderHandler) CustomerProfile(c *fiber.Ctx) error {
	phone := normalizeCustomerPhone(c.Query("phone"))
	if phone == "" {
		return utils.Error(c, fiber.StatusBadRequest, "phone is required")
	}

	customer, err := h.findCustomerByPhone(phone)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Success(c, fiber.StatusOK, "customer profile retrieved successfully", fiber.Map{
				"customer": nil,
			})
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get customer profile")
	}

	return utils.Success(c, fiber.StatusOK, "customer profile retrieved successfully", fiber.Map{
		"customer": publicCustomerResponseFromModel(customer),
	})
}

func (h *PublicOrderHandler) CustomerOrders(c *fiber.Ctx) error {
	phone := normalizeCustomerPhone(c.Query("phone"))
	if phone == "" {
		return utils.Error(c, fiber.StatusBadRequest, "phone is required")
	}

	customer, err := h.findCustomerByPhone(phone)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Success(c, fiber.StatusOK, "customer orders retrieved successfully", fiber.Map{
				"customer": nil,
				"orders":   []customerHistoryOrderResponse{},
			})
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get customer orders")
	}

	var orders []models.Order
	if err := h.db.
		Preload("Table").
		Preload("Payment").
		Preload("Items.Menu").
		Preload("Items.Options").
		Where("customer_id = ?", customer.ID).
		Order("created_at DESC").
		Find(&orders).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get customer orders")
	}

	return utils.Success(c, fiber.StatusOK, "customer orders retrieved successfully", fiber.Map{
		"customer": publicCustomerResponseFromModel(customer),
		"orders":   customerHistoryOrderResponses(orders),
	})
}

func (h *PublicOrderHandler) CreateSnap(c *fiber.Ctx) error {
	orderCode := strings.TrimSpace(c.Params("order_code"))
	if orderCode == "" {
		return utils.Error(c, fiber.StatusBadRequest, "order_code is required")
	}

	var order models.Order
	if err := h.db.Preload("Items.Menu").Preload("Items.Options").Preload("Payment").Where("order_code = ?", orderCode).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "order not found")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get order")
	}
	if order.Payment == nil {
		return utils.Error(c, fiber.StatusNotFound, "payment not found")
	}
	if order.Payment.PaymentMethod == models.PaymentCash {
		return utils.Error(c, fiber.StatusBadRequest, "cash payment does not need Midtrans")
	}
	if !models.IsOnlinePaymentMethod(order.Payment.PaymentMethod) {
		return utils.Error(c, fiber.StatusBadRequest, "payment_method must be qris")
	}
	if order.Payment.Status == models.PaymentPaid {
		return utils.Error(c, fiber.StatusConflict, "payment already paid")
	}
	if order.Payment.SnapToken != nil && *order.Payment.SnapToken != "" {
		if order.Payment.PaymentType == nil || strings.TrimSpace(*order.Payment.PaymentType) == "" {
			if err := h.db.Model(order.Payment).Update("payment_type", midtransPaymentType(order.Payment.PaymentMethod)).Error; err != nil {
				return utils.Error(c, fiber.StatusInternalServerError, "failed to save snap token")
			}
		}
		return utils.Success(c, fiber.StatusOK, "snap created successfully", fiber.Map{
			"order_code":        order.OrderCode,
			"snap_token":        *order.Payment.SnapToken,
			"snap_redirect_url": stringValue(order.Payment.SnapRedirectURL),
		})
	}

	snapResponse, err := h.midtrans.CreateSnapTransaction(order, *order.Payment)
	if err != nil {
		return utils.Error(c, fiber.StatusBadGateway, "failed to create snap token")
	}

	if err := h.db.Model(order.Payment).Updates(map[string]any{
		"midtrans_order_id": order.OrderCode,
		"snap_token":        snapResponse.Token,
		"snap_redirect_url": snapResponse.RedirectURL,
		"payment_type":      midtransPaymentType(order.Payment.PaymentMethod),
	}).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to save snap token")
	}

	return utils.Success(c, fiber.StatusOK, "snap created successfully", fiber.Map{
		"order_code":        order.OrderCode,
		"snap_token":        snapResponse.Token,
		"snap_redirect_url": snapResponse.RedirectURL,
	})
}

func (h *PublicOrderHandler) SyncPayment(c *fiber.Ctx) error {
	orderCode := strings.TrimSpace(c.Params("order_code"))
	if orderCode == "" {
		return utils.Error(c, fiber.StatusBadRequest, "order_code is required")
	}

	order, err := syncMidtransPaymentByOrderCode(h.db, h.midtrans, orderCode)
	if err != nil {
		return orderError(c, err, "failed to sync payment")
	}
	broadcastMidtransPaymentUpdate(order)
	return utils.Success(c, fiber.StatusOK, "payment synced successfully", publicOrderPaymentSyncResponseFromOrder(order))
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
		if !models.IsOnlinePaymentMethod(order.Payment.PaymentMethod) {
			return &orderServiceError{Status: 400, Message: "payment proof is only accepted for qris payment"}
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

func (h *PublicOrderHandler) findCustomerByPhone(phone string) (models.Customer, error) {
	var customer models.Customer
	err := h.db.Where("phone = ?", phone).First(&customer).Error
	return customer, err
}

func publicCustomerResponseFromModel(customer models.Customer) publicCustomerResponse {
	return publicCustomerResponse{
		ID:    customer.ID,
		Name:  customer.Name,
		Phone: customer.Phone,
		Email: customer.Email,
	}
}

func publicOrderPaymentSyncResponseFromOrder(order models.Order) publicOrderPaymentSyncResponse {
	var paymentStatus models.PaymentStatus
	if order.Payment != nil {
		paymentStatus = order.Payment.Status
	}

	return publicOrderPaymentSyncResponse{
		Order:         order,
		PaymentStatus: paymentStatus,
		OrderStatus:   order.Status,
	}
}

func customerHistoryOrderResponses(orders []models.Order) []customerHistoryOrderResponse {
	response := make([]customerHistoryOrderResponse, 0, len(orders))
	for _, order := range orders {
		var tableName string
		if order.Table != nil {
			tableName = order.Table.TableNumber
		}

		var paymentMethod models.PaymentMethod
		var paymentStatus models.PaymentStatus
		if order.Payment != nil {
			paymentMethod = order.Payment.PaymentMethod
			paymentStatus = order.Payment.Status
		}

		items := make([]customerHistoryItemResponse, 0, len(order.Items))
		for _, item := range order.Items {
			options := make([]customerHistoryOptionResponse, 0, len(item.Options))
			for _, option := range item.Options {
				options = append(options, customerHistoryOptionResponse{
					GroupName:       option.GroupName,
					OptionName:      option.OptionName,
					AdditionalPrice: option.AdditionalPrice,
				})
			}
			unitPrice := item.UnitPrice
			if unitPrice == 0 {
				unitPrice = item.Price
			}
			items = append(items, customerHistoryItemResponse{
				MenuName:  item.Menu.Name,
				Quantity:  item.Quantity,
				Price:     item.Price,
				UnitPrice: unitPrice,
				Subtotal:  item.Subtotal,
				Notes:     item.Note,
				Options:   options,
			})
		}

		response = append(response, customerHistoryOrderResponse{
			ID:            order.ID,
			OrderCode:     order.OrderCode,
			OrderType:     order.OrderType,
			TableName:     tableName,
			TableNumber:   tableName,
			TotalAmount:   order.TotalAmount,
			PaymentMethod: paymentMethod,
			PaymentStatus: paymentStatus,
			OrderStatus:   order.Status,
			CreatedAt:     order.CreatedAt,
			Items:         items,
		})
	}
	return response
}
