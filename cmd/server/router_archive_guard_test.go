package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carev01/adhive/internal/handler"
	"github.com/carev01/adhive/internal/middleware"
	"github.com/gin-gonic/gin"
)

func TestArchivesRoute_UUIDGuard_BlocksNormalizedPlainTraversal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RawPathTraversalGuard())

	api := r.Group("/api/v1")
	files := api.Group("/files")
	{
		archivesUUID := files.Group("/archives/:entryID")
		archivesUUID.Use(func(c *gin.Context) {
			id := c.Param("entryID")
			if len(id) != 36 {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid entry_id"})
				return
			}
			c.Next()
		})
		archivesUUID.GET("", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"files": []string{}})
		})
	}

	cases := []string{
		"/api/v1/files/archives/../test",
		"/api/v1/files/archives/%2e%2e/test",
		"/api/v1/files/archives/....//test",
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

	// Control case: valid UUID-format id reaches handler (actual existence handled downstream)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/archives/11111111-1111-1111-1111-111111111111", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid UUID path guard control got %d", w.Code)
	}

	_ = handler.ErrorResponse{}
}
