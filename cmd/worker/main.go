package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/ai"
	"github.com/pepa/pepa/internal/bootstrap"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/internal/events"
	"github.com/pepa/pepa/internal/queue"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/service"
	"github.com/pepa/pepa/internal/workflow"
	"github.com/pepa/pepa/pkg/models"
	"github.com/redis/go-redis/v9"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	slog.Info("PEPA Worker starting", "version", version, "build_time", buildTime)

	// Bootstrap all shared components
	comp, err := bootstrap.Bootstrap()
	if err != nil {
		slog.Error("bootstrap failed", "error", err)
		os.Exit(1)
	}

	// Start event bus
	// (AutoRegisterPlugins runs asynchronously inside Bootstrap after plugin discovery)
	comp.StartEventBus()

	// Initialize deployment service for real workflow deployments
	deploymentSvc := service.NewDeploymentService(comp.ClusterRepo, comp.DeploymentRepo, comp.HelmRepo)
	// Wire up deployment event recorder for timeline
	if comp.DB != nil {
		deploymentSvc.SetEventRecorder(func(deploymentID uuid.UUID, eventType, message string) {
			_, _ = comp.DB.Pool.Exec(context.Background(),
				`INSERT INTO deployment_events (deployment_id, event_type, message) VALUES ($1, $2, $3)`,
				deploymentID, eventType, message)
		})
	}

	// Initialize workflow engine
	wfEngine := workflow.NewEngine(comp.WorkflowRepo, comp.EntityRepo, comp.DeploymentRepo, deploymentSvc, comp.EventBus, comp.ProviderRegistry)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	// Start workers
	var wg sync.WaitGroup
	numWorkers := comp.Config.Worker.Concurrency
	if numWorkers < 1 {
		numWorkers = 3
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			runWorker(ctx, workerID, comp.Redis.Client, comp.DB, comp.EventBus, wfEngine, comp.EntityRepo, comp.JobQueue, comp.AIManager, deploymentSvc)
		}(i)
	}

	slog.Info("workers started", "count", numWorkers)

	// Start delayed job promoter
	wg.Add(1)
	go func() {
		defer wg.Done()
		runDelayedJobPromoter(ctx, comp.JobQueue)
	}()

	// Wait for shutdown signal
	<-done
	slog.Info("shutting down worker")
	cancel()

	// Wait for workers to finish
	wg.Wait()
	comp.Shutdown(ctx)
	slog.Info("PEPA Worker stopped")
}

func runWorker(ctx context.Context, id int, client *redis.Client, db *database.DB, bus *events.Bus, wfEngine *workflow.Engine, entityRepo *repository.EntityRepository, jobQueue *queue.Queue, aiManager *ai.Manager, deploymentSvc *service.DeploymentService) {
	queueKey := "pepa:jobs"
	slog.Info("worker started", "id", id)

	for {
		select {
		case <-ctx.Done():
			slog.Info("worker stopping", "id", id)
			return
		default:
			// Block-pop from queue with 5s timeout
			result, err := client.BRPop(ctx, 5*time.Second, queueKey).Result()
			if err != nil {
				if err == redis.Nil {
					continue // No jobs, timeout
				}
				if ctx.Err() != nil {
					return // Shutting down
				}
				slog.Error("queue error", "id", id, "error", err)
				time.Sleep(time.Second)
				continue
			}

			var job queue.Job
			if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
				slog.Error("failed to unmarshal job", "id", id, "error", err)
				continue
			}

			slog.Info("processing job", "id", id, "job", job.ID, "type", job.Type)
			if err := processJob(ctx, &job, db, bus, wfEngine, entityRepo, aiManager, deploymentSvc); err != nil {
				slog.Error("job failed", "id", id, "job", job.ID, "error", err)
				// Re-queue with exponential backoff if retries remain
				if job.Retries < 3 {
					job.Retries++
					backoff := time.Duration(job.Retries*job.Retries) * 10 * time.Second
					if enqueueErr := jobQueue.EnqueueJobWithDelay(&job, backoff); enqueueErr != nil {
						slog.Error("failed to requeue job", "id", id, "job", job.ID, "error", enqueueErr)
					} else {
						slog.Info("job requeued with backoff", "id", id, "job", job.ID, "backoff", backoff, "retry", job.Retries)
					}
				} else {
					slog.Warn("job exhausted retries, moving to dead letter", "id", id, "job", job.ID)
					if err := jobQueue.MoveToDeadLetter(&job); err != nil {
						slog.Error("failed to dead-letter job", "id", id, "job", job.ID, "error", err)
					}
				}
			} else {
				slog.Info("job completed", "id", id, "job", job.ID)
			}
		}
	}
}

