package middleware

import (
	"strings"

	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
)

func JWTAuth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return utils.Error(c, fiber.StatusUnauthorized, "authorization header is required")
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			return utils.Error(c, fiber.StatusUnauthorized, "invalid authorization header")
		}

		claims, err := utils.ParseToken(parts[1], secret)
		if err != nil {
			return utils.Error(c, fiber.StatusUnauthorized, "invalid or expired token")
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("user_role", claims.Role)

		return c.Next()
	}
}
