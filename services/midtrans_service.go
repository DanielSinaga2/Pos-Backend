package services

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"pos-backend/config"
	"pos-backend/models"
)

const (
	sandboxSnapURL       = "https://app.sandbox.midtrans.com/snap/v1/transactions"
	productionSnapURL    = "https://app.midtrans.com/snap/v1/transactions"
	sandboxStatusBaseURL = "https://api.sandbox.midtrans.com/v2"
	productionStatusURL  = "https://api.midtrans.com/v2"
)

type MidtransService struct {
	serverKey     string
	frontendURL   string
	snapURL       string
	statusBaseURL string
	client        *http.Client
}

type SnapResponse struct {
	Token       string `json:"token"`
	RedirectURL string `json:"redirect_url"`
}

type NotificationPayload struct {
	OrderID           string `json:"order_id"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	PaymentType       string `json:"payment_type"`
	TransactionID     string `json:"transaction_id"`
	GrossAmount       string `json:"gross_amount"`
	StatusCode        string `json:"status_code"`
	SignatureKey      string `json:"signature_key"`
}

type TransactionStatusResponse struct {
	OrderID           string `json:"order_id"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	PaymentType       string `json:"payment_type"`
	TransactionID     string `json:"transaction_id"`
	GrossAmount       string `json:"gross_amount"`
	StatusCode        string `json:"status_code"`
	StatusMessage     string `json:"status_message"`
}

func NewMidtransService(cfg *config.Config) *MidtransService {
	snapURL := sandboxSnapURL
	statusBaseURL := sandboxStatusBaseURL
	if cfg.MidtransIsProduction {
		snapURL = productionSnapURL
		statusBaseURL = productionStatusURL
	}

	return &MidtransService{
		serverKey:     cfg.MidtransServerKey,
		frontendURL:   cfg.FrontendURL,
		snapURL:       snapURL,
		statusBaseURL: statusBaseURL,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *MidtransService) CreateSnapTransaction(order models.Order, payment models.Payment) (SnapResponse, error) {
	if s.serverKey == "" {
		return SnapResponse{}, errors.New("MIDTRANS_SERVER_KEY is required")
	}

	payload, err := s.buildSnapPayload(order, payment)
	if err != nil {
		return SnapResponse{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return SnapResponse{}, err
	}

	req, err := http.NewRequest(http.MethodPost, s.snapURL, bytes.NewReader(body))
	if err != nil {
		return SnapResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(s.serverKey, "")

	resp, err := s.client.Do(req)
	if err != nil {
		return SnapResponse{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return SnapResponse{}, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return SnapResponse{}, fmt.Errorf("midtrans snap request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	var snapResponse SnapResponse
	if err := json.Unmarshal(responseBody, &snapResponse); err != nil {
		return SnapResponse{}, err
	}
	if snapResponse.Token == "" || snapResponse.RedirectURL == "" {
		return SnapResponse{}, errors.New("midtrans snap response is incomplete")
	}
	return snapResponse, nil
}

func (s *MidtransService) GetTransactionStatus(orderID string) (TransactionStatusResponse, error) {
	if s.serverKey == "" {
		return TransactionStatusResponse{}, errors.New("MIDTRANS_SERVER_KEY is required")
	}

	statusURL := fmt.Sprintf("%s/%s/status", s.statusBaseURL, url.PathEscape(orderID))
	req, err := http.NewRequest(http.MethodGet, statusURL, nil)
	if err != nil {
		return TransactionStatusResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(s.serverKey, "")

	resp, err := s.client.Do(req)
	if err != nil {
		return TransactionStatusResponse{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return TransactionStatusResponse{}, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return TransactionStatusResponse{}, fmt.Errorf("midtrans status request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	var statusResponse TransactionStatusResponse
	if err := json.Unmarshal(responseBody, &statusResponse); err != nil {
		return TransactionStatusResponse{}, err
	}
	return statusResponse, nil
}

func (s *MidtransService) VerifyNotification(payload NotificationPayload) bool {
	signatureSource := payload.OrderID + payload.StatusCode + payload.GrossAmount + s.serverKey
	sum := sha512.Sum512([]byte(signatureSource))
	return strings.EqualFold(hex.EncodeToString(sum[:]), payload.SignatureKey)
}

func (s *MidtransService) buildSnapPayload(order models.Order, payment models.Payment) (map[string]any, error) {
	if order.TotalAmount <= 0 {
		return nil, errors.New("gross_amount must be positive")
	}

	itemDetails := make([]map[string]any, 0, len(order.Items))
	for _, item := range order.Items {
		name := item.Menu.Name
		nameRunes := []rune(name)
		if len(nameRunes) > 50 {
			name = string(nameRunes[:50])
		}
		itemDetails = append(itemDetails, map[string]any{
			"id":       strconv.FormatUint(uint64(item.MenuID), 10),
			"price":    item.Price,
			"quantity": item.Quantity,
			"name":     name,
		})
	}

	enabledPayments := enabledPaymentsFor(payment.PaymentMethod)
	finishURL := s.customerPaymentFinishURL(order.OrderCode)

	customerName := strings.TrimSpace(order.CustomerName)
	if customerName == "" {
		customerName = order.OrderCode
	}
	customerDetails := map[string]any{
		"first_name": customerName,
	}
	if customerPhone := strings.TrimSpace(order.CustomerPhone); customerPhone != "" {
		customerDetails["phone"] = customerPhone
	}

	return map[string]any{
		"transaction_details": map[string]any{
			"order_id":     order.OrderCode,
			"gross_amount": order.TotalAmount,
		},
		"customer_details": customerDetails,
		"finish_url":       finishURL,
		"callbacks": map[string]any{
			"finish": finishURL,
		},
		"enabled_payments": enabledPayments,
		"item_details":     itemDetails,
	}, nil
}

func (s *MidtransService) customerPaymentFinishURL(orderCode string) string {
	return strings.TrimRight(s.frontendURL, "/") + "/order/payment-return?order_code=" + url.QueryEscape(orderCode)
}

func enabledPaymentsFor(method models.PaymentMethod) []string {
	if method == models.PaymentTransfer {
		return []string{"bank_transfer"}
	}

	return []string{"gopay", "shopeepay"}
}
