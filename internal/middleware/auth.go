package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/drivebai/backend/internal/auth"
	"github.com/drivebai/backend/internal/httputil"
	"github.com/drivebai/backend/internal/models"
)

// AuthMiddleware creates authentication middleware
func AuthMiddleware(jwtService *auth.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
				return
			}

			tokenString := parts[1]
			claims, err := jwtService.ValidateAccessToken(tokenString)
			if err != nil {
				if apiErr := models.GetAPIError(err); apiErr != nil {
					httputil.WriteError(w, http.StatusUnauthorized, apiErr)
				} else {
					httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
				}
				return
			}

			// Add user info to context
			ctx := r.Context()
			ctx = context.WithValue(ctx, httputil.UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, httputil.EmailKey, claims.Email)
			ctx = context.WithValue(ctx, httputil.RoleKey, claims.Role)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole creates middleware that checks for specific roles
func RequireRole(roles ...models.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userRole, ok := httputil.GetRole(r.Context())
			if !ok {
				httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
				return
			}

			for _, role := range roles {
				if userRole == role {
					next.ServeHTTP(w, r)
					return
				}
			}

			httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "Insufficient permissions"))
		})
	}
}
