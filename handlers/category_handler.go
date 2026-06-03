package handlers

import (
	"errors"
	"strings"

	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type CategoryHandler struct {
	db *gorm.DB
}

type categoryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    *bool  `json:"is_active"`
}

func NewCategoryHandler(db *gorm.DB) *CategoryHandler {
	return &CategoryHandler{db: db}
}

func (h *CategoryHandler) List(c *fiber.Ctx) error {
	var categories []models.Category
	if err := h.db.Order("name ASC").Find(&categories).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get categories")
	}
	return utils.Success(c, fiber.StatusOK, "categories retrieved successfully", categories)
}

func (h *CategoryHandler) Get(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	var category models.Category
	if err := h.db.First(&category, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "category not found")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get category")
	}
	return utils.Success(c, fiber.StatusOK, "category retrieved successfully", category)
}

func (h *CategoryHandler) Create(c *fiber.Ctx) error {
	var request categoryRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		return utils.Error(c, fiber.StatusBadRequest, "name is required")
	}

	category := models.Category{Name: request.Name, Description: strings.TrimSpace(request.Description), IsActive: true}
	if request.IsActive != nil {
		category.IsActive = *request.IsActive
	}
	if err := h.db.Create(&category).Error; err != nil {
		if isDuplicateKey(err) {
			return utils.Error(c, fiber.StatusConflict, "category name is already registered")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to create category")
	}
	return utils.Success(c, fiber.StatusCreated, "category created successfully", category)
}

func (h *CategoryHandler) Update(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var request categoryRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		return utils.Error(c, fiber.StatusBadRequest, "name is required")
	}

	var category models.Category
	if err := h.db.First(&category, id).Error; err != nil {
		return categoryLookupError(c, err)
	}
	category.Name = request.Name
	category.Description = strings.TrimSpace(request.Description)
	if request.IsActive != nil {
		category.IsActive = *request.IsActive
	}
	if err := h.db.Save(&category).Error; err != nil {
		if isDuplicateKey(err) {
			return utils.Error(c, fiber.StatusConflict, "category name is already registered")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to update category")
	}
	return utils.Success(c, fiber.StatusOK, "category updated successfully", category)
}

func (h *CategoryHandler) Delete(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var category models.Category
	if err := h.db.First(&category, id).Error; err != nil {
		return categoryLookupError(c, err)
	}
	var menuCount int64
	if err := h.db.Model(&models.Menu{}).Where("category_id = ?", id).Count(&menuCount).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to check category usage")
	}
	if menuCount > 0 {
		return utils.Error(c, fiber.StatusConflict, "category cannot be deleted because it still has menus")
	}
	if err := h.db.Delete(&category).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to delete category")
	}
	return utils.Success(c, fiber.StatusOK, "category deleted successfully", category)
}

func categoryLookupError(c *fiber.Ctx, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.Error(c, fiber.StatusNotFound, "category not found")
	}
	return utils.Error(c, fiber.StatusInternalServerError, "failed to get category")
}
