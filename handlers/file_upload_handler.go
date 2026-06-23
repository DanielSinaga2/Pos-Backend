package handlers

import (
	"errors"
	"fmt"
	"strings"

	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h *MenuHandler) UploadImage(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	menu, err := h.find(id)
	if err != nil {
		return menuLookupError(c, err)
	}
	header, err := c.FormFile("image")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "file tidak valid")
	}
	imageURL, savedPath, err := utils.SaveUploadedImage(header, "uploads/menus", fmt.Sprintf("menu_%d", menu.ID))
	if err != nil {
		return uploadFileError(c, err)
	}
	oldImageURL := menu.ImageURL
	if err := h.db.Model(&menu).Update("image_url", imageURL).Error; err != nil {
		utils.RemoveUploadedFile(savedPath)
		return utils.Error(c, fiber.StatusInternalServerError, "gagal menyimpan gambar menu")
	}
	updatedMenu, err := h.find(menu.ID)
	if err != nil {
		return menuLookupError(c, err)
	}
	if oldImageURL != imageURL {
		utils.RemoveUploadedFileByURL(oldImageURL)
	}
	return utils.Success(c, fiber.StatusOK, "upload berhasil", updatedMenu)
}

func (h *PublicOrderHandler) UploadPaymentProofFile(c *fiber.Ctx) error {
	orderCode := strings.TrimSpace(c.Params("order_code"))
	if orderCode == "" {
		return utils.Error(c, fiber.StatusBadRequest, "order_code is required")
	}
	var order models.Order
	if err := h.db.Preload("Payment").Where("order_code = ?", orderCode).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "order not found")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get order")
	}
	if message := validatePaymentProofUpload(order); message != "" {
		return utils.Error(c, fiber.StatusBadRequest, message)
	}

	header, err := c.FormFile("image")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "file tidak valid")
	}
	imageURL, savedPath, err := utils.SaveUploadedImage(header, "uploads/payments", "payment_"+order.OrderCode)
	if err != nil {
		return uploadFileError(c, err)
	}

	oldImageURL := order.Payment.ProofImageURL
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Payment").
			Where("order_code = ?", orderCode).
			First(&order).Error; err != nil {
			return err
		}
		if message := validatePaymentProofUpload(order); message != "" {
			return &orderServiceError{Status: fiber.StatusBadRequest, Message: message}
		}
		oldImageURL = order.Payment.ProofImageURL
		return tx.Model(order.Payment).Updates(map[string]any{
			"proof_image_url": imageURL,
			"status":          models.PaymentWaitingConfirmation,
		}).Error
	})
	if err != nil {
		utils.RemoveUploadedFile(savedPath)
		return orderError(c, err, "failed to upload payment proof")
	}

	updatedOrder, err := findOrder(h.db, order.ID)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get updated order")
	}
	if oldImageURL != imageURL {
		utils.RemoveUploadedFileByURL(oldImageURL)
	}
	broadcastPaymentWaitingConfirmation(updatedOrder)
	return utils.Success(c, fiber.StatusOK, "upload berhasil", updatedOrder)
}

func validatePaymentProofUpload(order models.Order) string {
	if order.Status == models.OrderCancelled || order.Status == models.OrderCompleted {
		return "payment proof cannot be uploaded for this order"
	}
	if order.Payment == nil {
		return "payment record not found"
	}
	if !models.IsOnlinePaymentMethod(order.Payment.PaymentMethod) {
		return "payment proof is only accepted for qris payment"
	}
	if order.Payment.Status == models.PaymentPaid {
		return "payment has already been confirmed"
	}
	return ""
}

func uploadFileError(c *fiber.Ctx, err error) error {
	if errors.Is(err, utils.ErrInvalidUploadFile) {
		return utils.Error(c, fiber.StatusBadRequest, "file tidak valid")
	}
	return utils.Error(c, fiber.StatusInternalServerError, "gagal menyimpan file")
}
