package middleware

import (
	"pos-backend/models"
	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
)

func AllowRoles(allowedRoles ...models.Role) fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, ok := c.Locals("user_role").(models.Role)
		if !ok {
			return utils.Error(c, fiber.StatusUnauthorized, "authentication is required")
		}

		for _, allowedRole := range allowedRoles {
			if role == allowedRole {
				return c.Next()
			}
		}

		return utils.Error(c, fiber.StatusForbidden, "you do not have permission to access this resource")
	}
}
