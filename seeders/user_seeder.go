package seeders

import (
	"fmt"

	"pos-backend/models"
	"pos-backend/utils"

	"gorm.io/gorm"
)

type defaultUser struct {
	Name     string
	Email    string
	Password string
	Role     models.Role
}

func SeedDefaultUsers(db *gorm.DB) error {
	users := []defaultUser{
		{Name: "Admin", Email: "admin@mail.com", Password: "admin123", Role: models.RoleAdmin},
		{Name: "Cashier", Email: "cashier@mail.com", Password: "cashier123", Role: models.RoleCashier},
		{Name: "Kitchen", Email: "kitchen@mail.com", Password: "kitchen123", Role: models.RoleKitchen},
	}

	for _, defaultUser := range users {
		var count int64
		if err := db.Model(&models.User{}).Where("email = ?", defaultUser.Email).Count(&count).Error; err != nil {
			return fmt.Errorf("check default user %s: %w", defaultUser.Email, err)
		}
		if count > 0 {
			continue
		}

		hashedPassword, err := utils.HashPassword(defaultUser.Password)
		if err != nil {
			return fmt.Errorf("hash default user password %s: %w", defaultUser.Email, err)
		}

		user := models.User{
			Name:     defaultUser.Name,
			Email:    defaultUser.Email,
			Password: hashedPassword,
			Role:     defaultUser.Role,
		}
		if err := db.Create(&user).Error; err != nil {
			return fmt.Errorf("create default user %s: %w", defaultUser.Email, err)
		}
	}

	return nil
}
