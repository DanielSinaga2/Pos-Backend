package middleware

import (
	"strings"

	"pos-backend/utils"

	"github.com/gofiber/fiber/v2"
)

func WebSocketJWTAuth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := strings.TrimSpace(c.Query("token"))
		if token == "" {
			authHeader := c.Get("Authorization")
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				token = strings.TrimSpace(parts[1])
			}
		}
		if token == "" {
			return utils.Error(c, fiber.StatusUnauthorized, "authentication token is required")
		}

		claims, err := utils.ParseToken(token, secret)
		if err != nil {
			return utils.Error(c, fiber.StatusUnauthorized, "invalid or expired token")
		}
		c.Locals("user_id", claims.UserID)
		c.Locals("user_role", claims.Role)
		return c.Next()
	}
}
