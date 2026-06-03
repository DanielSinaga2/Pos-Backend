package models

import "time"

type QRCodeType string

const (
	QRCodeDineIn   QRCodeType = "dine_in"
	QRCodeTakeAway QRCodeType = "take_away"
)

type QRCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Code      string     `gorm:"size:100;uniqueIndex;not null" json:"code"`
	Type      QRCodeType `gorm:"type:varchar(20);not null;check:type IN ('dine_in','take_away')" json:"type"`
	TableID   *uint      `gorm:"index" json:"table_id"`
	Table     *Table     `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"table,omitempty"`
	URL       string     `gorm:"size:500;not null" json:"url"`
	IsActive  bool       `gorm:"not null;default:true" json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}
