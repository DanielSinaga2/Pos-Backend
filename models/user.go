package models

import "time"

type Role string

const (
	RoleAdmin   Role = "admin"
	RoleCashier Role = "cashier"
	RoleKitchen Role = "kitchen"
)

type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	Email     string    `gorm:"size:150;uniqueIndex;not null" json:"email"`
	Password  string    `gorm:"size:255;not null" json:"-"`
	Role      Role      `gorm:"type:varchar(20);not null;default:'cashier';check:role IN ('admin','cashier','kitchen')" json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
