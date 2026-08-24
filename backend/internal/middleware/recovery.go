package middleware

import (
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

func Recovery() gin.HandlerFunc {
	return func(context *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("panic recovered", "request_id", context.GetString("request_id"), "panic", recovered, "stack", string(debug.Stack()))
			}
		}()
		context.Next()
	}
}
