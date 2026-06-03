package models

import "time"

type Menu struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	CategoryID  uint      `gorm:"not null;index" json:"category_id"`
	Category    Category  `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"category"`
	Name        string    `gorm:"size:150;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	Price       int64     `gorm:"not null;check:price >= 0" json:"price"`
	ImageURL    string    `gorm:"size:500" json:"image_url"`
	IsAvailable bool      `gorm:"not null;default:true" json:"is_available"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
