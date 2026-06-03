package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type QRCodeHandler struct {
	db *gorm.DB
}

type qrCodeActiveRequest struct {
	IsActive *bool `json:"is_active"`
}

func NewQRCodeHandler(db *gorm.DB) *QRCodeHandler {
	return &QRCodeHandler{db: db}
}

func (h *QRCodeHandler) List(c *fiber.Ctx) error {
	var qrCodes []models.QRCode
	if err := h.db.Preload("Table").Order("created_at DESC").Find(&qrCodes).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get QR codes")
	}
	return utils.Success(c, fiber.StatusOK, "QR codes retrieved successfully", qrCodes)
}

func (h *QRCodeHandler) Get(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	qrCode, err := h.find(id)
	if err != nil {
		return qrCodeLookupError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "QR code retrieved successfully", qrCode)
}

func (h *QRCodeHandler) GenerateTable(c *fiber.Ctx) error {
	tableID, err := parseID(c, "table_id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var table models.Table
	if err := h.db.First(&table, tableID).Error; err != nil {
		return tableLookupError(c, err)
	}
	code, err := generateQRCode()
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to generate QR code")
	}
	qrCode := models.QRCode{
		Code:     code,
		Type:     models.QRCodeDineIn,
		TableID:  &table.ID,
		URL:      fmt.Sprintf("/order?type=dine_in&table=%d", table.ID),
		IsActive: true,
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.QRCode{}).Where("table_id = ? AND is_active = ?", table.ID, true).Update("is_active", false).Error; err != nil {
			return err
		}
		if err := tx.Create(&qrCode).Error; err != nil {
			return err
		}
		return tx.Model(&table).Update("qr_code", qrCode.Code).Error
	}); err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to save QR code")
	}
	table.QRCode = qrCode.Code
	qrCode.Table = &table
	return utils.Success(c, fiber.StatusCreated, "table QR code generated successfully", qrCode)
}

func (h *QRCodeHandler) GenerateTakeaway(c *fiber.Ctx) error {
	code, err := generateQRCode()
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to generate QR code")
	}
	qrCode := models.QRCode{
		Code:     code,
		Type:     models.QRCodeTakeAway,
		URL:      "/order?type=take_away",
		IsActive: true,
	}
	if err := h.db.Create(&qrCode).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to save QR code")
	}
	return utils.Success(c, fiber.StatusCreated, "takeaway QR code generated successfully", qrCode)
}

func (h *QRCodeHandler) UpdateActive(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var request qrCodeActiveRequest
	if err := c.BodyParser(&request); err != nil || request.IsActive == nil {
		return utils.Error(c, fiber.StatusBadRequest, "is_active is required")
	}
	qrCode, err := h.find(id)
	if err != nil {
		return qrCodeLookupError(c, err)
	}
	qrCode.IsActive = *request.IsActive
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if qrCode.Type == models.QRCodeDineIn && qrCode.TableID != nil {
			if qrCode.IsActive {
				if err := tx.Model(&models.QRCode{}).
					Where("table_id = ? AND id <> ? AND is_active = ?", *qrCode.TableID, qrCode.ID, true).
					Update("is_active", false).Error; err != nil {
					return err
				}
				if err := tx.Model(&models.Table{}).Where("id = ?", *qrCode.TableID).Update("qr_code", qrCode.Code).Error; err != nil {
					return err
				}
			} else if qrCode.Table != nil && qrCode.Table.QRCode == qrCode.Code {
				if err := tx.Model(&models.Table{}).Where("id = ?", *qrCode.TableID).Update("qr_code", "").Error; err != nil {
					return err
				}
			}
		}
		return tx.Save(&qrCode).Error
	}); err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to update QR code")
	}
	return utils.Success(c, fiber.StatusOK, "QR code status updated successfully", qrCode)
}

func (h *QRCodeHandler) find(id uint) (models.QRCode, error) {
	var qrCode models.QRCode
	err := h.db.Preload("Table").First(&qrCode, id).Error
	return qrCode, err
}

func generateQRCode() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func qrCodeLookupError(c *fiber.Ctx, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.Error(c, fiber.StatusNotFound, "QR code not found")
	}
	return utils.Error(c, fiber.StatusInternalServerError, "failed to get QR code")
}
