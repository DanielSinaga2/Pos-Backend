package handlers

import (
	"errors"
	"strings"

	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type MenuHandler struct {
	db *gorm.DB
}

type menuRequest struct {
	CategoryID  uint   `json:"category_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       int64  `json:"price"`
	ImageURL    string `json:"image_url"`
	IsAvailable *bool  `json:"is_available"`
}

type menuAvailabilityRequest struct {
	IsAvailable *bool `json:"is_available"`
}

func NewMenuHandler(db *gorm.DB) *MenuHandler {
	return &MenuHandler{db: db}
}

func (h *MenuHandler) List(c *fiber.Ctx) error {
	var menus []models.Menu
	if err := preloadMenuOptions(h.db.Preload("Category")).Order("name ASC").Find(&menus).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get menus")
	}
	return utils.Success(c, fiber.StatusOK, "menus retrieved successfully", menus)
}

func (h *MenuHandler) Get(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	menu, err := h.find(id)
	if err != nil {
		return menuLookupError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "menu retrieved successfully", menu)
}

func (h *MenuHandler) ListByCategory(c *fiber.Ctx) error {
	categoryID, err := parseID(c, "category_id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	exists, err := recordExists[models.Category](h.db, categoryID)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to check category")
	}
	if !exists {
		return utils.Error(c, fiber.StatusNotFound, "category not found")
	}

	var menus []models.Menu
	if err := preloadMenuOptions(h.db.Preload("Category")).Where("category_id = ?", categoryID).Order("name ASC").Find(&menus).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get menus")
	}
	return utils.Success(c, fiber.StatusOK, "menus retrieved successfully", menus)
}

func (h *MenuHandler) Create(c *fiber.Ctx) error {
	var request menuRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	if message := validateMenuRequest(h.db, &request); message != "" {
		return utils.Error(c, fiber.StatusBadRequest, message)
	}

	menu := models.Menu{
		CategoryID:  request.CategoryID,
		Name:        strings.TrimSpace(request.Name),
		Description: strings.TrimSpace(request.Description),
		Price:       request.Price,
		ImageURL:    strings.TrimSpace(request.ImageURL),
		IsAvailable: true,
	}
	if request.IsAvailable != nil {
		menu.IsAvailable = *request.IsAvailable
	}
	if err := h.db.Create(&menu).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to create menu")
	}
	h.db.Preload("Category").First(&menu, menu.ID)
	return utils.Success(c, fiber.StatusCreated, "menu created successfully", menu)
}

func (h *MenuHandler) Update(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var request menuRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	if message := validateMenuRequest(h.db, &request); message != "" {
		return utils.Error(c, fiber.StatusBadRequest, message)
	}
	menu, err := h.find(id)
	if err != nil {
		return menuLookupError(c, err)
	}
	menu.CategoryID = request.CategoryID
	menu.Name = strings.TrimSpace(request.Name)
	menu.Description = strings.TrimSpace(request.Description)
	menu.Price = request.Price
	menu.ImageURL = strings.TrimSpace(request.ImageURL)
	if request.IsAvailable != nil {
		menu.IsAvailable = *request.IsAvailable
	}
	if err := h.db.Save(&menu).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to update menu")
	}
	h.db.Preload("Category").First(&menu, menu.ID)
	return utils.Success(c, fiber.StatusOK, "menu updated successfully", menu)
}

func (h *MenuHandler) Delete(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	menu, err := h.find(id)
	if err != nil {
		return menuLookupError(c, err)
	}
	if err := h.db.Delete(&menu).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to delete menu")
	}
	return utils.Success(c, fiber.StatusOK, "menu deleted successfully", menu)
}

func (h *MenuHandler) UpdateAvailability(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var request menuAvailabilityRequest
	if err := c.BodyParser(&request); err != nil || request.IsAvailable == nil {
		return utils.Error(c, fiber.StatusBadRequest, "is_available is required")
	}
	menu, err := h.find(id)
	if err != nil {
		return menuLookupError(c, err)
	}
	menu.IsAvailable = *request.IsAvailable
	if err := h.db.Save(&menu).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to update menu availability")
	}
	return utils.Success(c, fiber.StatusOK, "menu availability updated successfully", menu)
}

func (h *MenuHandler) find(id uint) (models.Menu, error) {
	var menu models.Menu
	err := preloadMenuOptions(h.db.Preload("Category")).First(&menu, id).Error
	return menu, err
}

func preloadMenuOptions(db *gorm.DB) *gorm.DB {
	return db.
		Preload("OptionGroups", func(db *gorm.DB) *gorm.DB {
			return db.Where("is_active = ?", true).Order("sort_order ASC, id ASC")
		}).
		Preload("OptionGroups.Options", func(db *gorm.DB) *gorm.DB {
			return db.Where("is_active = ?", true).Order("sort_order ASC, id ASC")
		})
}

func validateMenuRequest(db *gorm.DB, request *menuRequest) string {
	request.Name = strings.TrimSpace(request.Name)
	request.ImageURL = strings.TrimSpace(request.ImageURL)
	if request.CategoryID == 0 || request.Name == "" {
		return "category_id and name are required"
	}
	if request.Price < 0 {
		return "price cannot be negative"
	}
	if !isValidHTTPURL(request.ImageURL) {
		return "image_url must be a valid HTTP or HTTPS URL"
	}
	exists, err := recordExists[models.Category](db, request.CategoryID)
	if err != nil || !exists {
		return "category not found"
	}
	return ""
}

func menuLookupError(c *fiber.Ctx, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.Error(c, fiber.StatusNotFound, "menu not found")
	}
	return utils.Error(c, fiber.StatusInternalServerError, "failed to get menu")
}
