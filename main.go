package main

import (
	"errors"
	"log"

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
		&models.Table{},
		&models.QRCode{},
		&models.Order{},
		&models.OrderItem{},
		&models.Payment{},
	); err != nil {
		log.Fatalf("migrate database: %v", err)
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

	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.FrontendURL,
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
	}))
	app.Static("/uploads", "./uploads")

	routes.Setup(app, db, cfg)

	log.Printf("server is running on port %s", cfg.AppPort)
	log.Fatal(app.Listen(":" + cfg.AppPort))
}
