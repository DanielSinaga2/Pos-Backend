package services

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"pos-backend/dto"
	kitchenws "pos-backend/internal/ws"
	"pos-backend/models"
	realtime "pos-backend/websocket"

	"github.com/midtrans/midtrans-go"
	"github.com/midtrans/midtrans-go/coreapi"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const generateQRCodeAction = "generate-qr-code"

var (
	ErrInvalidAmount     = errors.New("amount must be greater than 0")
	ErrMidtransNotConfig = errors.New("MIDTRANS_SERVER_KEY is required")
)

type corePaymentGateway interface {
	Charge(orderID string, amount int64) (*coreapi.ChargeResponse, error)
	Status(orderID string) (*coreapi.TransactionStatusResponse, error)
}

type midtransCoreGateway struct {
	client *coreapi.Client
}

func (g *midtransCoreGateway) Charge(orderID string, amount int64) (*coreapi.ChargeResponse, error) {
	response, err := g.client.ChargeTransaction(&coreapi.ChargeReq{
		PaymentType: coreapi.PaymentTypeQris,
		TransactionDetails: midtrans.TransactionDetails{
			OrderID:  orderID,
			GrossAmt: amount,
		},
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

func (g *midtransCoreGateway) Status(orderID string) (*coreapi.TransactionStatusResponse, error) {
	response, err := g.client.CheckTransaction(orderID)
	if err != nil {
		return nil, err
	}

	return response, nil
}

type PaymentService struct {
	gateway    corePaymentGateway
	configured bool
	db         *gorm.DB
}

func NewPaymentService(client *coreapi.Client, db ...*gorm.DB) *PaymentService {
	var database *gorm.DB
	if len(db) > 0 {
		database = db[0]
	}

	if client == nil {
		return &PaymentService{db: database}
	}

	return &PaymentService{
		gateway:    &midtransCoreGateway{client: client},
		configured: strings.TrimSpace(client.ServerKey) != "",
		db:         database,
	}
}

func (s *PaymentService) CreateQRIS(
	orderID string,
	amount int64,
) (dto.CorePaymentCreateResponse, error) {

	if amount <= 0 {
		return dto.CorePaymentCreateResponse{}, ErrInvalidAmount
	}

	if strings.TrimSpace(orderID) == "" {
		return dto.CorePaymentCreateResponse{}, errors.New("order_id is required")
	}

	if !s.configured || s.gateway == nil {
		return dto.CorePaymentCreateResponse{}, ErrMidtransNotConfig
	}

	charge, err := s.gateway.Charge(orderID, amount)
	if err != nil {
		return dto.CorePaymentCreateResponse{}, fmt.Errorf(
			"charge QRIS transaction: %w",
			err,
		)
	}

	qrURL := ""

	for _, action := range charge.Actions {
		if action.Name == generateQRCodeAction {
			qrURL = action.URL
			break
		}
	}

	if qrURL == "" {
		return dto.CorePaymentCreateResponse{},
			errors.New("Midtrans response does not contain generate-qr-code action")
	}

	// Save payment to DB immediately after a successful charge (201 or 200).
	if s.db != nil && (charge.StatusCode == "201" || charge.StatusCode == "200") {
		if err := s.saveCorePaymentCreation(orderID, charge); err != nil {
			log.Printf("create Core API QRIS payment metadata sync skipped order_id=%s error=%v", orderID, err)
		}
	}

	return dto.CorePaymentCreateResponse{
		Success:           true,
		OrderID:           charge.OrderID,
		TransactionID:     charge.TransactionID,
		GrossAmount:       amount,
		QRURL:             qrURL,
		ExpiryTime:        charge.ExpiryTime,
		TransactionStatus: charge.TransactionStatus,
	}, nil
}

func (s *PaymentService) GetStatus(orderID string) (dto.CorePaymentStatusResponse, error) {
	orderID = strings.TrimSpace(orderID)

	if orderID == "" {
		return dto.CorePaymentStatusResponse{}, errors.New("order_id is required")
	}

	// --- 1. Try DB first: look up payment by midtrans_order_id or order_code ---
	if s.db != nil {
		dbStatus, found, err := s.getStatusFromDB(orderID)
		if err != nil {
			log.Printf("GetStatus DB lookup error order_id=%s: %v", orderID, err)
			// Non-fatal: fall through to Midtrans API
		} else if found {
			return dbStatus, nil
		}
	}

	// --- 2. DB record not found — require Midtrans gateway ---
	if !s.configured || s.gateway == nil {
		return dto.CorePaymentStatusResponse{}, ErrMidtransNotConfig
	}

	// --- 3. Resolve midtrans_order_id from local order_code if possible ---
	midtransOrderID := orderID
	if s.db != nil {
		if resolved, err := s.resolveMidtransOrderID(orderID); err == nil {
			midtransOrderID = resolved
		}
	}

	transaction, err := s.gateway.Status(midtransOrderID)
	if err != nil {
		return dto.CorePaymentStatusResponse{}, fmt.Errorf(
			"get Midtrans transaction status: %w",
			err,
		)
	}
	log.Printf(
		"Status sync local order=%s midtrans_order=%s transaction_status=%s",
		orderID,
		midtransOrderID,
		transaction.TransactionStatus,
	)

	if s.db != nil {
		if err := s.syncStatusToDatabase(midtransOrderID, transaction); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Printf("Midtrans status sync skipped: payment or order not found for midtrans_order_id=%s", midtransOrderID)
			} else {
				log.Printf("Midtrans status sync error for midtrans_order_id=%s: %v", midtransOrderID, err)
			}
		}
	}

	return dto.CorePaymentStatusResponse{
		Status:            transaction.TransactionStatus,
		TransactionStatus: transaction.TransactionStatus,
	}, nil
}

// getStatusFromDB looks up a payment record in the local database by midtrans_order_id
// (direct match) or by order_code (fallback). Returns the status and true if found.
func (s *PaymentService) getStatusFromDB(orderID string) (dto.CorePaymentStatusResponse, bool, error) {
	var payment models.Payment

	// Try direct midtrans_order_id lookup first.
	err := s.db.Where("midtrans_order_id = ?", orderID).First(&payment).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return dto.CorePaymentStatusResponse{}, false, err
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Fallback: look up by order_code.
		var order models.Order
		if err2 := s.db.Where("order_code = ?", orderID).First(&order).Error; err2 != nil {
			if errors.Is(err2, gorm.ErrRecordNotFound) {
				return dto.CorePaymentStatusResponse{}, false, nil
			}
			return dto.CorePaymentStatusResponse{}, false, err2
		}
		if err2 := s.db.Where("order_id = ?", order.ID).First(&payment).Error; err2 != nil {
			if errors.Is(err2, gorm.ErrRecordNotFound) {
				return dto.CorePaymentStatusResponse{}, false, nil
			}
			return dto.CorePaymentStatusResponse{}, false, err2
		}
	}

	// Only consider the payment record "found" if it has a midtrans_order_id set.
	if payment.MidtransOrderID == nil || strings.TrimSpace(*payment.MidtransOrderID) == "" {
		return dto.CorePaymentStatusResponse{}, false, nil
	}

	log.Printf("GetStatus served from DB: midtrans_order_id=%s status=%s", *payment.MidtransOrderID, payment.Status)
	return dto.CorePaymentStatusResponse{
		Status:            string(payment.Status),
		TransactionStatus: string(payment.Status),
	}, true, nil
}

