package dto

type CorePaymentCreateRequest struct {
	OrderID string `json:"order_id"`
	Amount  int64  `json:"amount"`
}

type CorePaymentCreateResponse struct {
	Success           bool   `json:"success"`
	OrderID           string `json:"order_id"`
	TransactionID     string `json:"transaction_id"`
	GrossAmount       int64  `json:"gross_amount"`
	QRURL             string `json:"qr_url"`
	ExpiryTime        string `json:"expiry_time"`
	TransactionStatus string `json:"transaction_status"`
}

type CorePaymentStatusResponse struct {
	Status string `json:"status"`
}
