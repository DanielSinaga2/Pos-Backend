package models

import "time"

type PaymentMethod string

const (
	PaymentCash       PaymentMethod = "cash"
	PaymentQRISManual PaymentMethod = "qris_manual"
	PaymentTransfer   PaymentMethod = "transfer"
)

type PaymentStatus string

const (
	PaymentUnpaid              PaymentStatus = "unpaid"
	PaymentWaitingConfirmation PaymentStatus = "waiting_confirmation"
	PaymentPaid                PaymentStatus = "paid"
	PaymentRejected            PaymentStatus = "rejected"
)

type Payment struct {
	ID            uint          `gorm:"primaryKey" json:"id"`
	OrderID       uint          `gorm:"not null;uniqueIndex" json:"order_id"`
	Order         *Order        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"order,omitempty"`
	PaymentMethod PaymentMethod `gorm:"type:varchar(30);not null;check:payment_method IN ('cash','qris_manual','transfer')" json:"payment_method"`
	Amount        int64         `gorm:"not null;check:amount >= 0" json:"amount"`
	Status        PaymentStatus `gorm:"type:varchar(30);not null;index;check:status IN ('unpaid','waiting_confirmation','paid','rejected')" json:"status"`
	ProofImageURL string        `gorm:"size:500" json:"proof_image_url,omitempty"`
	PaidAt        *time.Time    `json:"paid_at,omitempty"`
	ConfirmedBy   *uint         `gorm:"index" json:"confirmed_by"`
	Confirmer     *User         `gorm:"foreignKey:ConfirmedBy;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"confirmer,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}
