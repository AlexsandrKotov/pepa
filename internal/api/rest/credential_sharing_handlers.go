package rest

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	pepacrypto "github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/repository"
)

// CredentialShare represents a sharing entry for a user credential.
type CredentialShare struct {
	ID             string `json:"id"`
	CredentialID   string `json:"credential_id"`
	OwnerUserID    string `json:"owner_user_id"`
	SharedWithUser *struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"shared_with_user,omitempty"`
	SharedWithTeam *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"shared_with_team,omitempty"`
	CreatedAt string `json:"created_at"`
}

// SharedCredential is returned when listing credentials shared with the current user.
type SharedCredential struct {
	ID          string `json:"id"`
	OwnerName   string `json:"owner_name"`
	OwnerEmail  string `json:"owner_email"`
	Provider    string `json:"provider"`
	ProviderURL string `json:"provider_url"`
	DisplayName string `json:"display_name"`
	TokenMasked string `json:"token_masked"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	CreatedAt   string `json:"created_at"`
}

// registerCredentialSharingRoutes registers sharing endpoints under /my/credentials.
func registerCredentialSharingRoutes(r interface{ POST(string, ...gin.HandlerFunc) gin.IRoutes }, deps Dependencies) {
	// This is called from registerUserCredentialRoutes, which already has the
	// /my/credentials group.  We add sharing sub-routes there.
}

func addSharingRoutes(creds *gin.RouterGroup, deps Dependencies) {
	creds.POST("/:id/share", shareMyCredential(deps))
	creds.GET("/:id/shares", listMyCredentialShares(deps))
	creds.DELETE("/:id/shares/:shareId", revokeMyCredentialShare(deps))
	creds.GET("/shared", listSharedWithMe(deps))
}

// shareMyCredential grants another user or team access to a credential.
func shareMyCredential(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		credID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential ID"})
			return
		}

		var req struct {
			SharedWithUser *string `json:"shared_with_user"`
			SharedWithTeam *string `json:"shared_with_team"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "shared_with_user or shared_with_team is required"})
			return
		}
		if req.SharedWithUser == nil && req.SharedWithTeam == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "shared_with_user or shared_with_team is required"})
			return
		}
		if req.SharedWithUser != nil && req.SharedWithTeam != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "specify only one of shared_with_user or shared_with_team"})
			return
		}

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		// Verify ownership
		owned, err := deps.Repos.UserCredential.VerifyOwnershipWithTenant(ctx, credID, *userID, tenantID)
		if err != nil || !owned {
			c.JSON(http.StatusNotFound, gin.H{"error": "credential not found or not owned by you"})
			return
		}

		// Cannot share with yourself
		if req.SharedWithUser != nil && *req.SharedWithUser == userID.String() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot share a credential with yourself"})
			return
		}

		var sharedWithUser, sharedWithTeam *uuid.UUID
		if req.SharedWithUser != nil {
			id, err := uuid.Parse(*req.SharedWithUser)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
				return
			}
			sharedWithUser = &id
		}
		if req.SharedWithTeam != nil {
			id, err := uuid.Parse(*req.SharedWithTeam)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
				return
			}
			sharedWithTeam = &id
		}

		shareID := uuid.New()
		share := &repository.CredentialShare{
			ID:             shareID,
			CredentialID:   credID,
			OwnerUserID:    *userID,
			TenantID:       tenantID,
			SharedWithUser: sharedWithUser,
			SharedWithTeam: sharedWithTeam,
		}

		if err := deps.Repos.CredentialShare.Create(ctx, share); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "share", "credential", credID.String(), nil, gin.H{
			"share_id": shareID.String(),
		})
		c.JSON(http.StatusCreated, gin.H{"id": shareID, "message": "credential shared"})
	}
}

// listMyCredentialShares lists all shares for a credential owned by the current user.
func listMyCredentialShares(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		credID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential ID"})
			return
		}

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		// Verify ownership
		owned, err := deps.Repos.UserCredential.VerifyOwnershipWithTenant(ctx, credID, *userID, tenantID)
		if err != nil || !owned {
			c.JSON(http.StatusNotFound, gin.H{"error": "credential not found or not owned by you"})
			return
		}

		shareModels, err := deps.Repos.CredentialShare.ListByCredential(ctx, credID, *userID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		shares := make([]CredentialShare, len(shareModels))
		for i, sm := range shareModels {
			shares[i] = CredentialShare{
				ID:           sm.ID.String(),
				CredentialID: sm.CredentialID.String(),
				OwnerUserID:  sm.OwnerUserID.String(),
				CreatedAt:    sm.CreatedAt.Format(time.RFC3339),
			}
			if sm.SharedWithUser != nil {
				shares[i].SharedWithUser = &struct {
					ID    string `json:"id"`
					Name  string `json:"name"`
					Email string `json:"email"`
				}{
					ID:    sm.SharedWithUser.String(),
					Name:  sm.SharedWithUserName,
					Email: sm.SharedWithUserEmail,
				}
			}
			if sm.SharedWithTeam != nil {
				shares[i].SharedWithTeam = &struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{
					ID:   sm.SharedWithTeam.String(),
					Name: sm.SharedWithTeamName,
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{"shares": shares, "total": len(shares)})
	}
}

// revokeMyCredentialShare removes a sharing entry.
func revokeMyCredentialShare(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		credID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential ID"})
			return
		}
		shareID, err := uuid.Parse(c.Param("shareId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid share ID"})
			return
		}

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		// Verify ownership of the credential
		owned, err := deps.Repos.UserCredential.VerifyOwnershipWithTenant(ctx, credID, *userID, tenantID)
		if err != nil || !owned {
			c.JSON(http.StatusNotFound, gin.H{"error": "credential not found or not owned by you"})
			return
		}

		if err := deps.Repos.CredentialShare.Delete(ctx, shareID, credID, *userID); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "revoke", "credential_share", shareID.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "share revoked"})
	}
}

// listSharedWithMe returns credentials that other users have shared with the
// current user (directly or via team membership).
func listSharedWithMe(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		credModels, err := deps.Repos.CredentialShare.ListSharedWithMe(ctx, *userID, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		creds := make([]SharedCredential, len(credModels))
		for i, cm := range credModels {
			creds[i] = SharedCredential{
				ID:          cm.ID.String(),
				Provider:    cm.Provider,
				ProviderURL: cm.ProviderURL,
				DisplayName: cm.DisplayName,
				TokenMasked: maskToken(cm.TokenEnc),
				Username:    cm.Username,
				Email:       cm.Email,
				OwnerName:   cm.OwnerName,
				OwnerEmail:  cm.OwnerEmail,
				CreatedAt:   cm.CreatedAt.Format(time.RFC3339),
			}
		}

		c.JSON(http.StatusOK, gin.H{"credentials": creds, "total": len(creds)})
	}
}

// GetSharedCredentialToken retrieves a shared credential's decrypted token.
// Used by handlers that need to act on behalf of the user with a shared credential.
func GetSharedCredentialToken(ctx context.Context, deps Dependencies, userID, tenantID uuid.UUID, provider, providerURL string) (token string, username string, email string, err error) {
	tokenEnc, username, email, err := deps.Repos.CredentialShare.GetSharedToken(ctx, userID, tenantID, provider, providerURL)
	if err != nil {
		return "", "", "", fmt.Errorf("no shared credential found for user %s, provider %s: %w", userID, provider, err)
	}

	decrypted, err := pepacrypto.Decrypt(tokenEnc)
	if err != nil {
		return "", "", "", fmt.Errorf("decrypt shared credential: %w", err)
	}
	return decrypted, username, email, nil
}
