package services

import (
	"reflect"
	"testing"

	"pos-backend/models"
)

func TestEnabledPaymentsForMethod(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		want    []string
		wantErr bool
	}{
		{name: "qris only", method: "qris", want: []string{"qris"}},
		{name: "trim and lowercase", method: " QRIS ", want: []string{"qris"}},
		{name: "gopay is rejected", method: "gopay", wantErr: true},
		{name: "shopeepay is rejected", method: "shopeepay", wantErr: true},
		{name: "invalid", method: "bank_transfer", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EnabledPaymentsForMethod(tt.method)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("enabled payments = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildPaymentPayloadUsesQRISOnly(t *testing.T) {
	service := &MidtransService{}

	payload, enabledPayments, err := service.BuildPaymentPayload(CreatePaymentTransactionInput{
		OrderID:       "ORD-TEST-QRIS",
		GrossAmount:   30000,
		PaymentMethod: "qris",
		Customer: PaymentCustomer{
			FirstName: "Daniel",
			Email:     "daniel@example.com",
			Phone:     "08123456789",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(enabledPayments, []string{"qris"}) {
		t.Fatalf("enabled payments = %v, want [qris]", enabledPayments)
	}
	if !reflect.DeepEqual(payload["enabled_payments"], []string{"qris"}) {
		t.Fatalf("payload enabled payments = %v, want [qris]", payload["enabled_payments"])
	}
}

func TestExistingOrderSnapPayloadUsesQRISOnlyForOnlineMethod(t *testing.T) {
	service := &MidtransService{}

	payload, err := service.buildSnapPayload(models.Order{
		OrderCode:    "ORD-TEST-ONLINE",
		TotalAmount:  30000,
		CustomerName: "Daniel",
	}, models.Payment{PaymentMethod: models.PaymentOnline})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(payload["enabled_payments"], []string{"qris"}) {
		t.Fatalf("payload enabled payments = %v, want [qris]", payload["enabled_payments"])
	}
}
