package handlers

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func parseID(c *fiber.Ctx, param string) (uint, error) {
	id, err := strconv.ParseUint(c.Params(param), 10, 64)
	if err != nil || id == 0 {
		return 0, errors.New("invalid id")
	}
	return uint(id), nil
}

func isDuplicateKey(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate key")
}

func isValidHTTPURL(value string) bool {
	if value == "" {
		return true
	}
	parsedURL, err := url.ParseRequestURI(value)
	return err == nil && (parsedURL.Scheme == "http" || parsedURL.Scheme == "https") && parsedURL.Host != ""
}

func recordExists[T any](db *gorm.DB, id uint) (bool, error) {
	var count int64
	err := db.Model(new(T)).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}
