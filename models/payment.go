package models

import "time"

type PaymentMethod string

const (
	PaymentCash       PaymentMethod = "cash"
	PaymentQRIS       PaymentMethod = "qris"
	PaymentQRISManual PaymentMethod = "qris_manual"
	PaymentTransfer   PaymentMethod = "transfer"
)

type PaymentStatus string

const (
	PaymentUnpaid              PaymentStatus = "unpaid"
	PaymentWaitingConfirmation PaymentStatus = "waiting_confirmation"
	PaymentPending             PaymentStatus = "pending"
	PaymentPaid                PaymentStatus = "paid"
	PaymentRejected            PaymentStatus = "rejected"
	PaymentFailed              PaymentStatus = "failed"
	PaymentCancelled           PaymentStatus = "cancelled"
)

type Payment struct {
	ID              uint          `gorm:"primaryKey" json:"id"`
	OrderID         uint          `gorm:"not null;uniqueIndex" json:"order_id"`
	Order           *Order        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"order,omitempty"`
	PaymentMethod   PaymentMethod `gorm:"type:varchar(30);not null;check:payment_method IN ('cash','qris','qris_manual','transfer')" json:"payment_method"`
	Amount          int64         `gorm:"not null;check:amount >= 0" json:"amount"`
	Status          PaymentStatus `gorm:"type:varchar(30);not null;index;check:status IN ('unpaid','waiting_confirmation','pending','paid','rejected','failed','cancelled')" json:"status"`
	ProofImageURL   string        `gorm:"size:500" json:"proof_image_url,omitempty"`
	MidtransOrderID *string       `gorm:"size:100;index" json:"midtrans_order_id,omitempty"`
	SnapToken       *string       `gorm:"size:255" json:"snap_token,omitempty"`
	SnapRedirectURL *string       `gorm:"size:500" json:"snap_redirect_url,omitempty"`
	TransactionID   *string       `gorm:"size:100;index" json:"transaction_id,omitempty"`
	FraudStatus     *string       `gorm:"size:50" json:"fraud_status,omitempty"`
	PaymentType     *string       `gorm:"size:50" json:"payment_type,omitempty"`
	PaidAt          *time.Time    `json:"paid_at,omitempty"`
	ConfirmedBy     *uint         `gorm:"index" json:"confirmed_by"`
	Confirmer       *User         `gorm:"foreignKey:ConfirmedBy;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"confirmer,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}
