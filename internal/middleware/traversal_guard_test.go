package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRawPathTraversalGuard_BlocksBeforeRouteNormalization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RawPathTraversalGuard())

	hit := false
	r.GET("/api/v1/files/archives/:entryID", func(c *gin.Context) {
		hit = true
		c.JSON(http.StatusOK, gin.H{"files": []string{}})
	})

	cases := []string{
		"/api/v1/files/archives/../test",
		"/api/v1/files/archives/%2e%2e/test",
		"/api/v1/files/archives/..%2Ftest",
		"/api/v1/files/archives/%252e%252e/test",
	}

	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			hit = false
			req := httptest.NewRequest(http.MethodGet, u, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code < 400 || w.Code >= 500 {
				t.Fatalf("expected 4xx for %s, got %d body=%s", u, w.Code, w.Body.String())
			}
			if hit {
				t.Fatalf("route handler should not be reached for %s", u)
			}
		})
	}
}
