package api

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func adminWebCORS() gin.HandlerFunc {
	allowedOrigins := map[string]struct{}{}
	defaultOrigins := []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
		"http://localhost:4173",
		"http://127.0.0.1:4173",
	}
	for _, origin := range defaultOrigins {
		allowedOrigins[origin] = struct{}{}
	}
	for _, raw := range strings.Split(os.Getenv("ADMIN_WEB_ORIGIN"), ",") {
		value := strings.TrimSpace(raw)
		if value != "" {
			allowedOrigins[value] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin != "" {
			if _, ok := allowedOrigins[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-User-ID, X-User-Role")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
