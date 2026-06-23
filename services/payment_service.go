package services

import (
	"errors"
	"fmt"
	"strings"

	"pos-backend/dto"

	"github.com/midtrans/midtrans-go"
	"github.com/midtrans/midtrans-go/coreapi"
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
}

func NewPaymentService(client *coreapi.Client) *PaymentService {
	if client == nil {
		return &PaymentService{}
	}

	return &PaymentService{
		gateway:    &midtransCoreGateway{client: client},
		configured: strings.TrimSpace(client.ServerKey) != "",
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

	if !s.configured || s.gateway == nil {
		return dto.CorePaymentStatusResponse{}, ErrMidtransNotConfig
	}

	transaction, err := s.gateway.Status(orderID)
	if err != nil {
		return dto.CorePaymentStatusResponse{}, fmt.Errorf(
			"get Midtrans transaction status: %w",
			err,
		)
	}

	return dto.CorePaymentStatusResponse{
		Status: transaction.TransactionStatus,
	}, nil
}
