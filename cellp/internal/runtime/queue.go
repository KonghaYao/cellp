package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
)

// ErrQueueNotFound is returned when wrangler does not declare the queue (or its DLQ).
var ErrQueueNotFound = errors.New("queue_not_found")

// HasQueue reports whether wrangler declares the name as a producer queue,
// consumer queue, or consumers[].dead_letter_queue.
func HasQueue(projectDir, name string) bool {
	b, err := ParseBindings(projectDir)
	if err != nil {
		return false
	}
	for _, q := range b.Queues {
		if q.Name == name {
			return true
		}
		if q.DeadLetterQueue != nil && *q.DeadLetterQueue == name {
			return true
		}
	}
	// Plan: if DLQ is not on Binding, scan wrangler consumers[].dead_letter_queue.
	raw, err := readWranglerConfig(projectDir)
	if err != nil {
		return false
	}
	var cfg wranglerFile
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg.Queues == nil {
		return false
	}
	for _, c := range cfg.Queues.Consumers {
		if c.Queue == name {
			return true
		}
		if c.DeadLetterQueue != nil && *c.DeadLetterQueue == name {
			return true
		}
	}
	for _, p := range cfg.Queues.Producers {
		if p.Queue == name {
			return true
		}
	}
	return false
}

// DeclaredQueueNames returns wrangler queue names (producers, consumers, DLQs), de-duplicated.
func DeclaredQueueNames(projectDir string) []string {
	names := make([]string, 0)
	seen := make(map[string]struct{})
	add := func(n string) {
		if n == "" {
			return
		}
		if _, ok := seen[n]; ok {
			return
		}
		seen[n] = struct{}{}
		names = append(names, n)
	}
	b, err := ParseBindings(projectDir)
	if err != nil {
		return names
	}
	for _, q := range b.Queues {
		add(q.Name)
		if q.DeadLetterQueue != nil {
			add(*q.DeadLetterQueue)
		}
	}
	raw, err := readWranglerConfig(projectDir)
	if err != nil {
		return names
	}
	var cfg wranglerFile
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg.Queues == nil {
		return names
	}
	for _, p := range cfg.Queues.Producers {
		add(p.Queue)
	}
	for _, c := range cfg.Queues.Consumers {
		add(c.Queue)
		if c.DeadLetterQueue != nil {
			add(*c.DeadLetterQueue)
		}
	}
	return names
}

// QueueInfo runs `celld queue info --json`.
func (m *Manager) QueueInfo(ctx context.Context, project, version, projectDir, name string) (json.RawMessage, error) {
	return m.queueJSON(ctx, project, version, projectDir, name, []string{"info", name})
}

// QueuePeek runs `celld queue peek --limit n --json`.
func (m *Manager) QueuePeek(ctx context.Context, project, version, projectDir, name string, limit int) (json.RawMessage, error) {
	return m.queueJSON(ctx, project, version, projectDir, name, []string{
		"peek", name, "--limit", strconv.Itoa(limit),
	})
}

// QueuePurge runs `celld queue purge --force --json`. HTTP must validate force first.
func (m *Manager) QueuePurge(ctx context.Context, project, version, projectDir, name string) (json.RawMessage, error) {
	return m.queueJSON(ctx, project, version, projectDir, name, []string{"purge", name, "--force"})
}

// QueuePause runs `celld queue pause --json`.
func (m *Manager) QueuePause(ctx context.Context, project, version, projectDir, name string) (json.RawMessage, error) {
	return m.queueJSON(ctx, project, version, projectDir, name, []string{"pause", name})
}

// QueueResume runs `celld queue resume --json`.
func (m *Manager) QueueResume(ctx context.Context, project, version, projectDir, name string) (json.RawMessage, error) {
	return m.queueJSON(ctx, project, version, projectDir, name, []string{"resume", name})
}

// QueueRedrive runs `celld queue redrive --limit n --json`.
func (m *Manager) QueueRedrive(ctx context.Context, project, version, projectDir, name string, limit int) (json.RawMessage, error) {
	return m.queueJSON(ctx, project, version, projectDir, name, []string{
		"redrive", name, "--limit", strconv.Itoa(limit),
	})
}

func (m *Manager) queueJSON(ctx context.Context, project, version, projectDir, name string, argv []string) (json.RawMessage, error) {
	if !HasQueue(projectDir, name) {
		return nil, ErrQueueNotFound
	}
	args := append([]string{"queue"}, argv...)
	args = m.appendFleet(args, project, version, true)
	out, err := m.execCelld(ctx, project, version, args)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(bytes.TrimSpace(out)), nil
}
