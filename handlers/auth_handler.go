package handlers

import (
	"errors"
	"strings"

	"pos-backend/config"
	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type AuthHandler struct {
	db     *gorm.DB
	config *config.Config
}

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string      `json:"token"`
	User  models.User `json:"user"`
}

func NewAuthHandler(db *gorm.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{db: db, config: cfg}
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var request registerRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}

	request.Name = strings.TrimSpace(request.Name)
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	if request.Name == "" || request.Email == "" || request.Password == "" {
		return utils.Error(c, fiber.StatusBadRequest, "name, email, and password are required")
	}
	if len(request.Password) < 8 {
		return utils.Error(c, fiber.StatusBadRequest, "password must be at least 8 characters")
	}

	var existingUser models.User
	err := h.db.Where("email = ?", request.Email).First(&existingUser).Error
	if err == nil {
		return utils.Error(c, fiber.StatusConflict, "email is already registered")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to check email availability")
	}

	hashedPassword, err := utils.HashPassword(request.Password)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to secure password")
	}

	user := models.User{
		Name:     request.Name,
		Email:    request.Email,
		Password: hashedPassword,
		Role:     models.RoleCashier,
	}
	if err := h.db.Create(&user).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to create user")
	}

	token, err := utils.GenerateToken(user, h.config.JWTSecret)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to generate token")
	}

	return utils.Success(c, fiber.StatusCreated, "user registered successfully", authResponse{
		Token: token,
		User:  user,
	})
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var request loginRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}

	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	if request.Email == "" || request.Password == "" {
		return utils.Error(c, fiber.StatusBadRequest, "email and password are required")
	}

	var user models.User
	if err := h.db.Where("email = ?", request.Email).First(&user).Error; err != nil {
		return utils.Error(c, fiber.StatusUnauthorized, "invalid email or password")
	}
	if err := utils.CheckPassword(user.Password, request.Password); err != nil {
		return utils.Error(c, fiber.StatusUnauthorized, "invalid email or password")
	}

	token, err := utils.GenerateToken(user, h.config.JWTSecret)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to generate token")
	}

	return utils.Success(c, fiber.StatusOK, "login successful", authResponse{
		Token: token,
		User:  user,
	})
}

func (h *AuthHandler) Me(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return utils.Error(c, fiber.StatusUnauthorized, "authentication is required")
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "user not found")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get user profile")
	}

	return utils.Success(c, fiber.StatusOK, "profile retrieved successfully", user)
}
