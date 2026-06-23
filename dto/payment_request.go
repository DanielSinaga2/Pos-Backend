package dto

type PaymentCustomerRequest struct {
	FirstName string `json:"first_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
}

type CreatePaymentRequest struct {
	OrderID       string                 `json:"order_id"`
	GrossAmount   int64                  `json:"gross_amount"`
	Customer      PaymentCustomerRequest `json:"customer"`
	PaymentMethod string                 `json:"payment_method"`
}
