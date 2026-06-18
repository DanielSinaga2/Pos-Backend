package handlers

import (
	"errors"
	"strings"
	"time"

	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type UserHandler struct {
	db *gorm.DB
}

type userRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type userResponse struct {
	ID        uint        `json:"id"`
	Name      string      `json:"name"`
	Email     string      `json:"email"`
	Role      models.Role `json:"role"`
	IsActive  bool        `json:"is_active"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

var errUserAlreadyInactive = errors.New("user already inactive")

func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{db: db}
}

func (h *UserHandler) List(c *fiber.Ctx) error {
	var users []models.User
	status := strings.ToLower(strings.TrimSpace(c.Query("status", "active")))

	query := h.db.Order("name ASC")
	switch status {
	case "all":
	case "active", "":
		query = query.Where("is_active = ?", true)
	case "inactive":
		query = query.Where("is_active = ?", false)
	default:
		return utils.Error(c, fiber.StatusBadRequest, "status must be all, active, or inactive")
	}

	if err := query.Find(&users).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to get users")
	}

	return utils.Success(c, fiber.StatusOK, "users retrieved successfully", toUserResponses(users))
}

func (h *UserHandler) Get(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	user, err := h.find(id)
	if err != nil {
		return userLookupError(c, err)
	}

	return utils.Success(c, fiber.StatusOK, "user retrieved successfully", toUserResponse(user))
}

func (h *UserHandler) Create(c *fiber.Ctx) error {
	var request userRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}

	if message := validateUserRequest(&request, true); message != "" {
		return utils.Error(c, fiber.StatusBadRequest, message)
	}

	available, err := h.isEmailAvailable(request.Email, 0)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to check email availability")
	}
	if !available {
		return utils.Error(c, fiber.StatusConflict, "email is already registered")
	}

	hashedPassword, err := utils.HashPassword(request.Password)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to secure password")
	}

	user := models.User{
		Name:     request.Name,
		Email:    request.Email,
		Password: hashedPassword,
		Role:     models.Role(request.Role),
		IsActive: true,
	}
	if err := h.db.Create(&user).Error; err != nil {
		if isDuplicateKey(err) {
			return utils.Error(c, fiber.StatusConflict, "email is already registered")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to create user")
	}

	return utils.Success(c, fiber.StatusCreated, "user created successfully", toUserResponse(user))
}

func (h *UserHandler) Update(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	var request userRequest
	if err := c.BodyParser(&request); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}

	if message := validateUserRequest(&request, false); message != "" {
		return utils.Error(c, fiber.StatusBadRequest, message)
	}

	user, err := h.find(id)
	if err != nil {
		return userLookupError(c, err)
	}

	available, err := h.isEmailAvailable(request.Email, id)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "failed to check email availability")
	}
	if !available {
		return utils.Error(c, fiber.StatusConflict, "email is already registered")
	}

	user.Name = request.Name
	user.Email = request.Email
	user.Role = models.Role(request.Role)
	if request.Password != "" {
		hashedPassword, err := utils.HashPassword(request.Password)
		if err != nil {
			return utils.Error(c, fiber.StatusInternalServerError, "failed to secure password")
		}
		user.Password = hashedPassword
	}

	if err := h.db.Save(&user).Error; err != nil {
		if isDuplicateKey(err) {
			return utils.Error(c, fiber.StatusConflict, "email is already registered")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to update user")
	}

	return utils.Success(c, fiber.StatusOK, "user updated successfully", toUserResponse(user))
}

func (h *UserHandler) Delete(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	currentUserID, ok := c.Locals("user_id").(uint)
	if !ok {
		return utils.Error(c, fiber.StatusUnauthorized, "authentication is required")
	}
	if id == currentUserID {
		return utils.Error(c, fiber.StatusBadRequest, "you cannot delete your own user")
	}

	var user models.User
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&user, id).Error; err != nil {
			return err
		}
		if !user.IsActive {
			return errUserAlreadyInactive
		}
		if err := tx.Model(&user).Update("is_active", false).Error; err != nil {
			return err
		}
		user.IsActive = false
		return nil
	}); err != nil {
		if errors.Is(err, errUserAlreadyInactive) {
			return utils.Error(c, fiber.StatusBadRequest, "user already inactive")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return userLookupError(c, err)
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to deactivate user")
	}

	return utils.Success(c, fiber.StatusOK, "user deactivated successfully", nil)
}

func (h *UserHandler) Activate(c *fiber.Ctx) error {
	id, err := parseID(c, "id")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	var user models.User
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&user, id).Error; err != nil {
			return err
		}
		if err := tx.Model(&user).Update("is_active", true).Error; err != nil {
			return err
		}
		user.IsActive = true
		return nil
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return userLookupError(c, err)
		}
		return utils.Error(c, fiber.StatusInternalServerError, "failed to activate user")
	}

	return utils.Success(c, fiber.StatusOK, "user activated successfully", nil)
}

func (h *UserHandler) find(id uint) (models.User, error) {
	var user models.User
	err := h.db.First(&user, id).Error
	return user, err
}

func (h *UserHandler) isEmailAvailable(email string, exceptID uint) (bool, error) {
	var count int64
	query := h.db.Model(&models.User{}).Where("email = ?", email)
	if exceptID != 0 {
		query = query.Where("id <> ?", exceptID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count == 0, nil
}

func validateUserRequest(request *userRequest, requirePassword bool) string {
	request.Name = strings.TrimSpace(request.Name)
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	request.Password = strings.TrimSpace(request.Password)
	request.Role = strings.ToLower(strings.TrimSpace(request.Role))

	if request.Name == "" {
		return "name is required"
	}
	if request.Email == "" {
		return "email is required"
	}
	if requirePassword && request.Password == "" {
		return "password is required"
	}
	if request.Password != "" && len(request.Password) < 6 {
		return "password must be at least 6 characters"
	}
	if !isValidUserRole(request.Role) {
		return "role must be admin, cashier, or kitchen"
	}
	return ""
}

func isValidUserRole(role string) bool {
	switch models.Role(role) {
	case models.RoleAdmin, models.RoleCashier, models.RoleKitchen:
		return true
	default:
		return false
	}
}

func toUserResponse(user models.User) userResponse {
	return userResponse{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		Role:      user.Role,
		IsActive:  user.IsActive,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

func toUserResponses(users []models.User) []userResponse {
	responses := make([]userResponse, 0, len(users))
	for _, user := range users {
		responses = append(responses, toUserResponse(user))
	}
	return responses
}

func userLookupError(c *fiber.Ctx, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.Error(c, fiber.StatusNotFound, "user not found")
	}
	return utils.Error(c, fiber.StatusInternalServerError, "failed to get user")
}
