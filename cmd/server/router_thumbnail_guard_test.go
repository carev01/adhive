package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carev01/adhive/internal/middleware"
	"github.com/gin-gonic/gin"
)

func TestThumbnailsRoute_UUIDGuard_BlocksPlainTraversal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RawPathTraversalGuard())

	api := r.Group("/api/v1")
	files := api.Group("/files")
	thumbUUID := files.Group("/thumbnails/:entryID")
	thumbUUID.Use(func(c *gin.Context) {
		id := c.Param("entryID")
		if len(id) != 36 || strings.Count(id, "-") != 4 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid entry_id"})
			return
		}
		c.Next()
	})
	thumbUUID.GET("", func(c *gin.Context) { c.Status(http.StatusOK) })

	cases := []string{
		"/api/v1/files/thumbnails/../test",
		"/api/v1/files/thumbnails/%2e%2e/test",
		"/api/v1/files/thumbnails/....//test",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, u, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code < 400 || w.Code >= 500 {
				t.Fatalf("expected 4xx for %s got %d body=%s", u, w.Code, w.Body.String())
			}
		})
	}
}
