package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestQueuePeekArgv(t *testing.T) {
	argsLog, _ := installFakeOperatorCelld(t)
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if _, err := m.QueuePeek(context.Background(), "demo", "v1", projectDir, "tasks", 10); err != nil {
		t.Fatalf("QueuePeek: %v", err)
	}
	want := append([]string{"queue", "peek", "tasks", "--limit", "10"}, fleetJSON()...)
	assertArgv(t, argsLog, want)
}

func TestQueuePurgeArgv(t *testing.T) {
	argsLog, _ := installFakeOperatorCelld(t)
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if _, err := m.QueuePurge(context.Background(), "demo", "v1", projectDir, "tasks"); err != nil {
		t.Fatalf("QueuePurge: %v", err)
	}
	want := append([]string{"queue", "purge", "tasks", "--force"}, fleetJSON()...)
	assertArgv(t, argsLog, want)
	got := readArgv(t, argsLog)
	hasForce, hasJSON := false, false
	for _, a := range got {
		if a == "--force" {
			hasForce = true
		}
		if a == "--json" {
			hasJSON = true
		}
	}
	if !hasForce || !hasJSON {
		t.Fatalf("argv missing --force or --json: %q", got)
	}
}

func TestQueueRedriveArgv(t *testing.T) {
	argsLog, _ := installFakeOperatorCelld(t)
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if _, err := m.QueueRedrive(context.Background(), "demo", "v1", projectDir, "tasks", 100); err != nil {
		t.Fatalf("QueueRedrive: %v", err)
	}
	want := append([]string{"queue", "redrive", "tasks", "--limit", "100"}, fleetJSON()...)
	assertArgv(t, argsLog, want)
}

func TestQueuePauseResumeInfoArgv(t *testing.T) {
	argsLog, _ := installFakeOperatorCelld(t)
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	ctx := context.Background()

	if _, err := m.QueuePause(ctx, "demo", "v1", projectDir, "tasks"); err != nil {
		t.Fatalf("QueuePause: %v", err)
	}
	assertArgv(t, argsLog, append([]string{"queue", "pause", "tasks"}, fleetJSON()...))
	resetArgv(t, argsLog)

	if _, err := m.QueueResume(ctx, "demo", "v1", projectDir, "tasks"); err != nil {
		t.Fatalf("QueueResume: %v", err)
	}
	assertArgv(t, argsLog, append([]string{"queue", "resume", "tasks"}, fleetJSON()...))
	resetArgv(t, argsLog)

	if _, err := m.QueueInfo(ctx, "demo", "v1", projectDir, "tasks"); err != nil {
		t.Fatalf("QueueInfo: %v", err)
	}
	assertArgv(t, argsLog, append([]string{"queue", "info", "tasks"}, fleetJSON()...))
}

func TestQueueSkipsUnknownName(t *testing.T) {
	argsLog, _ := installFakeOperatorCelld(t)
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")

	_, err := m.QueueInfo(context.Background(), "demo", "v1", projectDir, "bogus")
	if !errors.Is(err, ErrQueueNotFound) {
		t.Fatalf("bogus err = %v", err)
	}
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Fatalf("celld should not run for bogus queue, stat=%v", err)
	}

	if !HasQueue(projectDir, "events-dlq") {
		t.Fatal("events-dlq (dead_letter_queue) must be allowed")
	}
	if _, err := m.QueueInfo(context.Background(), "demo", "v1", projectDir, "events-dlq"); err != nil {
		t.Fatalf("events-dlq QueueInfo: %v", err)
	}
	if _, err := os.Stat(argsLog); err != nil {
		t.Fatalf("celld should run for events-dlq: %v", err)
	}
}
