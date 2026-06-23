package handlers

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"pos-backend/models"

	"gorm.io/gorm"
)

var jakartaLocation = time.FixedZone("Asia/Jakarta", 7*60*60)

type orderItemRequest struct {
	MenuID   uint                     `json:"menu_id"`
	Quantity int                      `json:"quantity"`
	Note     string                   `json:"note"`
	Notes    string                   `json:"notes"`
	Options  []orderItemOptionRequest `json:"options"`
}

type orderItemOptionRequest struct {
	GroupID  uint `json:"group_id"`
	OptionID uint `json:"option_id"`
}

type createOrderRequest struct {
	OrderType     models.OrderType     `json:"order_type"`
	TableID       *uint                `json:"table_id"`
	CustomerName  string               `json:"customer_name"`
	CustomerPhone string               `json:"customer_phone"`
	CustomerEmail string               `json:"customer_email"`
	Items         []orderItemRequest   `json:"items"`
	PaymentMethod models.PaymentMethod `json:"payment_method"`
}

type orderServiceError struct {
	Status  int
	Message string
}

func (e *orderServiceError) Error() string {
	return e.Message
}

func createOrder(db *gorm.DB, request createOrderRequest, createdBy *uint, public bool) (models.Order, error) {
	request.CustomerName = strings.TrimSpace(request.CustomerName)
	request.CustomerPhone = normalizeCustomerPhone(request.CustomerPhone)
	request.CustomerEmail = strings.TrimSpace(request.CustomerEmail)

	if message := validateCreateOrderRequest(request, public); message != "" {
		return models.Order{}, &orderServiceError{Status: 400, Message: message}
	}

	var order models.Order
	err := db.Transaction(func(tx *gorm.DB) error {
		items, itemOptions, totalAmount, err := buildOrderItems(tx, request.Items)
		if err != nil {
			return err
		}
		orderCode, err := generateOrderCode(tx)
		if err != nil {
			return err
		}

		orderStatus := models.OrderPendingPayment
		paymentStatus := models.PaymentWaitingConfirmation
		var paidAt *time.Time
		var confirmedBy *uint

		if request.PaymentMethod == models.PaymentCash {
			if public {
				// Customer QR + CASH: menunggu kasir menerima uang, belum masuk kitchen.
				paymentStatus = models.PaymentWaitingConfirmation
			} else {
				// Cashier manual order + CASH: uang sudah diterima kasir,
				// jadi langsung paid dan masuk kitchen.
				now := time.Now()
				orderStatus = models.OrderSentToKitchen
				paymentStatus = models.PaymentPaid
				paidAt = &now
				confirmedBy = createdBy
			}
		}

		customerID, err := findOrCreateOrderCustomer(tx, request)
		if err != nil {
			return err
		}

		order = models.Order{
			OrderCode:     orderCode,
			OrderType:     request.OrderType,
			TableID:       request.TableID,
			CustomerID:    customerID,
			CustomerName:  request.CustomerName,
			CustomerPhone: request.CustomerPhone,
			TotalAmount:   totalAmount,
			Status:        orderStatus,
			CreatedBy:     createdBy,
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		for index := range items {
			items[index].OrderID = order.ID
		}
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
		for index := range items {
			for optionIndex := range itemOptions[index] {
				itemOptions[index][optionIndex].OrderItemID = items[index].ID
			}
			if len(itemOptions[index]) > 0 {
				if err := tx.Create(&itemOptions[index]).Error; err != nil {
					return err
				}
			}
		}

		payment := models.Payment{
			OrderID:       order.ID,
			PaymentMethod: request.PaymentMethod,
			Amount:        totalAmount,
			Status:        paymentStatus,
			PaidAt:        paidAt,
			ConfirmedBy:   confirmedBy,
		}
		return tx.Create(&payment).Error
	})
	if err != nil {
		return models.Order{}, err
	}

	return findOrder(db, order.ID)
}

func validateCreateOrderRequest(request createOrderRequest, public bool) string {
	if public && request.OrderType != models.OrderDineIn && request.OrderType != models.OrderTakeAway {
		return "order_type must be dine_in or take_away"
	}
	if !public && request.OrderType != models.OrderDineIn && request.OrderType != models.OrderTakeAway && request.OrderType != models.OrderCashier {
		return "order_type must be dine_in, take_away, or cashier"
	}
	if request.OrderType == models.OrderDineIn && (request.TableID == nil || *request.TableID == 0) {
		return "table_id is required for dine_in order"
	}
	if request.OrderType != models.OrderDineIn && request.TableID != nil {
		return "table_id is only allowed for dine_in order"
	}
	if len(request.Items) == 0 {
		return "items are required"
	}
	if public && request.CustomerName == "" {
		return "customer_name is required"
	}
	if public && request.CustomerPhone == "" {
		return "customer_phone is required"
	}
	if len(request.CustomerName) > 150 {
		return "customer_name cannot exceed 150 characters"
	}
	if len(request.CustomerPhone) > 30 {
		return "customer_phone cannot exceed 30 characters"
	}
	if len(request.CustomerEmail) > 150 {
		return "customer_email cannot exceed 150 characters"
	}
	for _, item := range request.Items {
		if item.MenuID == 0 {
			return "menu_id is required for every item"
		}
		if item.Quantity < 1 {
			return "quantity must be at least 1"
		}
	}
	if public && !isValidPublicPaymentMethod(request.PaymentMethod) {
		return "payment_method must be cash or qris"
	}
	if !public && !isValidCashierPaymentMethod(request.PaymentMethod) {
		return "payment_method must be cash or qris"
	}
	return ""
}

func findOrCreateOrderCustomer(tx *gorm.DB, request createOrderRequest) (*uint, error) {
	if request.CustomerPhone == "" {
		return nil, nil
	}

	var customer models.Customer
	err := tx.Where("phone = ?", request.CustomerPhone).First(&customer).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		customer = models.Customer{
			Name:  request.CustomerName,
			Phone: request.CustomerPhone,
			Email: request.CustomerEmail,
		}
		if customer.Name == "" {
			customer.Name = request.CustomerPhone
		}
		if err := tx.Create(&customer).Error; err != nil {
			return nil, err
		}
		return &customer.ID, nil
	}

	updates := map[string]any{}
	if request.CustomerName != "" && request.CustomerName != customer.Name {
		updates["name"] = request.CustomerName
	}
	if request.CustomerEmail != "" && request.CustomerEmail != customer.Email {
		updates["email"] = request.CustomerEmail
	}
	if len(updates) > 0 {
		if err := tx.Model(&customer).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	return &customer.ID, nil
}

func buildOrderItems(tx *gorm.DB, requests []orderItemRequest) ([]models.OrderItem, [][]models.OrderItemOption, int64, error) {
	items := make([]models.OrderItem, 0, len(requests))
	itemOptions := make([][]models.OrderItemOption, 0, len(requests))
	var totalAmount int64
	for _, request := range requests {
		var menu models.Menu
		if err := tx.First(&menu, request.MenuID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil, 0, &orderServiceError{Status: 404, Message: fmt.Sprintf("menu with id %d not found", request.MenuID)}
			}
			return nil, nil, 0, err
		}
		if !menu.IsAvailable {
			return nil, nil, 0, &orderServiceError{Status: 409, Message: fmt.Sprintf("menu %s is not available", menu.Name)}
		}

		options, optionsTotal, err := buildOrderItemOptions(tx, menu, request.Options)
		if err != nil {
			return nil, nil, 0, err
		}
		unitPrice := menu.Price + optionsTotal
		subtotal := unitPrice * int64(request.Quantity)
		totalAmount += subtotal
		note := strings.TrimSpace(request.Note)
		if note == "" {
			note = strings.TrimSpace(request.Notes)
		}
		items = append(items, models.OrderItem{
			MenuID:    menu.ID,
			Quantity:  request.Quantity,
			Price:     menu.Price,
			UnitPrice: unitPrice,
			Subtotal:  subtotal,
			Note:      note,
		})
		itemOptions = append(itemOptions, options)
	}
	return items, itemOptions, totalAmount, nil
}

