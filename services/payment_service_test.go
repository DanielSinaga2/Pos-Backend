package services

import (
	"errors"
	"testing"

	"github.com/midtrans/midtrans-go/coreapi"
)

type fakeCorePaymentGateway struct {
	chargeResponse *coreapi.ChargeResponse
	statusResponse *coreapi.TransactionStatusResponse
	chargeOrderID  string
	chargeAmount   int64
	err            error
}

func (f *fakeCorePaymentGateway) Charge(orderID string, amount int64) (*coreapi.ChargeResponse, error) {
	f.chargeOrderID = orderID
	f.chargeAmount = amount
	return f.chargeResponse, f.err
}

func (f *fakeCorePaymentGateway) Status(_ string) (*coreapi.TransactionStatusResponse, error) {
	return f.statusResponse, f.err
}

func TestPaymentServiceCreateQRIS(t *testing.T) {
	gateway := &fakeCorePaymentGateway{chargeResponse: &coreapi.ChargeResponse{
		OrderID:           "ORD-20260622-0005",
		TransactionID:     "transaction-id",
		TransactionStatus: "pending",
		ExpiryTime:        "2026-06-22 12:00:00",
		Actions: []coreapi.Action{
			{Name: "generate-qr-code", URL: "https://example.test/qr"},
		},
	}}
	service := &PaymentService{
		configured: true,
		gateway:    gateway,
	}

	response, err := service.CreateQRIS("ORD-20260622-0005", 45000)
	if err != nil {
		t.Fatalf("CreateQRIS returned error: %v", err)
	}
	if !response.Success || response.GrossAmount != 45000 || response.QRURL != "https://example.test/qr" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if gateway.chargeOrderID != "ORD-20260622-0005" || gateway.chargeAmount != 45000 {
		t.Fatalf("unexpected charge input: orderID=%s amount=%d", gateway.chargeOrderID, gateway.chargeAmount)
	}
}

func TestPaymentServiceCreateQRISValidatesAmount(t *testing.T) {
	service := &PaymentService{}
	_, err := service.CreateQRIS("ORD-20260622-0005", 0)
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}
}

func TestPaymentServiceCreateQRISValidatesOrderID(t *testing.T) {
	service := &PaymentService{}
	_, err := service.CreateQRIS(" ", 45000)
	if err == nil {
		t.Fatal("expected an error when order_id is empty")
	}
}

func TestPaymentServiceCreateQRISRequiresQRCodeAction(t *testing.T) {
	service := &PaymentService{
		configured: true,
		gateway: &fakeCorePaymentGateway{chargeResponse: &coreapi.ChargeResponse{
			OrderID: "QRIS-order",
		}},
	}
	if _, err := service.CreateQRIS("ORD-20260622-0005", 45000); err == nil {
		t.Fatal("expected an error when generate-qr-code action is missing")
	}
}

func TestPaymentServiceGetStatus(t *testing.T) {
	service := &PaymentService{
		configured: true,
		gateway: &fakeCorePaymentGateway{statusResponse: &coreapi.TransactionStatusResponse{
			TransactionStatus: "settlement",
		}},
	}

	response, err := service.GetStatus("QRIS-order")
	if err != nil {
		t.Fatalf("GetStatus returned error: %v", err)
	}
	if response.Status != "settlement" {
		t.Fatalf("unexpected status: %s", response.Status)
	}
}

func TestPaymentServiceGetStatusReturnsMidtransStatus(t *testing.T) {
	service := &PaymentService{
		configured: true,
		gateway: &fakeCorePaymentGateway{statusResponse: &coreapi.TransactionStatusResponse{
			TransactionStatus: "refund",
		}},
	}

	response, err := service.GetStatus("QRIS-order")
	if err != nil {
		t.Fatalf("GetStatus returned error: %v", err)
	}
	if response.Status != "refund" {
		t.Fatalf("unexpected status: %s", response.Status)
	}
}
