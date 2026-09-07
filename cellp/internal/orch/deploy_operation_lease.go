package orch

import (
	"context"
	"sync"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

var deployOperationLeaseTick = time.Minute

func (o *Orchestrator) renewDeployWorkerLeases(ctx context.Context, j *registry.Job, workerID string, lease time.Duration) error {
	if j == nil || j.ID == "" {
		return nil
	}
	if err := o.store.RenewClaimedJobLease(ctx, workerID, deployAttempt(j), lease); err != nil {
		return err
	}
	return o.store.RenewVersionDeployOperation(ctx, j.ProjectID, j.VersionID, deployAttempt(j), lease)
}

// startDeployWorkerLeaseKeeper renews job worker lease and deploy operation until stop is called.
// Failed renewal cancels ctx so in-flight deploy work stops issuing side effects.
func (o *Orchestrator) startDeployWorkerLeaseKeeper(parent context.Context, j *registry.Job, workerID string, lease time.Duration) (context.Context, func()) {
	if j == nil || j.ID == "" {
		return parent, func() {}
	}
	if lease <= 0 {
		lease = jobLease
	}
	ctx, cancel := context.WithCancel(parent)
	var once sync.Once
	var wg sync.WaitGroup
	cancelLoop := func() { once.Do(cancel) }
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := o.renewDeployWorkerLeases(ctx, j, workerID, lease); err != nil {
			cancelLoop()
			return
		}
		ticker := time.NewTicker(deployOperationLeaseTick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := o.renewDeployWorkerLeases(ctx, j, workerID, lease); err != nil {
					cancelLoop()
					return
				}
			}
		}
	}()
	stop := func() {
		cancelLoop()
		wg.Wait()
	}
	return ctx, stop
}

// withCompensatingJobLeases renews compensating job worker lease and deploy operation for the job attempt.
func (o *Orchestrator) withCompensatingJobLeases(parent context.Context, j *registry.Job, workerID string, lease time.Duration, fn func(context.Context) error) error {
	if j == nil || j.ID == "" {
		return fn(parent)
	}
	if lease <= 0 {
		lease = jobLease
	}
	ctx, cancel := context.WithCancel(parent)
	var wg sync.WaitGroup
	cancelLoop := func() { cancel() }
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := o.renewDeployWorkerLeases(ctx, j, workerID, lease); err != nil {
			cancelLoop()
			return
		}
		ticker := time.NewTicker(deployOperationLeaseTick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := o.renewDeployWorkerLeases(ctx, j, workerID, lease); err != nil {
					cancelLoop()
					return
				}
			}
		}
	}()
	err := fn(ctx)
	cancel()
	wg.Wait()
	return err
}
