package models

import "time"

type Menu struct {
	ID           uint              `gorm:"primaryKey" json:"id"`
	CategoryID   uint              `gorm:"not null;index" json:"category_id"`
	Category     Category          `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"category"`
	Name         string            `gorm:"size:150;not null" json:"name"`
	Description  string            `gorm:"type:text" json:"description"`
	Price        int64             `gorm:"not null;check:price >= 0" json:"price"`
	ImageURL     string            `gorm:"size:500" json:"image_url"`
	IsAvailable  bool              `gorm:"not null;default:true" json:"is_available"`
	OptionGroups []MenuOptionGroup `gorm:"foreignKey:MenuID" json:"option_groups"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

func (menu *Menu) AfterFind() error {
	if menu.OptionGroups == nil {
		menu.OptionGroups = []MenuOptionGroup{}
	}
	return nil
}

type MenuOptionGroupType string

const (
	MenuOptionGroupSingle   MenuOptionGroupType = "single"
	MenuOptionGroupMultiple MenuOptionGroupType = "multiple"
)

type MenuOptionGroup struct {
	ID        uint                `gorm:"primaryKey" json:"id"`
	MenuID    uint                `gorm:"not null;index" json:"menu_id"`
	Menu      *Menu               `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"menu,omitempty"`
	Name      string              `gorm:"size:150;not null" json:"name"`
	Type      MenuOptionGroupType `gorm:"type:varchar(20);not null;default:'single';check:type IN ('single','multiple')" json:"type"`
	Required  bool                `gorm:"not null;default:false" json:"required"`
	MinSelect int                 `gorm:"not null;default:0" json:"min_select"`
	MaxSelect int                 `gorm:"not null;default:1" json:"max_select"`
	SortOrder int                 `gorm:"not null;default:0" json:"sort_order"`
	IsActive  bool                `gorm:"not null;default:true;index" json:"is_active"`
	Options   []MenuOption        `gorm:"foreignKey:GroupID" json:"options"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

func (group *MenuOptionGroup) AfterFind() error {
	if group.Options == nil {
		group.Options = []MenuOption{}
	}
	return nil
}

type MenuOption struct {
	ID              uint             `gorm:"primaryKey" json:"id"`
	GroupID         uint             `gorm:"not null;index" json:"group_id"`
	Group           *MenuOptionGroup `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"group,omitempty"`
	Name            string           `gorm:"size:150;not null" json:"name"`
	AdditionalPrice int64            `gorm:"not null;default:0;check:additional_price >= 0" json:"additional_price"`
	SortOrder       int              `gorm:"not null;default:0" json:"sort_order"`
	IsActive        bool             `gorm:"not null;default:true;index" json:"is_active"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}
