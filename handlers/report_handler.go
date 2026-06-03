package handlers

import (
	"time"

	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type ReportHandler struct {
	db *gorm.DB
}

type salesSummary struct {
	TotalOrders     int64 `json:"total_orders"`
	TotalRevenue    int64 `json:"total_revenue"`
	TotalCash       int64 `json:"total_cash"`
	TotalQRISManual int64 `json:"total_qris_manual"`
	TotalTransfer   int64 `json:"total_transfer"`
}

type bestSellingMenu struct {
	MenuID        uint   `json:"menu_id"`
	MenuName      string `json:"menu_name"`
	TotalQuantity int64  `json:"total_quantity"`
	TotalRevenue  int64  `json:"total_revenue"`
}

type salesReportResponse struct {
	salesSummary
	BestSellingMenus []bestSellingMenu `json:"best_selling_menus"`
}

func NewReportHandler(db *gorm.DB) *ReportHandler {
	return &ReportHandler{db: db}
}

func (h *ReportHandler) Sales(c *fiber.Ctx) error {
	startDate, endDate, message := parseReportDateRange(c.Query("start_date"), c.Query("end_date"))
	if message != "" {
		return utils.Error(c, fiber.StatusBadRequest, message)
	}

	base := h.db.Table("payments").
		Joins("JOIN orders ON orders.id = payments.order_id").
		Where("payments.status = ? AND orders.status <> ?", models.PaymentPaid, models.OrderCancelled)
	if startDate != nil {
		base = base.Where("payments.paid_at >= ?", *startDate)
	}
	if endDate != nil {
		base = base.Where("payments.paid_at < ?", *endDate)
	}

	var summary salesSummary
	if err := base.Select(`
		COUNT(DISTINCT orders.id) AS total_orders,
		COALESCE(SUM(payments.amount), 0) AS total_revenue,
		COALESCE(SUM(CASE WHEN payments.payment_method = 'cash' THEN payments.amount ELSE 0 END), 0) AS total_cash,
		COALESCE(SUM(CASE WHEN payments.payment_method = 'qris_manual' THEN payments.amount ELSE 0 END), 0) AS total_qris_manual,
		COALESCE(SUM(CASE WHEN payments.payment_method = 'transfer' THEN payments.amount ELSE 0 END), 0) AS total_transfer
	`).Scan(&summary).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to generate sales report")
	}

	bestSellingQuery := h.db.Table("order_items").
		Select(`
			order_items.menu_id AS menu_id,
			menus.name AS menu_name,
			COALESCE(SUM(order_items.quantity), 0) AS total_quantity,
			COALESCE(SUM(order_items.subtotal), 0) AS total_revenue
		`).
		Joins("JOIN menus ON menus.id = order_items.menu_id").
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Joins("JOIN payments ON payments.order_id = orders.id").
		Where("payments.status = ? AND orders.status <> ?", models.PaymentPaid, models.OrderCancelled)
	if startDate != nil {
		bestSellingQuery = bestSellingQuery.Where("payments.paid_at >= ?", *startDate)
	}
	if endDate != nil {
		bestSellingQuery = bestSellingQuery.Where("payments.paid_at < ?", *endDate)
	}

	var bestSellingMenus []bestSellingMenu
	if err := bestSellingQuery.
		Group("order_items.menu_id, menus.name").
		Order("total_quantity DESC, menus.name ASC").
		Limit(10).
		Scan(&bestSellingMenus).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to generate best selling menu report")
	}
	return utils.Success(c, fiber.StatusOK, "sales report retrieved successfully", salesReportResponse{
		salesSummary:     summary,
		BestSellingMenus: bestSellingMenus,
	})
}

func parseReportDateRange(startValue, endValue string) (*time.Time, *time.Time, string) {
	var startDate *time.Time
	var endDate *time.Time
	if startValue != "" {
		parsed, err := time.ParseInLocation("2006-01-02", startValue, jakartaLocation)
		if err != nil {
			return nil, nil, "start_date must use YYYY-MM-DD format"
		}
		startDate = &parsed
	}
	if endValue != "" {
		parsed, err := time.ParseInLocation("2006-01-02", endValue, jakartaLocation)
		if err != nil {
			return nil, nil, "end_date must use YYYY-MM-DD format"
		}
		parsed = parsed.AddDate(0, 0, 1)
		endDate = &parsed
	}
	if startDate != nil && endDate != nil && !startDate.Before(*endDate) {
		return nil, nil, "start_date cannot be after end_date"
	}
	return startDate, endDate, ""
}
