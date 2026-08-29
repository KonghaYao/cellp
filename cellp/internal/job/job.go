package job

import (
	"context"

	"github.com/cellp/cellp/internal/registry"
)

// DeployJob is enqueued when a new version is submitted.
type DeployJob struct {
	ProjectID string
	VersionID string
	Step      string
}

// Queue persists and notifies orchestrator workers.
type Queue interface {
	Enqueue(ctx context.Context, j *DeployJob) error
}

// SQLiteQueue persists jobs via registry.Store and notifies workers.
type SQLiteQueue struct {
	store  registry.Store
	notify chan struct{}
}

// NewSQLiteQueue creates a queue backed by the registry store.
func NewSQLiteQueue(store registry.Store) *SQLiteQueue {
	return &SQLiteQueue{
		store:  store,
		notify: make(chan struct{}, 64),
	}
}

// Enqueue persists a deploy job and notifies waiting workers.
func (q *SQLiteQueue) Enqueue(ctx context.Context, j *DeployJob) error {
	if _, err := q.store.EnqueueJob(ctx, j.ProjectID, j.VersionID, j.Step); err != nil {
		return err
	}
	select {
	case q.notify <- struct{}{}:
	default:
	}
	return nil
}

// Notify returns a channel signaled when new jobs are enqueued.
func (q *SQLiteQueue) Notify() <-chan struct{} {
	return q.notify
}
