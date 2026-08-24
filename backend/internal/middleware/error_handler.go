package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorHandler sets default security headers and acts as the last line of
// defense: if a handler pushed errors onto the context but never wrote a
// response (or aborted without a body), the client would otherwise receive an
// empty response with nothing in the server logs. Translate any such pending
// errors into a deterministic 500 JSON body so the failure is always visible
// to the caller and to operators.
func ErrorHandler() gin.HandlerFunc {
	return func(context *gin.Context) {
		context.Header("X-Content-Type-Options", "nosniff")
		context.Header("X-Frame-Options", "DENY")
		context.Header("Referrer-Policy", "no-referrer")
		context.Next()

		if context.Writer.Written() {
			return
		}
		if len(context.Errors) == 0 {
			return
		}
		requestID := context.GetString("request_id")
		last := context.Errors.Last()
		slog.Error("unhandled request error",
			"request_id", requestID,
			"error", last.Err.Error(),
			"meta", last.Meta,
		)
		context.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "internal_error",
				"message": "an unexpected error occurred",
			},
			"request_id": requestID,
		})
	}
}
