package dto

type MidtransPayloadSummary struct {
	EnabledPayments []string `json:"enabled_payments"`
	GrossAmount     int64    `json:"gross_amount"`
}

type CreatePaymentResponse struct {
	OrderID                string                 `json:"order_id"`
	PaymentMethod          string                 `json:"payment_method"`
	SnapToken              string                 `json:"snap_token"`
	RedirectURL            string                 `json:"redirect_url"`
	MidtransPayloadSummary MidtransPayloadSummary `json:"midtrans_payload_summary"`
}