// resolveMidtransOrderID looks up the local Order by order_code,
// finds the associated Payment, and returns payment.MidtransOrderID.
// This is needed because the frontend sends the local order_code
// (e.g. ORD-20260623-0003) but Midtrans only knows the midtrans_order_id
// that was used during Charge.
func (s *PaymentService) resolveMidtransOrderID(orderCode string) (string, error) {
	var order models.Order
	if err := s.db.Where("order_code = ?", orderCode).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("order not found for order_code=%s", orderCode)
		}
		return "", err
	}

	var payment models.Payment
	if err := s.db.Where("order_id = ?", order.ID).First(&payment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("payment not found for order_id=%d (order_code=%s)", order.ID, orderCode)
		}
		return "", err
	}

	if payment.MidtransOrderID == nil || strings.TrimSpace(*payment.MidtransOrderID) == "" {
		return "", fmt.Errorf("midtrans_order_id is empty for order_code=%s payment_id=%d", orderCode, payment.ID)
	}

	resolved := strings.TrimSpace(*payment.MidtransOrderID)
	log.Printf("Resolved midtrans_order_id: order_code=%s → midtrans_order_id=%s", orderCode, resolved)
	return resolved, nil
}

func (s *PaymentService) saveCorePaymentCreation(orderID string, charge *coreapi.ChargeResponse) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// orderID here is the local order_code (e.g. ORD-20260624-0003) that was
		// sent to Midtrans as the transaction order_id. At the point of charge
		// creation the payment row has no midtrans_order_id yet, so we must
		// look it up by order_code → order → payment.
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_code = ?", orderID).
			First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Printf("Core API QRIS metadata sync: order not found for order_code=%s", orderID)
				return nil
			}
			return err
		}

		var payment models.Payment
		paymentFound := true
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_id = ?", order.ID).
			First(&payment).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			// No payment row yet — we'll create one below.
			paymentFound = false
			log.Printf("Core API QRIS metadata sync: no payment row for order_id=%d, will create one", order.ID)
		}

		midtransOrderID := strings.TrimSpace(charge.OrderID)
		if midtransOrderID == "" {
			midtransOrderID = orderID
		}

		grossAmount := parseGrossAmount(charge.GrossAmount)
		qrString := nullableString(charge.QRString)
		expiryTime := nullableString(charge.ExpiryTime)
		transactionID := nullableString(charge.TransactionID)
		paymentType := corePaymentType(charge.PaymentType)

		if paymentFound {
			// Payment row already exists — update it with Midtrans data.
			updates := map[string]any{
				"midtrans_order_id": midtransOrderID,
				"transaction_id":    transactionID,
				"payment_type":      paymentType,
				"gross_amount":      grossAmount,
				"qr_string":         qrString,
				"expiry_time":       expiryTime,
			}
			if charge.TransactionStatus == "pending" {
				updates["status"] = models.PaymentPending
			}
			if err := tx.Model(&payment).Updates(updates).Error; err != nil {
				return fmt.Errorf("update Core API QRIS payment fields order_code=%s: %w", orderID, err)
			}
		} else {
			// No payment row yet — create one.
			paymentStatus := models.PaymentPending
			if charge.TransactionStatus != "pending" {
				paymentStatus = models.PaymentUnpaid
			}
			payment = models.Payment{
				OrderID:         order.ID,
				PaymentMethod:   models.PaymentQRIS,
				Amount:          grossAmount,
				Status:          paymentStatus,
				MidtransOrderID: &midtransOrderID,
				TransactionID:   transactionID,
				PaymentType:     paymentType,
				GrossAmount:     &grossAmount,
				QRString:        qrString,
				ExpiryTime:      expiryTime,
			}
			if err := tx.Create(&payment).Error; err != nil {
				return fmt.Errorf("create Core API QRIS payment record order_code=%s: %w", orderID, err)
			}
		}

		log.Printf("Core API QRIS payment saved (created=%v): order_code=%s midtrans_order_id=%s status=%s",
			!paymentFound, orderID, midtransOrderID, charge.TransactionStatus)

		if charge.TransactionStatus == "pending" && order.Status != models.OrderPendingPayment {
			return tx.Model(&order).Update("status", models.OrderPendingPayment).Error
		}
		return nil
	})
}

