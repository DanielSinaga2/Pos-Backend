package handlers

import (
	"errors"

	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type PublicHandler struct {
	db *gorm.DB
}

func NewPublicHandler(db *gorm.DB) *PublicHandler {
	return &PublicHandler{db: db}
}

func (h *PublicHandler) ListMenu(c *fiber.Ctx) error {
	var menus []models.Menu
	if err := h.db.
		Preload("Category").
		Joins("JOIN categories ON categories.id = menus.category_id").
		Where("menus.is_available = ? AND categories.is_active = ?", true, true).
		Order("categories.name ASC, menus.name ASC").
		Find(&menus).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get public menu")
	}
	return utils.Success(c, fiber.StatusOK, "public menu retrieved successfully", menus)
}

func (h *PublicHandler) GetMenu(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var menu models.Menu
	err = h.db.
		Preload("Category").
		Joins("JOIN categories ON categories.id = menus.category_id").
		Where("menus.id = ? AND menus.is_available = ? AND categories.is_active = ?", id, true, true).
		First(&menu).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "menu not found")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get public menu")
	}
	return utils.Success(c, fiber.StatusOK, "public menu retrieved successfully", menu)
}

func (h *PublicHandler) GetQRCode(c *fiber.Ctx) error {
	code := c.Params("code")
	if code == "" {
		return utils.Error(c, fiber.StatusBadRequest, "QR code is required")
	}
	var qrCode models.QRCode
	if err := h.db.Preload("Table").Where("code = ? AND is_active = ?", code, true).First(&qrCode).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "QR code not found or inactive")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get QR code")
	}
	return utils.Success(c, fiber.StatusOK, "QR code retrieved successfully", qrCode)
}