func buildOrderItemOptions(tx *gorm.DB, menu models.Menu, requests []orderItemOptionRequest) ([]models.OrderItemOption, int64, error) {
	var groups []models.MenuOptionGroup
	if err := tx.
		Preload("Options", "is_active = ?", true).
		Where("menu_id = ? AND is_active = ?", menu.ID, true).
		Find(&groups).Error; err != nil {
		return nil, 0, err
	}

	groupByID := make(map[uint]models.MenuOptionGroup, len(groups))
	optionByID := map[uint]models.MenuOption{}
	for _, group := range groups {
		groupByID[group.ID] = group
		for _, option := range group.Options {
			optionByID[option.ID] = option
		}
	}

	selectedByGroup := map[uint][]models.MenuOption{}
	seenOptions := map[uint]bool{}
	for _, request := range requests {
		group, ok := groupByID[request.GroupID]
		if !ok {
			return nil, 0, &orderServiceError{Status: 400, Message: fmt.Sprintf("option group %d is not available for menu %s", request.GroupID, menu.Name)}
		}
		option, ok := optionByID[request.OptionID]
		if !ok || option.GroupID != group.ID {
			return nil, 0, &orderServiceError{Status: 400, Message: fmt.Sprintf("option %d is not available for group %s", request.OptionID, group.Name)}
		}
		if seenOptions[option.ID] {
			return nil, 0, &orderServiceError{Status: 400, Message: fmt.Sprintf("option %s is selected more than once", option.Name)}
		}
		seenOptions[option.ID] = true
		selectedByGroup[group.ID] = append(selectedByGroup[group.ID], option)
	}

	for _, group := range groups {
		selectedCount := len(selectedByGroup[group.ID])
		if group.Required && selectedCount == 0 {
			return nil, 0, &orderServiceError{Status: 400, Message: fmt.Sprintf("option group %s is required", group.Name)}
		}
		if selectedCount < group.MinSelect {
			return nil, 0, &orderServiceError{Status: 400, Message: fmt.Sprintf("option group %s requires at least %d selection(s)", group.Name, group.MinSelect)}
		}
		if selectedCount > group.MaxSelect {
			return nil, 0, &orderServiceError{Status: 400, Message: fmt.Sprintf("option group %s allows at most %d selection(s)", group.Name, group.MaxSelect)}
		}
		if group.Type == models.MenuOptionGroupSingle && selectedCount > 1 {
			return nil, 0, &orderServiceError{Status: 400, Message: fmt.Sprintf("option group %s only allows one selection", group.Name)}
		}
	}

	options := make([]models.OrderItemOption, 0, len(requests))
	var total int64
	for _, request := range requests {
		group := groupByID[request.GroupID]
		option := optionByID[request.OptionID]
		groupID := group.ID
		optionID := option.ID
		total += option.AdditionalPrice
		options = append(options, models.OrderItemOption{
			MenuOptionGroupID: &groupID,
			MenuOptionID:      &optionID,
			GroupName:         group.Name,
			OptionName:        option.Name,
			AdditionalPrice:   option.AdditionalPrice,
		})
	}

	return options, total, nil
}

