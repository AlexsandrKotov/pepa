package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims represents JWT claims for authenticated requests.
type Claims struct {
	UserID         uuid.UUID `json:"user_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Email          string    `json:"email"`
	Roles          []string  `json:"roles"`
	TokenVersion   int       `json:"token_version"`
	jwt.RegisteredClaims
}

// Context keys for storing claims in gin.Context.
const (
	CtxUserID       = "auth_user_id"
	CtxTenantID     = "auth_tenant_id"
	CtxOrgID        = "auth_org_id"
	CtxEmail        = "auth_email"
	CtxRoles        = "auth_roles"
	CtxTokenVersion = "auth_token_version" // #nosec // G101: not a credential, just a context key name
)

// CookieTokenName is the name of the httpOnly cookie that carries the JWT.
// Preferred over the Authorization header because it is not readable from
// JavaScript (XSS-resistant).
const CookieTokenName = "pepa_token"

// Middleware validates JWT tokens and injects claims into context.
// Always requires a valid token - no development bypass for security.
// The token is read from the Authorization header first, then from the
// httpOnly pepa_token cookie.
func Middleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var tokenStr string

		if header := c.GetHeader("Authorization"); header != "" {
			trimmed := strings.TrimPrefix(header, "Bearer ")
			if trimmed == header {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
				c.Abort()
				return
			}
			tokenStr = trimmed
		} else if cookieVal, err := c.Cookie(CookieTokenName); err == nil && cookieVal != "" {
			tokenStr = cookieVal
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header or token cookie"})
			c.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(jwtSecret), nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}

		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxTenantID, claims.TenantID)
		c.Set(CtxOrgID, claims.OrganizationID)
		c.Set(CtxEmail, claims.Email)
		c.Set(CtxRoles, claims.Roles)
		c.Set(CtxTokenVersion, claims.TokenVersion)
		c.Next()
	}
}

// GetTenantID extracts tenant ID from gin context.
func GetTenantID(c *gin.Context) uuid.UUID {
	if v, ok := c.Get(CtxTenantID); ok {
		if id, ok := v.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.UUID{}
}

// GetOrgID extracts organization ID from gin context.
func GetOrgID(c *gin.Context) uuid.UUID {
	if v, ok := c.Get(CtxOrgID); ok {
		if id, ok := v.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.UUID{}
}

// GetUserID extracts user ID from gin context.
func GetUserID(c *gin.Context) *uuid.UUID {
	if v, ok := c.Get(CtxUserID); ok {
		if id, ok := v.(uuid.UUID); ok {
			return &id
		}
	}
	return nil
}

// GetRoles extracts roles from gin context.
func GetRoles(c *gin.Context) []string {
	if v, ok := c.Get(CtxRoles); ok {
		if roles, ok := v.([]string); ok {
			return roles
		}
	}
	return nil
}

// GetEmail extracts email from gin context.
func GetEmail(c *gin.Context) string {
	if v, ok := c.Get(CtxEmail); ok {
		if email, ok := v.(string); ok {
			return email
		}
	}
	return ""
}

// GetTokenVersion extracts the token_version claim from gin context.
func GetTokenVersion(c *gin.Context) int {
	if v, ok := c.Get(CtxTokenVersion); ok {
		if tv, ok := v.(int); ok {
			return tv
		}
	}
	return 0
}

// IsPlatformAdmin reports whether the authenticated user holds a platform-wide
// admin role, based on the verified JWT. The role list mirrors the admin bypass
// in rbacMiddleware so that authorization and tenant-scoping bypasses agree.
func IsPlatformAdmin(c *gin.Context) bool {
	for _, r := range GetRoles(c) {
		switch strings.ToLower(r) {
		case "admin", "super_admin", "platform admin", "platform_admin":
			return true
		}
	}
	return false
}

// ValidateJWT parses and validates a JWT token string, returning the claims.
// Used by WebSocket handlers where the token is passed as a query parameter.
func ValidateJWT(tokenStr, jwtSecret string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(jwtSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, jwt.ErrSignatureInvalid
	}
	return claims, nil
}
