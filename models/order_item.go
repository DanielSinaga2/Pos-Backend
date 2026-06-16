package models

import "time"

type OrderItem struct {
	ID        uint              `gorm:"primaryKey" json:"id"`
	OrderID   uint              `gorm:"not null;index" json:"order_id"`
	MenuID    uint              `gorm:"not null;index" json:"menu_id"`
	Menu      Menu              `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"menu"`
	Quantity  int               `gorm:"not null;check:quantity > 0" json:"quantity"`
	Price     int64             `gorm:"not null;check:price >= 0" json:"price"`
	UnitPrice int64             `gorm:"not null;default:0;check:unit_price >= 0" json:"unit_price"`
	Subtotal  int64             `gorm:"not null;check:subtotal >= 0" json:"subtotal"`
	Note      string            `gorm:"type:text" json:"note,omitempty"`
	Options   []OrderItemOption `gorm:"foreignKey:OrderItemID" json:"options"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type OrderItemOption struct {
	ID                uint             `gorm:"primaryKey" json:"id"`
	OrderItemID       uint             `gorm:"not null;index" json:"order_item_id"`
	OrderItem         *OrderItem       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"order_item,omitempty"`
	MenuOptionGroupID *uint            `gorm:"index" json:"menu_option_group_id,omitempty"`
	MenuOptionGroup   *MenuOptionGroup `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"menu_option_group,omitempty"`
	MenuOptionID      *uint            `gorm:"index" json:"menu_option_id,omitempty"`
	MenuOption        *MenuOption      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"menu_option,omitempty"`
	GroupName         string           `gorm:"size:150;not null" json:"group_name"`
	OptionName        string           `gorm:"size:150;not null" json:"option_name"`
	AdditionalPrice   int64            `gorm:"not null;default:0;check:additional_price >= 0" json:"additional_price"`
	CreatedAt         time.Time        `json:"created_at"`
}

func (item *OrderItem) AfterFind() error {
	if item.UnitPrice == 0 {
		item.UnitPrice = item.Price
	}
	if item.Options == nil {
		item.Options = []OrderItemOption{}
	}
	return nil
}
