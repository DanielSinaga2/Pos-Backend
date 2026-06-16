package main

import (
	"errors"
	"log"
	"strings"

	"pos-backend/config"
	"pos-backend/database"
	"pos-backend/models"
	"pos-backend/routes"
	"pos-backend/seeders"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("initialize database: %v", err)
	}

	if err := db.AutoMigrate(
		&models.User{},
		&models.Category{},
		&models.Menu{},
		&models.MenuOptionGroup{},
		&models.MenuOption{},
		&models.Table{},
		&models.QRCode{},
		&models.Customer{},
		&models.Order{},
		&models.OrderItem{},
		&models.OrderItemOption{},
		&models.Payment{},
	); err != nil {
		log.Fatalf("migrate database: %v", err)
	}
	if err := database.EnsurePaymentMethodConstraint(db); err != nil {
		log.Fatalf("migrate payment method constraint: %v", err)
	}
	if err := database.EnsurePaymentStatusConstraint(db); err != nil {
		log.Fatalf("migrate payment status constraint: %v", err)
	}
	if err := database.EnsureTableStatusIsOptional(db); err != nil {
		log.Fatalf("migrate table status optional: %v", err)
	}

	if cfg.RunSeeder {
		if err := seeders.Run(db); err != nil {
			log.Fatalf("seed initial data: %v", err)
		}
		log.Print("initial data seeder completed")
	}

	app := fiber.New(fiber.Config{
		BodyLimit: int(utils.MaxUploadFileSize + 1024*1024),
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if fiberError, ok := err.(*fiber.Error); ok {
				code = fiberError.Code
			}
			if errors.Is(err, fiber.ErrRequestEntityTooLarge) {
				return utils.Error(c, code, "file tidak valid")
			}
			return utils.Error(c, code, err.Error())
		},
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CorsAllowedOrigins,
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,ngrok-skip-browser-warning",
		AllowCredentials: true,
		AllowOriginsFunc: func(origin string) bool {
			if origin == "" {
				return true
			}
			for _, allowedOrigin := range strings.Split(cfg.CorsAllowedOrigins, ",") {
				if strings.TrimSpace(allowedOrigin) == origin {
					return true
				}
			}
			return strings.HasPrefix(origin, "http://localhost:") ||
				strings.HasPrefix(origin, "http://127.0.0.1:") ||
				strings.HasSuffix(origin, ".ngrok-free.app")
		},
	}))

	app.Use(logger.New())
	app.Use(recover.New())
	app.Static("/uploads", "./uploads")

	routes.Setup(app, db, cfg)

	log.Printf("server is running on port %s", cfg.AppPort)
	log.Fatal(app.Listen(":" + cfg.AppPort))
}
