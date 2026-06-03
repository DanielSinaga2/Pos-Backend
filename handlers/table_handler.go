package handlers

import (
	"errors"
	"strings"

	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type TableHandler struct {
	db *gorm.DB
}

type tableRequest struct {
	TableNumber string             `json:"table_number"`
	Status      models.TableStatus `json:"status"`
}

type tableStatusRequest struct {
	Status models.TableStatus `json:"status"`
}

func NewTableHandler(db *gorm.DB) *TableHandler {
	return &TableHandler{db: db}
}

func (h *TableHandler) List(c *fiber.Ctx) error {
	var tables []models.Table
	if err := h.db.Order("table_number ASC").Find(&tables).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get tables")
	}
	return utils.Success(c, fiber.StatusOK, "tables retrieved successfully", tables)
}

func (h *TableHandler) Get(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	table, err := h.find(id)
	if err != nil {
		return tableLookupError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "table retrieved successfully", table)
}

func (h *TableHandler) Create(c *fiber.Ctx) error {
	var request tableRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	request.TableNumber = strings.TrimSpace(request.TableNumber)
	if request.TableNumber == "" {
		return utils.Error(c, fiber.StatusBadRequest, "table_number is required")
	}
	if request.Status == "" {
		request.Status = models.TableAvailable
	}
	if !isValidTableStatus(request.Status) {
		return utils.Error(c, fiber.StatusBadRequest, "status must be available, occupied, or reserved")
	}

	table := models.Table{TableNumber: request.TableNumber, Status: request.Status}
	if err := h.db.Create(&table).Error; err != nil {
		if isDuplicateKey(err) {
			return utils.Error(c, fiber.StatusConflict, "table number is already registered")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to create table")
	}
	return utils.Success(c, fiber.StatusCreated, "table created successfully", table)
}

func (h *TableHandler) Update(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var request tableRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	request.TableNumber = strings.TrimSpace(request.TableNumber)
	if request.TableNumber == "" {
		return utils.Error(c, fiber.StatusBadRequest, "table_number is required")
	}
	if !isValidTableStatus(request.Status) {
		return utils.Error(c, fiber.StatusBadRequest, "status must be available, occupied, or reserved")
	}
	table, err := h.find(id)
	if err != nil {
		return tableLookupError(c, err)
	}
	table.TableNumber = request.TableNumber
	table.Status = request.Status
	if err := h.db.Save(&table).Error; err != nil {
		if isDuplicateKey(err) {
			return utils.Error(c, fiber.StatusConflict, "table number is already registered")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to update table")
	}
	return utils.Success(c, fiber.StatusOK, "table updated successfully", table)
}

func (h *TableHandler) Delete(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	table, err := h.find(id)
	if err != nil {
		return tableLookupError(c, err)
	}
	var qrCodeCount int64
	if err := h.db.Model(&models.QRCode{}).Where("table_id = ?", id).Count(&qrCodeCount).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to check table usage")
	}
	if qrCodeCount > 0 {
		return utils.Error(c, fiber.StatusConflict, "table cannot be deleted because it still has QR codes")
	}
	if err := h.db.Delete(&table).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to delete table")
	}
	return utils.Success(c, fiber.StatusOK, "table deleted successfully", table)
}

func (h *TableHandler) UpdateStatus(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var request tableStatusRequest
	if err := c.BodyParser(&request); err != nil || !isValidTableStatus(request.Status) {
		return utils.Error(c, fiber.StatusBadRequest, "status must be available, occupied, or reserved")
	}
	table, err := h.find(id)
	if err != nil {
		return tableLookupError(c, err)
	}
	table.Status = request.Status
	if err := h.db.Save(&table).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to update table status")
	}
	return utils.Success(c, fiber.StatusOK, "table status updated successfully", table)
}

func (h *TableHandler) find(id uint) (models.Table, error) {
	var table models.Table
	err := h.db.First(&table, id).Error
	return table, err
}

func isValidTableStatus(status models.TableStatus) bool {
	return status == models.TableAvailable || status == models.TableOccupied || status == models.TableReserved
}

func tableLookupError(c *fiber.Ctx, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.Error(c, fiber.StatusNotFound, "table not found")
	}
	return utils.Error(c, fiber.StatusInternalServerError, "failed to get table")
}
