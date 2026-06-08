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
	TotalOrders         int64 `json:"total_orders"`
	TotalRevenue        int64 `json:"total_revenue"`
	TotalCash           int64 `json:"total_cash"`
	TotalQRISManual     int64 `json:"total_qris_manual"`
	TotalTransfer       int64 `json:"total_transfer"`
	WaitingPayment      int64 `json:"waiting_payment"`
	Ready               int64 `json:"ready"`
	WaitingPaymentCount int64 `json:"waiting_payment_count"`
	ReadyCount          int64 `json:"ready_count"`
}

type bestSellingMenu struct {
	MenuID        uint   `json:"menu_id"`
	MenuName      string `json:"menu_name"`
	TotalQuantity int64  `json:"total_quantity"`
	TotalRevenue  int64  `json:"total_revenue"`
}

type salesTrend struct {
	Date         string `json:"date"`
	TotalOrders  int64  `json:"total_orders"`
	TotalRevenue int64  `json:"total_revenue"`
}

type paymentMethodBreakdown struct {
	PaymentMethod string `json:"payment_method"`
	TotalOrders   int64  `json:"total_orders"`
	TotalRevenue  int64  `json:"total_revenue"`
}

type salesReportResponse struct {
	salesSummary
	SalesTrend             []salesTrend             `json:"sales_trend"`
	PaymentMethodBreakdown []paymentMethodBreakdown `json:"payment_method_breakdown"`
	BestSellingMenus       []bestSellingMenu        `json:"best_selling_menus"`
}

func NewReportHandler(db *gorm.DB) *ReportHandler {
	return &ReportHandler{db: db}
}

func (h *ReportHandler) Sales(c *fiber.Ctx) error {
	startDate, endDate, message := parseReportDateRange(c.Query("start_date"), c.Query("end_date"))
	if message != "" {
		return utils.Error(c, fiber.StatusBadRequest, message)
	}

	paidAtOrOrderCreatedAt := "COALESCE(payments.paid_at, orders.created_at)"

	paidPaymentsQuery := h.db.Table("payments").
		Joins("JOIN orders ON orders.id = payments.order_id").
		Where("payments.status = ?", models.PaymentPaid)
	if startDate != nil {
		paidPaymentsQuery = paidPaymentsQuery.Where(paidAtOrOrderCreatedAt+" >= ?", *startDate)
	}
	if endDate != nil {
		paidPaymentsQuery = paidPaymentsQuery.Where(paidAtOrOrderCreatedAt+" < ?", *endDate)
	}

	var summary salesSummary
	if err := paidPaymentsQuery.Select(`
		COUNT(DISTINCT orders.id) AS total_orders,
		COALESCE(SUM(payments.amount), 0) AS total_revenue,
		COALESCE(SUM(CASE WHEN payments.payment_method = 'cash' THEN payments.amount ELSE 0 END), 0) AS total_cash,
		COALESCE(SUM(CASE WHEN payments.payment_method = 'qris_manual' THEN payments.amount ELSE 0 END), 0) AS total_qris_manual,
		COALESCE(SUM(CASE WHEN payments.payment_method = 'transfer' THEN payments.amount ELSE 0 END), 0) AS total_transfer
	`).Scan(&summary).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to generate sales report")
	}

	waitingPaymentQuery := h.db.Table("orders").
		Joins("LEFT JOIN payments ON payments.order_id = orders.id").
		Where("(orders.status = ? OR payments.status = ?)", models.OrderPendingPayment, models.PaymentWaitingConfirmation)
	if startDate != nil {
		waitingPaymentQuery = waitingPaymentQuery.Where("orders.created_at >= ?", *startDate)
	}
	if endDate != nil {
		waitingPaymentQuery = waitingPaymentQuery.Where("orders.created_at < ?", *endDate)
	}
	if err := waitingPaymentQuery.Count(&summary.WaitingPayment).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to count waiting payment orders")
	}
	summary.WaitingPaymentCount = summary.WaitingPayment

	readyQuery := h.db.Model(&models.Order{}).Where("status = ?", models.OrderReady)
	if startDate != nil {
		readyQuery = readyQuery.Where("created_at >= ?", *startDate)
	}
	if endDate != nil {
		readyQuery = readyQuery.Where("created_at < ?", *endDate)
	}
	if err := readyQuery.Count(&summary.Ready).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to count ready orders")
	}
	summary.ReadyCount = summary.Ready

	salesTrendQuery := h.db.Table("payments").
		Select(`
			TO_CHAR(DATE_TRUNC('day', `+paidAtOrOrderCreatedAt+`), 'YYYY-MM-DD') AS date,
			COUNT(DISTINCT orders.id) AS total_orders,
			COALESCE(SUM(payments.amount), 0) AS total_revenue
		`).
		Joins("JOIN orders ON orders.id = payments.order_id").
		Where("payments.status = ?", models.PaymentPaid)
	if startDate != nil {
		salesTrendQuery = salesTrendQuery.Where(paidAtOrOrderCreatedAt+" >= ?", *startDate)
	}
	if endDate != nil {
		salesTrendQuery = salesTrendQuery.Where(paidAtOrOrderCreatedAt+" < ?", *endDate)
	}

	var salesTrendItems []salesTrend
	if err := salesTrendQuery.
		Group("DATE_TRUNC('day', " + paidAtOrOrderCreatedAt + ")").
		Order("DATE_TRUNC('day', " + paidAtOrOrderCreatedAt + ") ASC").
		Scan(&salesTrendItems).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to generate sales trend report")
	}

	paymentMethodQuery := h.db.Table("payments").
		Select(`
			payments.payment_method AS payment_method,
			COUNT(DISTINCT orders.id) AS total_orders,
			COALESCE(SUM(payments.amount), 0) AS total_revenue
		`).
		Joins("JOIN orders ON orders.id = payments.order_id").
		Where("payments.status = ?", models.PaymentPaid)
	if startDate != nil {
		paymentMethodQuery = paymentMethodQuery.Where(paidAtOrOrderCreatedAt+" >= ?", *startDate)
	}
	if endDate != nil {
		paymentMethodQuery = paymentMethodQuery.Where(paidAtOrOrderCreatedAt+" < ?", *endDate)
	}

	var paymentMethodBreakdownItems []paymentMethodBreakdown
	if err := paymentMethodQuery.
		Group("payments.payment_method").
		Order("payments.payment_method ASC").
		Scan(&paymentMethodBreakdownItems).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to generate payment method report")
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
		Where("payments.status = ?", models.PaymentPaid)
	if startDate != nil {
		bestSellingQuery = bestSellingQuery.Where(paidAtOrOrderCreatedAt+" >= ?", *startDate)
	}
	if endDate != nil {
		bestSellingQuery = bestSellingQuery.Where(paidAtOrOrderCreatedAt+" < ?", *endDate)
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
		salesSummary:           summary,
		SalesTrend:             salesTrendItems,
		PaymentMethodBreakdown: paymentMethodBreakdownItems,
		BestSellingMenus:       bestSellingMenus,
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
