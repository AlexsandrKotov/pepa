package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Job represents a background job.
type Job struct {
	ID             string                 `json:"id"`
	Type           string                 `json:"type"`
	Payload        map[string]interface{} `json:"payload"`
	TenantID       string                 `json:"tenant_id"`
	IdempotencyKey string                 `json:"idempotency_key,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	Retries        int                    `json:"retries"`
}

// Queue is a Redis-backed job queue.
type Queue struct {
	client *redis.Client
	key    string
}

// dedupTTL is the window during which duplicate jobs are rejected.
const dedupTTL = 60 * time.Second

// New creates a new job queue.
func New(client *redis.Client) *Queue {
	return &Queue{
		client: client,
		key:    "pepa:jobs",
	}
}

// Enqueue adds a job to the queue.
func (q *Queue) Enqueue(jobType, tenantID string, payload map[string]interface{}) error {
	return q.enqueueWithKey(jobType, tenantID, "", payload, 0)
}

// EnqueueUnique adds a job to the queue with an idempotency key. If a job
// with the same key was enqueued within the dedup window (60 s), the call
// is a no-op and returns nil.
func (q *Queue) EnqueueUnique(jobType, tenantID string, payload map[string]interface{}, idempotencyKey string) error {
	return q.enqueueWithKey(jobType, tenantID, idempotencyKey, payload, 0)
}

// enqueueWithKey is the shared implementation for all Enqueue variants.
// A non-empty idempotencyKey triggers a SETNX guard with dedupTTL seconds.
// A delay > 0 routes the job through the delayed sorted set.
func (q *Queue) enqueueWithKey(jobType, tenantID, idempotencyKey string, payload map[string]interface{}, delay time.Duration) error {
	ctx := context.Background()

	// Deduplication guard
	if idempotencyKey != "" {
		ok, err := q.client.SetNX(ctx, q.key+":dedup:"+idempotencyKey, 1, dedupTTL).Result()
		if err != nil {
			return fmt.Errorf("dedup check: %w", err)
		}
		if !ok {
			return nil // duplicate — silently skip
		}
	}

	job := Job{
		ID:             uuid.New().String(),
		Type:           jobType,
		Payload:        payload,
		TenantID:       tenantID,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      time.Now().UTC(),
	}

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}

	if delay > 0 {
		score := float64(time.Now().Add(delay).Unix())
		return q.client.ZAdd(ctx, q.key+":delayed",
			redis.Z{Score: score, Member: data}).Err()
	}
	return q.client.LPush(ctx, q.key, data).Err()
}

// EnqueueWithDelay adds a job with a delay (uses Redis sorted set).
func (q *Queue) EnqueueWithDelay(jobType, tenantID string, payload map[string]interface{}, delay time.Duration) error {
	return q.enqueueWithKey(jobType, tenantID, "", payload, delay)
}

// EnqueueWithDelayUnique adds a delayed job with an idempotency key.
func (q *Queue) EnqueueWithDelayUnique(jobType, tenantID string, payload map[string]interface{}, delay time.Duration, idempotencyKey string) error {
	return q.enqueueWithKey(jobType, tenantID, idempotencyKey, payload, delay)
}

// EnqueueJob adds an existing Job struct to the queue (used for retries).
func (q *Queue) EnqueueJob(job *Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	return q.client.LPush(context.Background(), q.key, data).Err()
}

// EnqueueJobWithDelay adds an existing Job struct with a delay.
func (q *Queue) EnqueueJobWithDelay(job *Job, delay time.Duration) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	score := float64(time.Now().Add(delay).Unix())
	return q.client.ZAdd(context.Background(), q.key+":delayed",
		redis.Z{Score: score, Member: data}).Err()
}

// MoveToDeadLetter moves a job to the dead letter queue.
func (q *Queue) MoveToDeadLetter(job *Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	return q.client.LPush(context.Background(), q.key+":dead", data).Err()
}

// promoteScript atomically moves delayed jobs whose scheduled time has passed
// from the sorted set to the main list. This prevents duplicate promotion
// when multiple promoter goroutines run concurrently.
var promoteScript = redis.NewScript(`
local delayedKey = KEYS[1]
local queueKey   = KEYS[2]
local now        = ARGV[1]

local jobs = redis.call('ZRANGEBYSCORE', delayedKey, '-inf', now)
local count = 0
for _, data in ipairs(jobs) do
	redis.call('LPUSH', queueKey, data)
	redis.call('ZREM', delayedKey, data)
	count = count + 1
end
return count
`)

// PromoteDelayedJobs moves jobs from the delayed sorted set to the main queue
// when their scheduled time has arrived. Call this periodically.
func (q *Queue) PromoteDelayedJobs(ctx context.Context) error {
	now := fmt.Sprintf("%f", float64(time.Now().Unix()))

	result, err := promoteScript.Run(ctx, q.client, []string{q.key + ":delayed", q.key}, now).Int()
	if err != nil {
		return fmt.Errorf("promote delayed jobs: %w", err)
	}

	if result > 0 {
		log.Printf("queue: promoted %d delayed job(s)", result)
	}
	return nil
}
