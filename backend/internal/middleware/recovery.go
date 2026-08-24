package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

func Recovery() gin.HandlerFunc {
	return func(context *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			requestID := context.GetString("request_id")
			slog.Error("panic recovered",
				"request_id", requestID,
				"panic", recovered,
				"stack", string(debug.Stack()),
			)
			// Abort the chain so no later handlers/middleware keep running on a
			// half-written context, then emit a JSON body so the client gets a
			// deterministic 500 instead of an empty response that hangs until
			// its read timeout fires.
			context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    "internal_error",
					"message": "an unexpected error occurred",
				},
				"request_id": requestID,
			})
		}()
		context.Next()
	}
}
