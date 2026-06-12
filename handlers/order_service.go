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
	MenuID   uint   `json:"menu_id"`
	Quantity int    `json:"quantity"`
	Note     string `json:"note"`
}

type createOrderRequest struct {
	OrderType     models.OrderType     `json:"order_type"`
	TableID       *uint                `json:"table_id"`
	CustomerName  string               `json:"customer_name"`
	CustomerPhone string               `json:"customer_phone"`
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
	if message := validateCreateOrderRequest(request, public); message != "" {
		return models.Order{}, &orderServiceError{Status: 400, Message: message}
	}

	var order models.Order
	err := db.Transaction(func(tx *gorm.DB) error {
		items, totalAmount, err := buildOrderItems(tx, request.Items)
		if err != nil {
			return err
		}
		orderCode, err := generateOrderCode(tx)
		if err != nil {
			return err
		}

		order = models.Order{
			OrderCode:     orderCode,
			OrderType:     request.OrderType,
			TableID:       request.TableID,
			CustomerName:  strings.TrimSpace(request.CustomerName),
			CustomerPhone: strings.TrimSpace(request.CustomerPhone),
			TotalAmount:   totalAmount,
			Status:        models.OrderPendingPayment,
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

		paymentStatus := models.PaymentWaitingConfirmation
		if request.PaymentMethod == models.PaymentCash {
			paymentStatus = models.PaymentUnpaid
		}
		payment := models.Payment{
			OrderID:       order.ID,
			PaymentMethod: request.PaymentMethod,
			Amount:        totalAmount,
			Status:        paymentStatus,
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
	if len(strings.TrimSpace(request.CustomerName)) > 150 {
		return "customer_name cannot exceed 150 characters"
	}
	if len(strings.TrimSpace(request.CustomerPhone)) > 30 {
		return "customer_phone cannot exceed 30 characters"
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
		return "payment_method must be cash, qris, qris_manual, or transfer"
	}
	return ""
}

func buildOrderItems(tx *gorm.DB, requests []orderItemRequest) ([]models.OrderItem, int64, error) {
	items := make([]models.OrderItem, 0, len(requests))
	var totalAmount int64
	for _, request := range requests {
		var menu models.Menu
		if err := tx.First(&menu, request.MenuID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, 0, &orderServiceError{Status: 404, Message: fmt.Sprintf("menu with id %d not found", request.MenuID)}
			}
			return nil, 0, err
		}
		if !menu.IsAvailable {
			return nil, 0, &orderServiceError{Status: 409, Message: fmt.Sprintf("menu %s is not available", menu.Name)}
		}
		subtotal := menu.Price * int64(request.Quantity)
		totalAmount += subtotal
		items = append(items, models.OrderItem{
			MenuID:   menu.ID,
			Quantity: request.Quantity,
			Price:    menu.Price,
			Subtotal: subtotal,
			Note:     strings.TrimSpace(request.Note),
		})
	}
	return items, totalAmount, nil
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
		Preload("Creator").
		Preload("Items.Menu").
		Preload("Items.Menu.Category").
		Preload("Payment").
		Preload("Payment.Confirmer")
}

func isValidPublicPaymentMethod(method models.PaymentMethod) bool {
	return method == models.PaymentCash || method == models.PaymentQRIS
}

func isValidCashierPaymentMethod(method models.PaymentMethod) bool {
	return method == models.PaymentCash ||
		method == models.PaymentQRIS ||
		method == models.PaymentQRISManual ||
		method == models.PaymentTransfer
}
