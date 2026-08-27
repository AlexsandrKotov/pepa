package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	"github.com/pepa/pepa/internal/workflow"
	"github.com/pepa/pepa/pkg/models"
	"github.com/redis/go-redis/v9"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	log.Printf("PEPA Worker starting... (version=%s, built=%s)", version, buildTime)

	// Bootstrap all shared components
	comp, err := bootstrap.Bootstrap()
	if err != nil {
		log.Fatalf("Bootstrap failed: %v", err)
	}

	// Reconcile discovered plugins with the DB: only explicitly installed
	// plugins are active, so workflow/pipeline steps can't hit uninstalled ones.
	comp.AutoRegisterPlugins()

	// Start event bus
	comp.StartEventBus()

	// Initialize workflow engine
	wfEngine := workflow.NewEngine(comp.WorkflowRepo, comp.EntityRepo, comp.DeploymentRepo, comp.EventBus, comp.ProviderRegistry)

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
			runWorker(ctx, workerID, comp.Redis.Client, comp.DB, comp.EventBus, wfEngine, comp.EntityRepo, comp.JobQueue, comp.AIManager)
		}(i)
	}

	log.Printf("Started %d workers", numWorkers)

	// Start delayed job promoter
	wg.Add(1)
	go func() {
		defer wg.Done()
		runDelayedJobPromoter(ctx, comp.JobQueue)
	}()

	// Wait for shutdown signal
	<-done
	log.Println("Shutting down worker...")
	cancel()

	// Wait for workers to finish
	wg.Wait()
	comp.Shutdown(ctx)
	log.Println("PEPA Worker stopped")
}

func runWorker(ctx context.Context, id int, client *redis.Client, db *database.DB, bus *events.Bus, wfEngine *workflow.Engine, entityRepo *repository.EntityRepository, jobQueue *queue.Queue, aiManager *ai.Manager) {
	queueKey := "pepa:jobs"
	log.Printf("Worker %d started", id)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d stopping", id)
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
				log.Printf("Worker %d: queue error: %v", id, err)
				time.Sleep(time.Second)
				continue
			}

			var job queue.Job
			if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
				log.Printf("Worker %d: unmarshal job: %v", id, err)
				continue
			}

			log.Printf("Worker %d: processing job %s (type=%s)", id, job.ID, job.Type)
			if err := processJob(ctx, &job, db, bus, wfEngine, entityRepo, aiManager); err != nil {
				log.Printf("Worker %d: job %s failed: %v", id, job.ID, err)
				// Re-queue with exponential backoff if retries remain
				if job.Retries < 3 {
					job.Retries++
					backoff := time.Duration(job.Retries*job.Retries) * 10 * time.Second
					if enqueueErr := jobQueue.EnqueueJobWithDelay(&job, backoff); enqueueErr != nil {
						log.Printf("Worker %d: failed to requeue job %s: %v", id, job.ID, enqueueErr)
					} else {
						log.Printf("Worker %d: job %s requeued with %v backoff (retry %d/3)", id, job.ID, backoff, job.Retries)
					}
				} else {
					log.Printf("Worker %d: job %s exhausted retries, moving to dead letter", id, job.ID)
					if err := jobQueue.MoveToDeadLetter(&job); err != nil {
						log.Printf("Worker %d: failed to dead-letter job %s: %v", id, job.ID, err)
					}
				}
			} else {
				log.Printf("Worker %d: job %s completed", id, job.ID)
			}
		}
	}
}

func processJob(ctx context.Context, job *queue.Job, db *database.DB, bus *events.Bus, wfEngine *workflow.Engine, entityRepo *repository.EntityRepository, aiManager *ai.Manager) error {
	switch job.Type {
	case "entity.sync":
		return processEntitySync(ctx, job, db, bus, entityRepo)
	case "workflow.execute":
		return processWorkflowExecute(ctx, job, wfEngine)
	case "entity.index":
		return processEntityIndex(ctx, job, entityRepo, aiManager)
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

	log.Printf("Syncing entity %s (%s/%s)", entity.Name, entity.TypeKey, entity.Status)

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

	log.Printf("Executing workflow %s (execution %s)", workflowID, executionID)
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
		log.Printf("entity.index: AI manager not configured, skipping embedding for %s", entityIDStr)
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

	log.Printf("Entity %s indexed (embedding: %d dims, tokens: %d)", entityIDStr, len(resp.Vectors[0]), resp.TokensUsed)
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
				log.Printf("Delayed job promoter error: %v", err)
			}
		}
	}
}
