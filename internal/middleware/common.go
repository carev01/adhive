package middleware

import (
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger returns a gin middleware for request logging
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		log.Printf("[%d] %s %s %v",
			status,
			c.Request.Method,
			path,
			latency,
		)
	}
}

// Recover returns a gin middleware for panic recovery
func Recover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic recovered: %v", err)
				c.AbortWithStatusJSON(500, gin.H{
					"type":   "about:blank",
					"title":  "Internal Server Error",
					"status": 500,
					"detail": "an unexpected error occurred",
				})
			}
		}()
		c.Next()
	}
}

func hasTraversalToken(v string) bool {
	if v == "" {
		return false
	}
	l := strings.ToLower(v)
	if strings.Contains(l, "%2e") || strings.Contains(l, "%2f") || strings.Contains(l, "%5c") {
		return true
	}
	n := strings.ReplaceAll(l, "\\", "/")
	if strings.Contains(n, "../") || strings.HasPrefix(n, "..") || strings.Contains(n, "/..") {
		return true
	}
	for _, seg := range strings.Split(n, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// RawPathTraversalGuard blocks traversal payloads before Gin route normalization/param binding logic can hide them.
func RawPathTraversalGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		candidates := []string{c.Request.RequestURI, c.Request.URL.EscapedPath(), c.Request.URL.RawPath, c.Request.URL.Path}
		for _, raw := range candidates {
			if raw == "" {
				continue
			}
			if hasTraversalToken(raw) {
				c.AbortWithStatusJSON(400, gin.H{"error": "invalid traversal payload"})
				return
			}
			decoded := raw
			for i := 0; i < 3; i++ {
				next, err := url.PathUnescape(decoded)
				if err != nil {
					c.AbortWithStatusJSON(400, gin.H{"error": "invalid traversal payload"})
					return
				}
				if hasTraversalToken(next) {
					c.AbortWithStatusJSON(400, gin.H{"error": "invalid traversal payload"})
					return
				}
				if next == decoded {
					break
				}
				decoded = next
			}
		}
		c.Next()
	}
}

// CORS returns a gin middleware for CORS handling
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