func processJob(ctx context.Context, job *queue.Job, db *database.DB, bus *events.Bus, wfEngine *workflow.Engine, entityRepo *repository.EntityRepository, aiManager *ai.Manager, deploymentSvc *service.DeploymentService) error {
	switch job.Type {
	case "entity.sync":
		return processEntitySync(ctx, job, db, bus, entityRepo)
	case "workflow.execute":
		return processWorkflowExecute(ctx, job, wfEngine)
	case "entity.index":
		return processEntityIndex(ctx, job, entityRepo, aiManager)
	case "deployment.execute":
		return processDeploymentExecute(ctx, job, deploymentSvc)
	default:
		return fmt.Errorf("unknown job type: %s", job.Type)
	}
}

func processEntitySync(ctx context.Context, job *queue.Job, db *database.DB, bus *events.Bus, entityRepo *repository.EntityRepository) error {
	entityIDStr, ok := job.Payload["entity_id"].(string)
	if !ok || entityIDStr == "" {
		return fmt.Errorf("entity.sync: missing entity_id in payload")
	}

	entityID, err := uuid.Parse(entityIDStr)
	if err != nil {
		return fmt.Errorf("entity.sync: invalid entity_id: %w", err)
	}

	// Load entity
	entity, err := entityRepo.Get(ctx, entityID, uuid.Nil)
	if err != nil {
		return fmt.Errorf("entity.sync: load entity: %w", err)
	}

	slog.Info("syncing entity", "entity", entity.Name, "type", entity.TypeKey, "status", entity.Status)

	// Update sync status
	_, err = entityRepo.Update(ctx, entityID, models.UpdateEntityRequest{
		Status: strPtr(entity.Status),
	}, nil, uuid.Nil)
	if err != nil {
		return fmt.Errorf("entity.sync: update sync status: %w", err)
	}

	return bus.Publish(events.Event{
		Type:     "entity.synced",
		TenantID: job.TenantID,
		EntityID: entityIDStr,
		Payload: map[string]interface{}{
			"entity_id": entityIDStr,
			"type_key":  entity.TypeKey,
			"name":      entity.Name,
			"status":    entity.Status,
		},
	})
}

func processWorkflowExecute(ctx context.Context, job *queue.Job, wfEngine *workflow.Engine) error {
	workflowIDStr, ok := job.Payload["workflow_id"].(string)
	if !ok || workflowIDStr == "" {
		return fmt.Errorf("workflow.execute: missing workflow_id in payload")
	}
	executionIDStr, ok := job.Payload["execution_id"].(string)
	if !ok || executionIDStr == "" {
		return fmt.Errorf("workflow.execute: missing execution_id in payload")
	}

	workflowID, err := uuid.Parse(workflowIDStr)
	if err != nil {
		return fmt.Errorf("workflow.execute: invalid workflow_id: %w", err)
	}
	executionID, err := uuid.Parse(executionIDStr)
	if err != nil {
		return fmt.Errorf("workflow.execute: invalid execution_id: %w", err)
	}

	slog.Info("executing workflow", "workflow", workflowID, "execution", executionID)
	return wfEngine.Execute(ctx, workflowID, executionID)
}

