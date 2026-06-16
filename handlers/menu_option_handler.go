package handlers

import (
	"errors"
	"strings"

	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type MenuOptionHandler struct {
	db *gorm.DB
}

type menuOptionGroupRequest struct {
	Name      string                     `json:"name"`
	Type      models.MenuOptionGroupType `json:"type"`
	Required  bool                       `json:"required"`
	MinSelect int                        `json:"min_select"`
	MaxSelect int                        `json:"max_select"`
	SortOrder int                        `json:"sort_order"`
	IsActive  *bool                      `json:"is_active"`
}

type menuOptionRequest struct {
	Name            string `json:"name"`
	AdditionalPrice int64  `json:"additional_price"`
	SortOrder       int    `json:"sort_order"`
	IsActive        *bool  `json:"is_active"`
}

func NewMenuOptionHandler(db *gorm.DB) *MenuOptionHandler {
	return &MenuOptionHandler{db: db}
}

func (h *MenuOptionHandler) ListByMenu(c *fiber.Ctx) error {
	menuID, err := parseID(c, "menu_id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	if exists, err := recordExists[models.Menu](h.db, menuID); err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to check menu")
	} else if !exists {
		return utils.Error(c, fiber.StatusNotFound, "menu not found")
	}

	var groups []models.MenuOptionGroup
	if err := h.db.
		Preload("Options", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order ASC, id ASC")
		}).
		Where("menu_id = ?", menuID).
		Order("sort_order ASC, id ASC").
		Find(&groups).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get menu options")
	}

	return utils.Success(c, fiber.StatusOK, "menu options retrieved successfully", fiber.Map{
		"menu_id": menuID,
		"groups":  groups,
	})
}

func (h *MenuOptionHandler) CreateGroup(c *fiber.Ctx) error {
	menuID, err := parseID(c, "menu_id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	if exists, err := recordExists[models.Menu](h.db, menuID); err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to check menu")
	} else if !exists {
		return utils.Error(c, fiber.StatusNotFound, "menu not found")
	}

	var request menuOptionGroupRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	if message := normalizeAndValidateOptionGroupRequest(&request); message != "" {
		return utils.Error(c, fiber.StatusBadRequest, message)
	}

	group := models.MenuOptionGroup{
		MenuID:    menuID,
		Name:      request.Name,
		Type:      request.Type,
		Required:  request.Required,
		MinSelect: request.MinSelect,
		MaxSelect: request.MaxSelect,
		SortOrder: request.SortOrder,
		IsActive:  true,
	}
	if request.IsActive != nil {
		group.IsActive = *request.IsActive
	}
	if err := h.db.Create(&group).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to create option group")
	}
	return utils.Success(c, fiber.StatusCreated, "option group created successfully", group)
}

func (h *MenuOptionHandler) UpdateGroup(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	var request menuOptionGroupRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	if message := normalizeAndValidateOptionGroupRequest(&request); message != "" {
		return utils.Error(c, fiber.StatusBadRequest, message)
	}

	var group models.MenuOptionGroup
	if err := h.db.First(&group, groupID).Error; err != nil {
		return menuOptionGroupLookupError(c, err)
	}
	group.Name = request.Name
	group.Type = request.Type
	group.Required = request.Required
	group.MinSelect = request.MinSelect
	group.MaxSelect = request.MaxSelect
	group.SortOrder = request.SortOrder
	if request.IsActive != nil {
		group.IsActive = *request.IsActive
	}
	if err := h.db.Save(&group).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to update option group")
	}
	return utils.Success(c, fiber.StatusOK, "option group updated successfully", group)
}

func (h *MenuOptionHandler) DeleteGroup(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var group models.MenuOptionGroup
	if err := h.db.First(&group, groupID).Error; err != nil {
		return menuOptionGroupLookupError(c, err)
	}
	if err := h.db.Model(&group).Update("is_active", false).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to delete option group")
	}
	group.IsActive = false
	return utils.Success(c, fiber.StatusOK, "option group deleted successfully", group)
}

func (h *MenuOptionHandler) CreateOption(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var group models.MenuOptionGroup
	if err := h.db.First(&group, groupID).Error; err != nil {
		return menuOptionGroupLookupError(c, err)
	}

	var request menuOptionRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	if message := normalizeAndValidateOptionRequest(&request); message != "" {
		return utils.Error(c, fiber.StatusBadRequest, message)
	}

	option := models.MenuOption{
		GroupID:         group.ID,
		Name:            request.Name,
		AdditionalPrice: request.AdditionalPrice,
		SortOrder:       request.SortOrder,
		IsActive:        true,
	}
	if request.IsActive != nil {
		option.IsActive = *request.IsActive
	}
	if err := h.db.Create(&option).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to create option")
	}
	return utils.Success(c, fiber.StatusCreated, "option created successfully", option)
}

func (h *MenuOptionHandler) UpdateOption(c *fiber.Ctx) error {
	optionID, err := parseID(c, "option_id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	var request menuOptionRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	if message := normalizeAndValidateOptionRequest(&request); message != "" {
		return utils.Error(c, fiber.StatusBadRequest, message)
	}

	var option models.MenuOption
	if err := h.db.First(&option, optionID).Error; err != nil {
		return menuOptionLookupError(c, err)
	}
	option.Name = request.Name
	option.AdditionalPrice = request.AdditionalPrice
	option.SortOrder = request.SortOrder
	if request.IsActive != nil {
		option.IsActive = *request.IsActive
	}
	if err := h.db.Save(&option).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to update option")
	}
	return utils.Success(c, fiber.StatusOK, "option updated successfully", option)
}

func (h *MenuOptionHandler) DeleteOption(c *fiber.Ctx) error {
	optionID, err := parseID(c, "option_id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var option models.MenuOption
	if err := h.db.First(&option, optionID).Error; err != nil {
		return menuOptionLookupError(c, err)
	}
	if err := h.db.Model(&option).Update("is_active", false).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to delete option")
	}
	option.IsActive = false
	return utils.Success(c, fiber.StatusOK, "option deleted successfully", option)
}

func normalizeAndValidateOptionGroupRequest(request *menuOptionGroupRequest) string {
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		return "name is required"
	}
	if request.Type == "" {
		request.Type = models.MenuOptionGroupSingle
	}
	if request.Type != models.MenuOptionGroupSingle && request.Type != models.MenuOptionGroupMultiple {
		return "type must be single or multiple"
	}
	if request.MinSelect < 0 {
		return "min_select cannot be negative"
	}
	if request.MaxSelect < 1 {
		return "max_select must be at least 1"
	}
	if request.Type == models.MenuOptionGroupSingle && request.MaxSelect != 1 {
		return "max_select must be 1 for single option group"
	}
	if request.Required && request.MinSelect < 1 {
		return "min_select must be at least 1 when required is true"
	}
	if request.MinSelect > request.MaxSelect {
		return "min_select cannot exceed max_select"
	}
	return ""
}

func normalizeAndValidateOptionRequest(request *menuOptionRequest) string {
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		return "name is required"
	}
	if request.AdditionalPrice < 0 {
		return "additional_price cannot be negative"
	}
	return ""
}

func menuOptionGroupLookupError(c *fiber.Ctx, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.Error(c, fiber.StatusNotFound, "option group not found")
	}
	return utils.Error(c, fiber.StatusInternalServerError, "failed to get option group")
}

func menuOptionLookupError(c *fiber.Ctx, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.Error(c, fiber.StatusNotFound, "option not found")
	}
	return utils.Error(c, fiber.StatusInternalServerError, "failed to get option")
}
