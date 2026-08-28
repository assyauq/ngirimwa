package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCSRouteGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		role       string
		method     string
		path       string
		route      string
		wantStatus int
	}{
		{
			name: "cs may send from assigned inbox", role: "cs", method: http.MethodPost,
			path: "/api/agents/12/send", route: "/api/agents/:id/send", wantStatus: http.StatusNoContent,
		},
		{
			name: "cs may stream assigned inbox events", role: "cs", method: http.MethodGet,
			path: "/api/agents/12/inbox/events", route: "/api/agents/:id/inbox/events", wantStatus: http.StatusNoContent,
		},
		{
			name: "cs may poll assigned inbox incoming cursor", role: "cs", method: http.MethodGet,
			path: "/api/agents/12/inbox/incoming-cursor", route: "/api/agents/:id/inbox/incoming-cursor", wantStatus: http.StatusNoContent,
		},
		{
			name: "cs may submit assigned inbox diagnostics", role: "cs", method: http.MethodPost,
			path: "/api/agents/12/inbox/client-debug", route: "/api/agents/:id/inbox/client-debug", wantStatus: http.StatusNoContent,
		},
		{
			name: "cs cannot create whatsapp agent", role: "cs", method: http.MethodPost,
			path: "/api/agents", route: "/api/agents", wantStatus: http.StatusForbidden,
		},
		{
			name: "owner may use admin route", role: "owner", method: http.MethodPost,
			path: "/api/agents", route: "/api/agents", wantStatus: http.StatusNoContent,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Handle(tc.method, tc.route,
				func(c *gin.Context) {
					c.Set("role", tc.role)
					c.Set("is_super_admin", false)
					c.Next()
				},
				CSRouteGuard(),
				func(c *gin.Context) { c.Status(http.StatusNoContent) },
			)
			request := httptest.NewRequest(tc.method, tc.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tc.wantStatus, response.Body.String())
			}
		})
	}
}

func TestCSUsernamePattern(t *testing.T) {
	valid := []string{"cs.andi", "operator_01", "cs-malam"}
	invalid := []string{"ab", "cs malam", "cs@kantor"}
	for _, username := range valid {
		if !csUsernamePattern.MatchString(username) {
			t.Errorf("username %q seharusnya valid", username)
		}
	}
	for _, username := range invalid {
		if csUsernamePattern.MatchString(username) {
			t.Errorf("username %q seharusnya ditolak", username)
		}
	}
}
