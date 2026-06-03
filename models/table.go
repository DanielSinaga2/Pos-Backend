package models

import "time"

type TableStatus string

const (
	TableAvailable TableStatus = "available"
	TableOccupied  TableStatus = "occupied"
	TableReserved  TableStatus = "reserved"
)

type Table struct {
	ID          uint        `gorm:"primaryKey" json:"id"`
	TableNumber string      `gorm:"size:50;uniqueIndex;not null" json:"table_number"`
	QRCode      string      `gorm:"size:100;index" json:"qr_code"`
	Status      TableStatus `gorm:"type:varchar(20);not null;default:'available';check:status IN ('available','occupied','reserved')" json:"status"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

func (Table) TableName() string {
	return "restaurant_tables"
}
