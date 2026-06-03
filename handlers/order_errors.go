package handlers

import (
	"errors"

	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func orderError(c *fiber.Ctx, err error, fallback string) error {
	var serviceError *orderServiceError
	if errors.As(err, &serviceError) {
		return utils.Error(c, serviceError.Status, serviceError.Message)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.Error(c, fiber.StatusNotFound, "order not found")
	}
	return utils.Error(c, fiber.StatusInternalServerError, fallback)
}

func orderLookupError(c *fiber.Ctx, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.Error(c, fiber.StatusNotFound, "order not found")
	}
	return utils.Error(c, fiber.StatusInternalServerError, "failed to get order")
}
