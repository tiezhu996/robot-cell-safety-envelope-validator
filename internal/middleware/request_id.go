package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

func RequestID() gin.HandlerFunc {
	return func(context *gin.Context) {
		requestID := strings.TrimSpace(context.GetHeader("X-Request-ID"))
		if requestID == "" || len(requestID) > 80 {
			requestID = newRequestID()
		}
		context.Set("request_id", requestID)
		context.Header("X-Request-ID", requestID)
		context.Next()
	}
}

func CORS(origin string) gin.HandlerFunc {
	return func(context *gin.Context) {
		requestOrigin := context.GetHeader("Origin")
		if requestOrigin != "" && requestOrigin == origin {
			context.Header("Access-Control-Allow-Origin", origin)
			context.Header("Vary", "Origin")
			context.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, Idempotency-Key")
			context.Header("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		}
		if context.Request.Method == http.MethodOptions {
			context.AbortWithStatus(http.StatusNoContent)
			return
		}
		context.Next()
	}
}

type rateBucket struct {
	minute int64
	count  int
}

// rateLimiter counts requests per (scope, client) minute window. The mutex is
// mandatory: Gin serves concurrent requests in separate goroutines, so an
// unguarded map crashes the process with "concurrent map read and map write".
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]rateBucket
}

var globalRateLimiter = &rateLimiter{buckets: make(map[string]rateBucket)}

// RateLimit caps requests per minute per client within scope. The bucket key
// combines scope and client IP, so each client gets its own budget and one
// client exhausting its limit never throttles another.
func RateLimit(limit int, scope string) gin.HandlerFunc {
	return func(context *gin.Context) {
		minute := time.Now().Unix() / 60
		key := scope + ":" + context.ClientIP()
		blocked := globalRateLimiter.take(key, minute, limit)
		if blocked {
			context.Header("Retry-After", "60")
			context.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": gin.H{"code": "rate_limited", "message": "local request limit exceeded for " + scope}, "request_id": context.GetString("request_id")})
			return
		}
		context.Next()
	}
}

func (limiter *rateLimiter) take(key string, minute int64, limit int) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	bucket := limiter.buckets[key]
	if bucket.minute != minute {
		bucket = rateBucket{minute: minute}
	}
	bucket.count++
	limiter.buckets[key] = bucket
	if len(limiter.buckets) > 4096 {
		for candidate, value := range limiter.buckets {
			if value.minute < minute-1 {
				delete(limiter.buckets, candidate)
			}
		}
	}
	return bucket.count > limit
}

func newRequestID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(buffer)
}
