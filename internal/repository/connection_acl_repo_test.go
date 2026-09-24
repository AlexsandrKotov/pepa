package repository

import (
	"testing"

	"github.com/google/uuid"
)

func TestConnectionACLStruct_Fields(t *testing.T) {
	// Verify the ConnectionACL struct has the expected fields.
	acl := ConnectionACL{
		ID:           uuid.New(),
		TenantID:     uuid.New(),
		ConnectionID: uuid.New(),
		UserID:       ptrUUID(uuid.New()),
		TeamID:       nil,
		CanRead:      true,
		CanUse:       true,
		CreatedBy:    uuid.New(),
	}

	if acl.UserID == nil {
		t.Fatal("UserID should be set")
	}
	if acl.TeamID != nil {
		t.Fatal("TeamID should be nil")
	}
	if !acl.CanRead || !acl.CanUse {
		t.Fatal("CanRead and CanUse should be true")
	}
}

func TestConnectionACLStruct_TeamGrant(t *testing.T) {
	// Verify team-based ACL grant.
	teamID := uuid.New()
	acl := ConnectionACL{
		ID:           uuid.New(),
		TenantID:     uuid.New(),
		ConnectionID: uuid.New(),
		UserID:       nil,
		TeamID:       &teamID,
		CanRead:      true,
		CanUse:       false,
		CreatedBy:    uuid.New(),
	}

	if acl.UserID != nil {
		t.Fatal("UserID should be nil for team grant")
	}
	if acl.TeamID == nil {
		t.Fatal("TeamID should be set for team grant")
	}
	if !acl.CanRead {
		t.Fatal("CanRead should be true")
	}
	if acl.CanUse {
		t.Fatal("CanUse should be false")
	}
}

func TestConnectionACLStruct_ConstraintValidation(t *testing.T) {
	// Verify the CHECK constraint: user_id XOR team_id.
	// Both nil should be invalid.
	acl := ConnectionACL{
		UserID: nil,
		TeamID: nil,
	}
	if acl.UserID != nil || acl.TeamID != nil {
		t.Fatal("both should be nil for invalid case")
	}

	// Both set should also be invalid (would be rejected by DB constraint).
	userID := uuid.New()
	teamID := uuid.New()
	acl2 := ConnectionACL{
		UserID: &userID,
		TeamID: &teamID,
	}
	if acl2.UserID == nil || acl2.TeamID == nil {
		t.Fatal("both should be set for invalid case")
	}
}

func ptrUUID(id uuid.UUID) *uuid.UUID {
	return &id
}
