package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/pkg/models"
)

// ScorecardEvalService handles automatic scorecard evaluation.
type ScorecardEvalService struct {
	scorecardRepo *repository.ScorecardRepository
	entityRepo    *repository.EntityRepository
}

// NewScorecardEvalService creates a new scorecard evaluation service.
func NewScorecardEvalService(scorecardRepo *repository.ScorecardRepository, entityRepo *repository.EntityRepository) *ScorecardEvalService {
	return &ScorecardEvalService{
		scorecardRepo: scorecardRepo,
		entityRepo:    entityRepo,
	}
}

// AutoEvaluateAll evaluates all enabled scorecards against all active entities for a tenant.
func (s *ScorecardEvalService) AutoEvaluateAll(ctx context.Context, tenantID uuid.UUID) (int, error) {
	// Get all enabled scorecards
	scorecards, err := s.scorecardRepo.ListScorecards(ctx, tenantID)
	if err != nil {
		return 0, err
	}

	enabled := make([]models.Scorecard, 0)
	for _, sc := range scorecards {
		if sc.Enabled {
			enabled = append(enabled, sc)
		}
	}

	if len(enabled) == 0 {
		return 0, nil
	}

	// Paginate through all active entities
	filter := models.EntityFilter{
		TenantID: tenantID,
		Status:   "active",
		PerPage:  500,
		Page:     1,
	}

	totalEvaluated := 0
	totalEntities := 0

	for {
		entityList, err := s.entityRepo.List(ctx, filter)
		if err != nil {
			return totalEvaluated, fmt.Errorf("list entities page %d: %w", filter.Page, err)
		}
		if len(entityList.Items) == 0 {
			break
		}

		for _, sc := range enabled {
			for _, entity := range entityList.Items {
				_, err := s.scorecardRepo.EvaluateEntity(ctx, sc.ID, entity.ID, tenantID)
				if err != nil {
					slog.Warn("auto-evaluate: failed", "scorecard", sc.Name, "entity", entity.Name, "error", err)
					continue
				}
				totalEvaluated++
			}
		}
		totalEntities += len(entityList.Items)

		// Check if we've processed all pages
		if filter.Page >= entityList.TotalPages {
			break
		}
		filter.Page++
	}

	slog.Info("auto-evaluation complete",
		"tenant_id", tenantID,
		"scorecards", len(enabled),
		"entities", totalEntities,
		"evaluations", totalEvaluated,
	)

	return totalEvaluated, nil
}

// EvaluateEntity evaluates all enabled scorecards for a single entity.
func (s *ScorecardEvalService) EvaluateEntity(ctx context.Context, entityID, tenantID uuid.UUID) (int, error) {
	scorecards, err := s.scorecardRepo.ListScorecards(ctx, tenantID)
	if err != nil {
		return 0, err
	}

	evaluated := 0
	for _, sc := range scorecards {
		if !sc.Enabled {
			continue
		}
		_, err := s.scorecardRepo.EvaluateEntity(ctx, sc.ID, entityID, tenantID)
		if err != nil {
			slog.Warn("evaluate entity: failed", "scorecard", sc.Name, "entity_id", entityID, "error", err)
			continue
		}
		evaluated++
	}

	return evaluated, nil
}
