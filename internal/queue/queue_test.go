package queue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupTestQueue(t *testing.T) (*Queue, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return New(client), mr, client
}

func TestEnqueue_Dequeue(t *testing.T) {
	q, mr, client := setupTestQueue(t)
	defer mr.Close()

	err := q.Enqueue("test.job", "tenant-1", map[string]interface{}{"key": "value"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// Check queue length
	ctx := context.Background()
	length, _ := client.LLen(ctx, "pepa:jobs").Result()
	if length != 1 {
		t.Fatalf("expected 1 job in queue, got %d", length)
	}

	// Pop and verify
	data, _ := client.LPop(ctx, "pepa:jobs").Result()
	var job Job
	if err := json.Unmarshal([]byte(data), &job); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if job.Type != "test.job" {
		t.Errorf("expected type test.job, got %s", job.Type)
	}
	if job.TenantID != "tenant-1" {
		t.Errorf("expected tenant tenant-1, got %s", job.TenantID)
	}
	if job.ID == "" {
		t.Error("expected non-empty job ID")
	}
	if job.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

func TestEnqueueWithDelay(t *testing.T) {
	q, mr, client := setupTestQueue(t)
	defer mr.Close()

	err := q.EnqueueWithDelay("delayed.job", "tenant-1", map[string]interface{}{"x": 1}, 10*time.Second)
	if err != nil {
		t.Fatalf("enqueue with delay: %v", err)
	}

	ctx := context.Background()
	// Should be in the delayed sorted set, not the main queue
	mainLen, _ := client.LLen(ctx, "pepa:jobs").Result()
	if mainLen != 0 {
		t.Error("expected 0 jobs in main queue")
	}
	delayed := client.ZCard(ctx, "pepa:jobs:delayed").Val()
	if delayed != 1 {
		t.Errorf("expected 1 delayed job, got %d", delayed)
	}
}

func TestEnqueueJob(t *testing.T) {
	q, mr, client := setupTestQueue(t)
	defer mr.Close()

	job := &Job{
		ID:        "custom-id",
		Type:      "retry.job",
		Payload:   map[string]interface{}{"retry": true},
		TenantID:  "tenant-2",
		CreatedAt: time.Now().UTC(),
		Retries:   2,
	}

	if err := q.EnqueueJob(job); err != nil {
		t.Fatalf("enqueue job: %v", err)
	}

	ctx := context.Background()
	data, _ := client.LPop(ctx, "pepa:jobs").Result()
	var decoded Job
	json.Unmarshal([]byte(data), &decoded)
	if decoded.ID != "custom-id" {
		t.Errorf("expected ID custom-id, got %s", decoded.ID)
	}
	if decoded.Retries != 2 {
		t.Errorf("expected 2 retries, got %d", decoded.Retries)
	}
}

func TestMoveToDeadLetter(t *testing.T) {
	q, mr, client := setupTestQueue(t)
	defer mr.Close()

	job := &Job{ID: "dead-job", Type: "fail.job"}
	if err := q.MoveToDeadLetter(job); err != nil {
		t.Fatalf("move to dead letter: %v", err)
	}

	ctx := context.Background()
	length, _ := client.LLen(ctx, "pepa:jobs:dead").Result()
	if length != 1 {
		t.Errorf("expected 1 dead letter job, got %d", length)
	}
}

func TestPromoteDelayedJobs(t *testing.T) {
	q, mr, client := setupTestQueue(t)
	defer mr.Close()

	ctx := context.Background()

	// Add a delayed job with score in the past (should be promoted)
	job := &Job{ID: "promote-me", Type: "delayed.job", CreatedAt: time.Now().UTC()}
	data, _ := json.Marshal(job)
	mr.ZAdd("pepa:jobs:delayed", float64(time.Now().Add(-10*time.Second).Unix()), string(data))

	// Add a delayed job with score in the future (should NOT be promoted)
	futureJob := &Job{ID: "wait-me", Type: "future.job", CreatedAt: time.Now().UTC()}
	futureData, _ := json.Marshal(futureJob)
	mr.ZAdd("pepa:jobs:delayed", float64(time.Now().Add(1*time.Hour).Unix()), string(futureData))

	if err := q.PromoteDelayedJobs(ctx); err != nil {
		t.Fatalf("promote: %v", err)
	}

	// The past job should be in the main queue
	mainLen, _ := client.LLen(ctx, "pepa:jobs").Result()
	if mainLen != 1 {
		t.Errorf("expected 1 job in main queue, got %d", mainLen)
	}
	// The future job should still be delayed
	delayed := client.ZCard(ctx, "pepa:jobs:delayed").Val()
	if delayed != 1 {
		t.Errorf("expected 1 delayed job remaining, got %d", delayed)
	}
}

func TestJobRetries(t *testing.T) {
	job := &Job{
		ID:      "retry-test",
		Type:    "test",
		Retries: 0,
	}
	if job.Retries != 0 {
		t.Errorf("expected 0 retries, got %d", job.Retries)
	}
	job.Retries++
	if job.Retries != 1 {
		t.Errorf("expected 1 retry, got %d", job.Retries)
	}
}
