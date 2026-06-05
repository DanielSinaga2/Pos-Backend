package routes

import (
	"pos-backend/config"
	"pos-backend/handlers"
	"pos-backend/middleware"
	"pos-backend/models"
	"pos-backend/services"
	"pos-backend/utils"
	realtime "pos-backend/websocket"

	"github.com/gofiber/fiber/v2"
	fiberws "github.com/gofiber/websocket/v2"
	"gorm.io/gorm"
)

func Setup(app *fiber.App, db *gorm.DB, cfg *config.Config) {
	api := app.Group("/api")

	api.Get("/health", func(c *fiber.Ctx) error {
		return utils.Success(c, fiber.StatusOK, "service is healthy", fiber.Map{
			"status": "ok",
		})
	})

	authHandler := handlers.NewAuthHandler(db, cfg)
	auth := api.Group("/auth")
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login)
	auth.Get("/me", middleware.JWTAuth(cfg.JWTSecret), authHandler.Me)

	jwtAuth := middleware.JWTAuth(cfg.JWTSecret)
	adminOnly := middleware.AllowRoles(models.RoleAdmin)

	userHandler := handlers.NewUserHandler(db)
	users := api.Group("/users", jwtAuth, adminOnly)
	users.Get("/", userHandler.List)
	users.Get("/:id", userHandler.Get)
	users.Post("/", userHandler.Create)
	users.Put("/:id", userHandler.Update)
	users.Delete("/:id", userHandler.Delete)

	categoryHandler := handlers.NewCategoryHandler(db)
	categories := api.Group("/categories", jwtAuth)
	categories.Get("/", middleware.AllowRoles(models.RoleAdmin, models.RoleCashier, models.RoleKitchen), categoryHandler.List)
	categories.Get("/:id", middleware.AllowRoles(models.RoleAdmin, models.RoleCashier, models.RoleKitchen), categoryHandler.Get)
	categories.Post("/", adminOnly, categoryHandler.Create)
	categories.Put("/:id", adminOnly, categoryHandler.Update)
	categories.Delete("/:id", adminOnly, categoryHandler.Delete)

	menuHandler := handlers.NewMenuHandler(db)
	menus := api.Group("/menus", jwtAuth)
	menus.Get("/", middleware.AllowRoles(models.RoleAdmin, models.RoleCashier, models.RoleKitchen), menuHandler.List)
	menus.Get("/category/:category_id", middleware.AllowRoles(models.RoleAdmin, models.RoleCashier, models.RoleKitchen), menuHandler.ListByCategory)
	menus.Get("/:id", middleware.AllowRoles(models.RoleAdmin, models.RoleCashier, models.RoleKitchen), menuHandler.Get)
	menus.Post("/", adminOnly, menuHandler.Create)
	menus.Put("/:id", adminOnly, menuHandler.Update)
	menus.Delete("/:id", adminOnly, menuHandler.Delete)
	menus.Patch("/:id/availability", adminOnly, menuHandler.UpdateAvailability)

	tableHandler := handlers.NewTableHandler(db)
	tables := api.Group("/tables", jwtAuth)
	tables.Get("/", middleware.AllowRoles(models.RoleAdmin, models.RoleCashier), tableHandler.List)
	tables.Get("/:id", middleware.AllowRoles(models.RoleAdmin, models.RoleCashier), tableHandler.Get)
	tables.Post("/", adminOnly, tableHandler.Create)
	tables.Put("/:id", adminOnly, tableHandler.Update)
	tables.Delete("/:id", adminOnly, tableHandler.Delete)
	tables.Patch("/:id/status", adminOnly, tableHandler.UpdateStatus)

	qrCodeHandler := handlers.NewQRCodeHandler(db)
	qrCodes := api.Group("/qrcodes", jwtAuth, adminOnly)
	qrCodes.Get("/", qrCodeHandler.List)
	qrCodes.Get("/:id", qrCodeHandler.Get)
	qrCodes.Post("/generate-table/:table_id", qrCodeHandler.GenerateTable)
	qrCodes.Post("/generate-takeaway", qrCodeHandler.GenerateTakeaway)
	qrCodes.Patch("/:id/active", qrCodeHandler.UpdateActive)

	publicHandler := handlers.NewPublicHandler(db)
	public := api.Group("/public")
	public.Get("/menu", publicHandler.ListMenu)
	public.Get("/menu/:id", publicHandler.GetMenu)
	public.Get("/qrcode/:code", publicHandler.GetQRCode)

	publicOrderHandler := handlers.NewPublicOrderHandler(db)
	public.Post("/orders", publicOrderHandler.Create)
	public.Get("/orders/:order_code", publicOrderHandler.Get)
	public.Post("/orders/:order_code/upload-payment-proof", publicOrderHandler.UploadPaymentProof)
	public.Post("/orders/:order_code/upload-payment-proof-file", publicOrderHandler.UploadPaymentProofFile)

	cashierHandler := handlers.NewCashierHandler(db)
	cashier := api.Group("/cashier", jwtAuth, middleware.AllowRoles(models.RoleCashier, models.RoleAdmin))
	cashier.Post("/orders", cashierHandler.CreateOrder)
	cashier.Get("/orders", cashierHandler.ListOrders)
	cashier.Get("/orders/:id", cashierHandler.GetOrder)
	cashier.Patch("/orders/:id/confirm-payment", cashierHandler.ConfirmPayment)
	cashier.Patch("/orders/:id/cancel", cashierHandler.CancelOrder)
	cashier.Patch("/orders/:id/complete", cashierHandler.CompleteOrder)
	cashier.Get("/payments/waiting-confirmation", cashierHandler.ListWaitingPayments)

	midtransHandler := handlers.NewMidtransPaymentHandler(db, services.NewMidtransService(cfg))
	payments := api.Group("/payments")
	payments.Post("/midtrans/notification", midtransHandler.Notification)
	payments.Post("/midtrans/create-snap/:order_id", jwtAuth, middleware.AllowRoles(models.RoleCashier, models.RoleAdmin), midtransHandler.CreateSnap)
	payments.Post("/midtrans/sync/:order_code", jwtAuth, middleware.AllowRoles(models.RoleCashier, models.RoleAdmin), midtransHandler.SyncStatus)
	payments.Get("/midtrans/status/:order_code", jwtAuth, middleware.AllowRoles(models.RoleCashier, models.RoleAdmin), midtransHandler.Status)

	kitchenHandler := handlers.NewKitchenHandler(db)
	kitchen := api.Group("/kitchen", jwtAuth, middleware.AllowRoles(models.RoleKitchen, models.RoleAdmin))
	kitchen.Get("/orders", kitchenHandler.ListOrders)
	kitchen.Patch("/orders/:id/cooking", kitchenHandler.StartCooking)
	kitchen.Patch("/orders/:id/ready", kitchenHandler.MarkReady)

	reportHandler := handlers.NewReportHandler(db)
	admin := api.Group("/admin", jwtAuth, adminOnly)
	admin.Get("/reports/sales", reportHandler.Sales)
	admin.Post("/menus/:id/upload-image", menuHandler.UploadImage)

	webSocketAuth := middleware.WebSocketJWTAuth(cfg.JWTSecret)
	webSocketUpgrade := func(c *fiber.Ctx) error {
		if !fiberws.IsWebSocketUpgrade(c) {
			return utils.Error(c, fiber.StatusUpgradeRequired, "websocket upgrade is required")
		}
		return c.Next()
	}
	app.Get("/ws/cashier",
		webSocketAuth,
		middleware.AllowRoles(models.RoleCashier, models.RoleAdmin),
		webSocketUpgrade,
		fiberws.New(realtime.NewClientHandler(realtime.CashierChannel)),
	)
	app.Get("/ws/kitchen",
		webSocketAuth,
		middleware.AllowRoles(models.RoleKitchen, models.RoleAdmin),
		webSocketUpgrade,
		fiberws.New(realtime.NewClientHandler(realtime.KitchenChannel)),
	)
	app.Get("/ws/customer/:order_code",
		func(c *fiber.Ctx) error {
			var count int64
			if err := db.Model(&models.Order{}).Where("order_code = ?", c.Params("order_code")).Count(&count).Error; err != nil {
				return utils.Error(c, fiber.StatusInternalServerError, "failed to validate order")
			}
			if count == 0 {
				return utils.Error(c, fiber.StatusNotFound, "order not found")
			}
			return c.Next()
		},
		webSocketUpgrade,
		fiberws.New(realtime.NewCustomerClientHandler()),
	)
}
