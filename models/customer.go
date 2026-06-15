package models

import "time"

type Customer struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:150;not null" json:"name"`
	Phone     string    `gorm:"size:30;uniqueIndex;not null" json:"phone"`
	Email     string    `gorm:"size:150" json:"email,omitempty"`
	Orders    []Order   `gorm:"foreignKey:CustomerID" json:"orders,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
