package registry

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

const (
	agentCommandRetention = 24 * time.Hour
	agentCommandLease     = 30 * time.Second
)

// ClaimAgentCommand atomically creates a running claim or returns its durable terminal result.
func (s *SQLiteStore) ClaimAgentCommand(ctx context.Context, command AgentCommand) (AgentCommandClaim, error) {
	if err := validateAgentCommand(command); err != nil {
		return AgentCommandClaim{}, err
	}
	now := time.Now().UTC()
	leaseExpires := now.Add(s.commandLease)
	token, err := newAttemptToken()
	if err != nil {
		return AgentCommandClaim{}, err
	}
	if command.ExpiresAt.IsZero() || command.ExpiresAt.After(now.Add(agentCommandRetention)) {
		command.ExpiresAt = now.Add(agentCommandRetention)
	}
	return withRetry(func() (AgentCommandClaim, error) {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return AgentCommandClaim{}, err
		}
		defer tx.Rollback()
		_, err = tx.ExecContext(ctx, `DELETE FROM runtime_agent_commands WHERE expires_at <= ?`, now.Format(time.RFC3339Nano))
		if err != nil {
			return AgentCommandClaim{}, err
		}
		stored, err := getAgentCommandTx(ctx, tx, command.IdempotencyKey)
		if err != nil {
			return AgentCommandClaim{}, err
		}
		if stored != nil {
			if !sameAgentCommandScope(*stored, command) {
				return AgentCommandClaim{}, ErrAgentCommandConflict
			}
			if stored.Status == "running" {
				if stored.LeaseExpiresAt.After(now) {
					return AgentCommandClaim{}, ErrAgentCommandInProgress
				}
				res, err := tx.ExecContext(ctx, `
UPDATE runtime_agent_commands SET lease_expires_at = ?, attempt_token = ?, updated_at = ?
WHERE idempotency_key = ? AND status = 'running' AND (lease_expires_at IS NULL OR lease_expires_at <= ?)`,
					leaseExpires.Format(time.RFC3339Nano), token, now.Format(time.RFC3339Nano),
					command.IdempotencyKey, now.Format(time.RFC3339Nano))
				if err != nil {
					return AgentCommandClaim{}, err
				}
				n, _ := res.RowsAffected()
				if n != 1 {
					return AgentCommandClaim{}, ErrAgentCommandInProgress
				}
				if err := tx.Commit(); err != nil {
					return AgentCommandClaim{}, err
				}
				command.Status, command.AttemptToken, command.LeaseExpiresAt = "running", token, leaseExpires
				return AgentCommandClaim{Claimed: true, Command: command}, nil
			}
			return AgentCommandClaim{Command: *stored}, tx.Commit()
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO runtime_agent_commands
(idempotency_key, action, node_id, project_id, version_id, replica_id, generation, status, expires_at, lease_expires_at, attempt_token, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 'running', ?, ?, ?, ?)`,
			command.IdempotencyKey, string(command.Action), command.NodeID, command.ProjectID,
			command.VersionID, command.ReplicaID, command.Generation,
			command.ExpiresAt.UTC().Format(time.RFC3339Nano), leaseExpires.Format(time.RFC3339Nano), token, now.Format(time.RFC3339Nano))
		if err != nil {
			return AgentCommandClaim{}, err
		}
		if err := tx.Commit(); err != nil {
			return AgentCommandClaim{}, err
		}
		command.Status, command.AttemptToken, command.LeaseExpiresAt = "running", token, leaseExpires
		return AgentCommandClaim{Claimed: true, Command: command}, nil
	})
}

// RenewAgentCommandLease extends a running command lease only for its current attempt owner.
func (s *SQLiteStore) RenewAgentCommandLease(ctx context.Context, command AgentCommand, expiry time.Time) error {
	if err := validateAgentCommand(command); err != nil {
		return err
	}
	if command.AttemptToken == "" || !expiry.After(time.Now().UTC()) {
		return ErrAgentCommandConflict
	}
	return withRetryErr(func() error {
		res, err := s.db.ExecContext(ctx, `
UPDATE runtime_agent_commands SET lease_expires_at = ?, updated_at = ?
WHERE idempotency_key = ? AND action = ? AND node_id = ? AND project_id = ?
  AND version_id = ? AND replica_id = ? AND generation = ? AND status = 'running' AND attempt_token = ?`,
			expiry.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano),
			command.IdempotencyKey, string(command.Action), command.NodeID, command.ProjectID,
			command.VersionID, command.ReplicaID, command.Generation, command.AttemptToken)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrAgentCommandConflict
		}
		return nil
	})
}

// CompleteAgentCommand persists a bounded success or failure result for replay.
func (s *SQLiteStore) CompleteAgentCommand(ctx context.Context, command AgentCommand) error {
	if err := validateAgentCommand(command); err != nil {
		return err
	}
	if command.Status != "succeeded" && command.Status != "failed" {
		return fmt.Errorf("terminal command status required")
	}
	if command.ExpiresAt.IsZero() {
		command.ExpiresAt = time.Now().UTC().Add(agentCommandRetention)
	}
	return withRetryErr(func() error {
		res, err := s.db.ExecContext(ctx, `
UPDATE runtime_agent_commands
SET status = ?, result_state = ?, reason = ?, expires_at = ?, updated_at = ?
WHERE idempotency_key = ? AND action = ? AND node_id = ? AND project_id = ?
  AND version_id = ? AND replica_id = ? AND generation = ? AND status = 'running' AND attempt_token = ?`,
			command.Status, nullCommandString(string(command.ResultState)), nullCommandString(string(command.Reason)),
			command.ExpiresAt.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano),
			command.IdempotencyKey, string(command.Action), command.NodeID, command.ProjectID,
			command.VersionID, command.ReplicaID, command.Generation, command.AttemptToken)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrAgentCommandConflict
		}
		return nil
	})
}

func getAgentCommandTx(ctx context.Context, tx *sql.Tx, key string) (*AgentCommand, error) {
	var c AgentCommand
	var action, status, state, reason, expiry string
	var leaseExpiry, attemptToken sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT action, node_id, project_id, version_id, replica_id, generation, status,
       COALESCE(result_state, ''), COALESCE(reason, ''), expires_at, lease_expires_at, attempt_token
FROM runtime_agent_commands WHERE idempotency_key = ?`, key).
		Scan(&action, &c.NodeID, &c.ProjectID, &c.VersionID, &c.ReplicaID, &c.Generation,
			&status, &state, &reason, &expiry, &leaseExpiry, &attemptToken)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.IdempotencyKey = key
	c.Action = contract.LifecycleAction(action)
	c.Status = status
	c.ResultState = contract.ReplicaState(state)
	c.Reason = contract.NormalizeWireReason(contract.ReasonCode(reason))
	if reason == "" {
		c.Reason = ""
	}
	c.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiry)
	if err != nil {
		return nil, fmt.Errorf("command timestamp corrupt")
	}
	if leaseExpiry.Valid {
		c.LeaseExpiresAt, err = time.Parse(time.RFC3339Nano, leaseExpiry.String)
		if err != nil {
			return nil, fmt.Errorf("command timestamp corrupt")
		}
	}
	c.AttemptToken = attemptToken.String
	return &c, nil
}

func validateAgentCommand(c AgentCommand) error {
	if strings.TrimSpace(c.IdempotencyKey) == "" || strings.TrimSpace(c.NodeID) == "" ||
		strings.TrimSpace(c.ProjectID) == "" || strings.TrimSpace(c.VersionID) == "" ||
		strings.TrimSpace(c.ReplicaID) == "" || c.Action == "" || c.Generation <= 0 {
		return fmt.Errorf("invalid agent command")
	}
	if len(c.IdempotencyKey) > 256 {
		return fmt.Errorf("invalid agent command")
	}
	return nil
}

func sameAgentCommandScope(a, b AgentCommand) bool {
	return a.Action == b.Action && a.NodeID == b.NodeID && a.ProjectID == b.ProjectID &&
		a.VersionID == b.VersionID && a.ReplicaID == b.ReplicaID && a.Generation == b.Generation
}

func newAttemptToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func nullCommandString(v string) interface{} {
	if v == "" {
		return nil
	}
	return v
}
