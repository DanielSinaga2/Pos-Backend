package models

import "time"

type OrderType string

const (
	OrderDineIn   OrderType = "dine_in"
	OrderTakeAway OrderType = "take_away"
	OrderCashier  OrderType = "cashier"
)

type OrderStatus string

const (
	OrderPendingPayment OrderStatus = "pending_payment"
	OrderPaid           OrderStatus = "paid"
	OrderSentToKitchen  OrderStatus = "sent_to_kitchen"
	OrderCooking        OrderStatus = "cooking"
	OrderReady          OrderStatus = "ready"
	OrderCompleted      OrderStatus = "completed"
	OrderCancelled      OrderStatus = "cancelled"
)

type Order struct {
	ID            uint        `gorm:"primaryKey" json:"id"`
	OrderCode     string      `gorm:"size:50;uniqueIndex;not null" json:"order_code"`
	OrderType     OrderType   `gorm:"type:varchar(20);not null;check:order_type IN ('dine_in','take_away','cashier')" json:"order_type"`
	TableID       *uint       `gorm:"index" json:"table_id"`
	Table         *Table      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"table,omitempty"`
	CustomerName  string      `gorm:"size:150" json:"customer_name,omitempty"`
	CustomerPhone string      `gorm:"size:30" json:"customer_phone,omitempty"`
	TotalAmount   int64       `gorm:"not null;check:total_amount >= 0" json:"total_amount"`
	Status        OrderStatus `gorm:"type:varchar(30);not null;index;check:status IN ('pending_payment','paid','sent_to_kitchen','cooking','ready','completed','cancelled')" json:"status"`
	CreatedBy     *uint       `gorm:"index" json:"created_by"`
	Creator       *User       `gorm:"foreignKey:CreatedBy;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"creator,omitempty"`
	Items         []OrderItem `gorm:"foreignKey:OrderID" json:"items,omitempty"`
	Payment       *Payment    `gorm:"foreignKey:OrderID" json:"payment,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}
