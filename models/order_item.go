package models

import "time"

type OrderItem struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrderID   uint      `gorm:"not null;index" json:"order_id"`
	MenuID    uint      `gorm:"not null;index" json:"menu_id"`
	Menu      Menu      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"menu"`
	Quantity  int       `gorm:"not null;check:quantity > 0" json:"quantity"`
	Price     int64     `gorm:"not null;check:price >= 0" json:"price"`
	Subtotal  int64     `gorm:"not null;check:subtotal >= 0" json:"subtotal"`
	Note      string    `gorm:"type:text" json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
