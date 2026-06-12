package models

import (
	"encoding/json"
	"time"
)

type Table struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	TableNumber string    `gorm:"size:50;uniqueIndex;not null" json:"table_number"`
	QRCode      string    `gorm:"size:100;index" json:"qr_code"`
	Status      string    `gorm:"type:varchar(20);default:null" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Table) TableName() string {
	return "restaurant_tables"
}

func (t Table) MarshalJSON() ([]byte, error) {
	type tableJSON struct {
		ID          uint      `json:"id"`
		TableNumber string    `json:"table_number"`
		Number      string    `json:"number"`
		Name        string    `json:"name"`
		QRCode      string    `json:"qr_code"`
		Status      string    `json:"status"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
	}

	return json.Marshal(tableJSON{
		ID:          t.ID,
		TableNumber: t.TableNumber,
		Number:      t.TableNumber,
		Name:        t.TableNumber,
		QRCode:      t.QRCode,
		Status:      t.Status,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	})
}