func (s *PaymentService) syncStatusToDatabase(orderID string, transaction *coreapi.TransactionStatusResponse) error {
	var syncedOrderID uint

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		payment, err := findCorePaymentForMidtransOrder(tx, orderID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Printf("Midtrans status sync payment not found midtrans_order_id=%s", orderID)
			}
			return err
		}

		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&order, payment.OrderID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Printf("Midtrans status sync order not found midtrans_order_id=%s order_id=%d", orderID, payment.OrderID)
			}
			return err
		}

		if err := applyCorePaymentStatus(tx, &payment, &order, transaction); err != nil {
			return err
		}
		syncedOrderID = order.ID
		return nil
	}); err != nil {
		return err
	}

	if syncedOrderID != 0 {
		var order models.Order
		if err := s.preloadOrder().First(&order, syncedOrderID).Error; err != nil {
			log.Printf("Midtrans status sync broadcast skipped: failed to load order id=%d error=%v", syncedOrderID, err)
			return nil
		}
		broadcastCorePaymentUpdate(order)
	}

	return nil
}

func findCorePaymentForMidtransOrder(tx *gorm.DB, orderID string) (models.Payment, error) {
	var payment models.Payment
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("midtrans_order_id = ?", orderID).
		First(&payment).Error
	if err == nil || !errors.Is(err, gorm.ErrRecordNotFound) {
		return payment, err
	}

	var order models.Order
	if err := tx.Where("order_code = ?", orderID).First(&order).Error; err != nil {
		return models.Payment{}, err
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("order_id = ?", order.ID).
		First(&payment).Error; err != nil {
		return models.Payment{}, err
	}
	return payment, nil
}

func applyCorePaymentStatus(tx *gorm.DB, payment *models.Payment, order *models.Order, transaction *coreapi.TransactionStatusResponse) error {
	updates := map[string]any{
		"payment_type":   nullableString(transaction.PaymentType),
		"transaction_id": nullableString(transaction.TransactionID),
		"fraud_status":   nullableString(transaction.FraudStatus),
	}

	switch transaction.TransactionStatus {
	case "settlement", "capture":
		now := time.Now()
		updates["status"] = models.PaymentPaid
		updates["paid_at"] = &now
		if err := tx.Model(payment).Updates(updates).Error; err != nil {
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
	default:
		return tx.Model(payment).Updates(updates).Error
	}

	return nil
}

func (s *PaymentService) preloadOrder() *gorm.DB {
	return s.db.
		Preload("Table").
		Preload("Customer").
		Preload("Payment").
		Preload("Items.Menu").
		Preload("Items.Options")
}

func broadcastCorePaymentUpdate(order models.Order) {
	if order.Payment == nil {
		realtime.BroadcastToCashier("order_updated", order)
		realtime.BroadcastToKitchen("order_updated", order)
		realtime.BroadcastToCustomer(order.OrderCode, "order_updated", order)
		return
	}

	switch order.Payment.Status {
	case models.PaymentPaid:
		kitchenws.BroadcastNewOrder(order)
		realtime.BroadcastToCashier("payment_confirmed", order)
		realtime.BroadcastToKitchen("payment_confirmed", order)
		realtime.BroadcastToCustomer(order.OrderCode, "payment_confirmed", order)
		realtime.BroadcastToCashier("order_sent_to_kitchen", order)
		realtime.BroadcastToKitchen("order_sent_to_kitchen", order)
		realtime.BroadcastToCustomer(order.OrderCode, "order_sent_to_kitchen", order)
	case models.PaymentPending:
		realtime.BroadcastToCashier("payment_waiting_confirmation", order)
		realtime.BroadcastToCustomer(order.OrderCode, "payment_waiting_confirmation", order)
	case models.PaymentFailed, models.PaymentCancelled:
		realtime.BroadcastToCashier("order_cancelled", order)
		realtime.BroadcastToKitchen("order_cancelled", order)
		realtime.BroadcastToCustomer(order.OrderCode, "order_cancelled", order)
	default:
		realtime.BroadcastToCashier("order_updated", order)
		realtime.BroadcastToKitchen("order_updated", order)
		realtime.BroadcastToCustomer(order.OrderCode, "order_updated", order)
	}
}

func nullableString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func corePaymentType(value string) *string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return &trimmed
	}
	return nullableString("qris")
}

// parseGrossAmount converts Midtrans' string gross_amount (e.g. "45000.00")
// to an int64 (e.g. 45000). Returns 0 if the string is empty or unparseable.
func parseGrossAmount(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Midtrans returns amounts as "45000.00" – truncate the decimal part.
	if idx := strings.Index(s, "."); idx >= 0 {
		s = s[:idx]
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}