func processEntityIndex(ctx context.Context, job *queue.Job, entityRepo *repository.EntityRepository, aiManager *ai.Manager) error {
	entityIDStr, ok := job.Payload["entity_id"].(string)
	if !ok {
		return fmt.Errorf("entity.index: missing entity_id")
	}

	entityID, err := uuid.Parse(entityIDStr)
	if err != nil {
		return fmt.Errorf("entity.index: invalid entity_id: %w", err)
	}

	if aiManager == nil {
		slog.Info("entity.index: AI manager not configured, skipping", "entity", entityIDStr)
		return nil
	}

	// Load entity to build embedding text
	entity, err := entityRepo.Get(ctx, entityID, uuid.Nil)
	if err != nil {
		return fmt.Errorf("entity.index: load entity: %w", err)
	}

	// Build text to embed: name + description + type
	text := entity.Name
	if entity.Description != "" {
		text += " " + entity.Description
	}
	text += " " + entity.TypeKey

	// Get default provider and generate embedding
	provider, err := aiManager.DefaultProvider()
	if err != nil {
		return fmt.Errorf("entity.index: no AI provider available: %w", err)
	}

	resp, err := provider.Embed(ctx, []string{text}, nil)
	if err != nil {
		return fmt.Errorf("entity.index: embedding failed: %w", err)
	}
	if len(resp.Vectors) == 0 {
		return fmt.Errorf("entity.index: embedding returned no vectors")
	}

	// Store embedding
	if err := entityRepo.UpdateEmbedding(ctx, entityID, resp.Vectors[0]); err != nil {
		return fmt.Errorf("entity.index: store embedding: %w", err)
	}

	slog.Info("entity indexed", "entity", entityIDStr, "dims", len(resp.Vectors[0]), "tokens", resp.TokensUsed)
	return nil
}

func processDeploymentExecute(ctx context.Context, job *queue.Job, deploymentSvc *service.DeploymentService) error {
	if deploymentSvc == nil {
		return fmt.Errorf("deployment.execute: deployment service not available")
	}

	deploymentIDStr, ok := job.Payload["deployment_id"].(string)
	if !ok || deploymentIDStr == "" {
		return fmt.Errorf("deployment.execute: missing deployment_id in payload")
	}
	clusterIDStr, ok := job.Payload["cluster_id"].(string)
	if !ok || clusterIDStr == "" {
		return fmt.Errorf("deployment.execute: missing cluster_id in payload")
	}

	deploymentID, err := uuid.Parse(deploymentIDStr)
	if err != nil {
		return fmt.Errorf("deployment.execute: invalid deployment_id: %w", err)
	}
	clusterID, err := uuid.Parse(clusterIDStr)
	if err != nil {
		return fmt.Errorf("deployment.execute: invalid cluster_id: %w", err)
	}

	namespace, _ := job.Payload["namespace"].(string)
	releaseName, _ := job.Payload["release_name"].(string)
	replicasFloat, _ := job.Payload["replicas"].(float64)
	replicas := int32(replicasFloat)
	if replicas <= 0 {
		replicas = 1
	}
	timeoutSeconds := 300
	if ts, ok := job.Payload["timeout_seconds"].(float64); ok && ts > 0 {
		timeoutSeconds = int(ts)
	}

	// Parse spec from payload
	var specJSON json.RawMessage
	if specRaw, ok := job.Payload["spec"]; ok {
		specJSON, _ = json.Marshal(specRaw)
	}

	slog.Info("executing deployment", "deployment", deploymentID, "cluster", clusterID, "namespace", namespace)

	result := deploymentSvc.PerformDeployment(ctx, deploymentID, clusterID, namespace, releaseName, replicas, specJSON, timeoutSeconds)
	if !result.Success {
		return fmt.Errorf("deployment failed: %s", result.Message)
	}

	// Publish deployment.succeeded event
	if bus := job.Payload; bus != nil {
		slog.Info("deployment executed successfully", "deployment", deploymentID)
	}

	return nil
}

func strPtr(s string) *string {
	return &s
}

// runDelayedJobPromoter periodically moves delayed jobs to the main queue.
func runDelayedJobPromoter(ctx context.Context, jobQueue *queue.Queue) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := jobQueue.PromoteDelayedJobs(ctx); err != nil {
				slog.Error("delayed job promoter error", "error", err)
			}
		}
	}
}
