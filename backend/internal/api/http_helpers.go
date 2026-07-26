package api

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// maxRequestBodyBytes caps any single request body. The largest legitimate
// payload here is a smart-draft prompt with recent post history, which is
// comfortably under this.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

func limitRequestBody(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}

func bindJSONOrBadRequest(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return false
	}
	return true
}

// serverError logs the underlying failure and returns a generic message.
//
// The raw error here is a GORM/SQL error: returning it told the caller about
// table and column names, and about which constraint they had just tripped.
// The detail belongs in the server log, not the response.
func serverError(c *gin.Context, err error) {
	log.Printf("request failed method=%s path=%s err=%v", c.Request.Method, c.Request.URL.Path, err)
	fail(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
}

func writeRecordError(c *gin.Context, err error, notFoundMsg, internalMsg string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": notFoundMsg})
		return
	}
	if strings.TrimSpace(internalMsg) == "" {
		internalMsg = err.Error()
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": internalMsg})
}