func generateOrderCode(tx *gorm.DB) (string, error) {
	now := time.Now().In(jakartaLocation)
	date := now.Format("20060102")
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "order-code-"+date).Error; err != nil {
		return "", err
	}

	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, jakartaLocation)
	var count int64
	if err := tx.Model(&models.Order{}).
		Where("created_at >= ? AND created_at < ?", startOfDay, startOfDay.AddDate(0, 0, 1)).
		Count(&count).Error; err != nil {
		return "", err
	}
	return fmt.Sprintf("ORD-%s-%04d", date, count+1), nil
}

func findOrder(db *gorm.DB, id uint) (models.Order, error) {
	var order models.Order
	err := preloadOrder(db).First(&order, id).Error
	return order, err
}

func preloadOrder(db *gorm.DB) *gorm.DB {
	return db.
		Preload("Table").
		Preload("Customer").
		Preload("Creator").
		Preload("Items.Menu").
		Preload("Items.Menu.Category").
		Preload("Items.Options").
		Preload("Payment").
		Preload("Payment.Confirmer")
}

func isValidPublicPaymentMethod(method models.PaymentMethod) bool {
	return method == models.PaymentCash || method == models.PaymentQRIS
}

func isValidCashierPaymentMethod(method models.PaymentMethod) bool {
	return method == models.PaymentCash || method == models.PaymentQRIS
}
