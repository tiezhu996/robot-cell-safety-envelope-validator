package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"robot-cell-safety-envelope-validator/backend/internal/dto"
	"robot-cell-safety-envelope-validator/backend/internal/service"
)

func RBAC(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}
	return func(context *gin.Context) {
		value, exists := context.Get(actorContextKey)
		actor, valid := value.(dto.Actor)
		if !exists || !valid {
			writeAuthError(context, service.Unauthorized("authentication context is missing"))
			return
		}
		if !allowed[actor.Role] {
			context.JSON(http.StatusForbidden, gin.H{
				"error":      gin.H{"code": "forbidden", "message": "role is not allowed to perform this action"},
				"request_id": context.GetString("request_id"),
			})
		}
		context.Next()
	}
}
