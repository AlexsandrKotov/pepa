package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/pkg/models"
)

// isNotFound checks if an error indicates a "not found" result from the repository.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no rows")
}

// EntitySyncService handles automatic synchronization of discovered services into entities.
type EntitySyncService struct {
	entityRepo *repository.EntityRepository
}

// NewEntitySyncService creates a new entity sync service.
func NewEntitySyncService(entityRepo *repository.EntityRepository) *EntitySyncService {
	return &EntitySyncService{entityRepo: entityRepo}
}

// DiscoveredItem represents a service discovered from external sources.
type DiscoveredItem struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Cluster   string `json:"cluster"`
	Source    string `json:"source"` // "argocd", "fluxcd", "kubernetes", "docker", "pepa"
	Status    string `json:"status"`
	Health    string `json:"health"`
	Image     string `json:"image"`
}

// SyncResult holds the result of a sync operation.
type SyncResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Total   int `json:"total"`
}

// SyncDiscoveryToEntities syncs discovered services into the entities table.
// For each discovered service, it creates or updates an entity with external_id
// set to "{source}:{cluster}:{namespace}:{name}" for deduplication.
func (s *EntitySyncService) SyncDiscoveryToEntities(ctx context.Context, tenantID, orgID uuid.UUID, items []DiscoveredItem) (*SyncResult, error) {
	result := &SyncResult{Total: len(items)}

	for _, item := range items {
		externalID := fmt.Sprintf("%s:%s:%s:%s", item.Source, item.Cluster, item.Namespace, item.Name)

		// Build metadata from discovered service info
		metadata := map[string]interface{}{
			"source":    item.Source,
			"cluster":   item.Cluster,
			"namespace": item.Namespace,
			"health":    item.Health,
		}
		if item.Image != "" {
			metadata["image"] = item.Image
		}
		if item.Status != "" {
			metadata["discovery_status"] = item.Status
		}

		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			slog.Warn("entity sync: failed to marshal metadata", "name", item.Name, "error", err)
			result.Skipped++
			continue
		}

		// Check if entity already exists by external_id
		existing, err := s.entityRepo.GetByExternalID(ctx, externalID, "service", tenantID)
		if err != nil {
			if !isNotFound(err) {
				// Real DB error — skip this item
				slog.Warn("entity sync: lookup failed", "external_id", externalID, "error", err)
				result.Skipped++
				continue
			}
			// Not found — create new entity
			createReq := models.CreateEntityRequest{
				TypeKey:    "service",
				Name:       item.Name,
				ExternalID: externalID,
				Metadata:   metadataJSON,
			}
			if item.Namespace != "" {
				createReq.Description = fmt.Sprintf("Discovered from %s in %s/%s", item.Source, item.Cluster, item.Namespace)
			}

			_, createErr := s.entityRepo.Create(ctx, createReq, tenantID, orgID, nil)
			if createErr != nil {
				slog.Warn("entity sync: failed to create entity", "name", item.Name, "error", createErr)
				result.Skipped++
				continue
			}
			result.Created++
		} else {
			// Found — update metadata
			name := existing.Name
			desc := existing.Description
			updateReq := models.UpdateEntityRequest{
				Name:        &name,
				Description: &desc,
				Metadata:    metadataJSON,
			}
			_, updateErr := s.entityRepo.Update(ctx, existing.ID, updateReq, nil, tenantID)
			if updateErr != nil {
				slog.Warn("entity sync: failed to update entity", "name", item.Name, "error", updateErr)
				result.Skipped++
				continue
			}
			result.Updated++
		}
	}

	slog.Info("entity sync complete",
		"tenant_id", tenantID,
		"total", result.Total,
		"created", result.Created,
		"updated", result.Updated,
		"skipped", result.Skipped,
	)

	return result, nil
}

// SyncServiceToEntity creates or updates an entity when a PEPA Service is created/updated.
func (s *EntitySyncService) SyncServiceToEntity(ctx context.Context, tenantID, orgID uuid.UUID, serviceID uuid.UUID, name, description, namespace, status string, metadata map[string]interface{}) error {
	externalID := fmt.Sprintf("pepa:service:%s", serviceID.String())

	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["source"] = "pepa"
	metadata["service_id"] = serviceID.String()
	if namespace != "" {
		metadata["namespace"] = namespace
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	existing, err := s.entityRepo.GetByExternalID(ctx, externalID, "service", tenantID)
	if err != nil {
		if !isNotFound(err) {
			return fmt.Errorf("lookup entity by external_id: %w", err)
		}
		// Not found — create
		createReq := models.CreateEntityRequest{
			TypeKey:     "service",
			Name:        name,
			Description: description,
			ExternalID:  externalID,
			Metadata:    metadataJSON,
		}
		_, createErr := s.entityRepo.Create(ctx, createReq, tenantID, orgID, nil)
		if createErr != nil {
			return fmt.Errorf("create entity from service: %w", createErr)
		}
		return nil
	}

	// Update existing
	updateReq := models.UpdateEntityRequest{
		Name:        &name,
		Description: &description,
		Metadata:    metadataJSON,
		Status:      &status,
	}
	_, err = s.entityRepo.Update(ctx, existing.ID, updateReq, nil, tenantID)
	if err != nil {
		return fmt.Errorf("update entity from service: %w", err)
	}
	return nil
}

// GetSyncStatus returns the sync status summary for a tenant.
func (s *EntitySyncService) GetSyncStatus(ctx context.Context, tenantID uuid.UUID) (map[string]interface{}, error) {
	// Count total entities and synced entities
	filter := models.EntityFilter{TenantID: tenantID, PerPage: 1}
	listResult, err := s.entityRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("count entities: %w", err)
	}

	syncFilter := models.EntityFilter{TenantID: tenantID, PerPage: 1}
	// We can't easily filter by sync_status in the current filter, so return basic info
	_ = syncFilter

	return map[string]interface{}{
		"total_entities":  listResult.Total,
		"last_synced_at":  time.Now().UTC(), // Will be updated by actual sync operations
		"status":          "ok",
	}, nil
}
