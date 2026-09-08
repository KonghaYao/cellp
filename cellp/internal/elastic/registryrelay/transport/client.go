package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	"github.com/cellp/cellp/internal/registry"
)

// ClientConfig configures the node-side registry relay HTTPS client.
type ClientConfig struct {
	BaseURL               string
	NodeIdentityURI       string
	ControllerIdentityURI string
	TLS                   agenttransport.TLSMaterials
	Timeout               time.Duration
	MaxBodyBytes          int64
}

// Client talks to the active controller registry relay API.
type Client struct {
	cfg        ClientConfig
	base       *url.URL
	httpClient *http.Client
	nonceSeq   uint64
}

// NewClient validates config and constructs the HTTPS client.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 1 << 20
	}
	u, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || u.Scheme != "https" {
		return nil, errors.New("https base url required")
	}
	tlsCfg := agenttransport.BuildClientTLSConfig(cfg.TLS, cfg.ControllerIdentityURI, cfg.NodeIdentityURI)
	return &Client{
		cfg:  cfg,
		base: u,
		httpClient: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: &http.Transport{TLSClientConfig: tlsCfg, Proxy: nil},
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("redirect forbidden")
			},
		},
	}, nil
}

func (c *Client) nextNonce() string {
	return fmt.Sprintf("relay-%d", atomic.AddUint64(&c.nonceSeq, 1))
}

func (c *Client) post(ctx context.Context, path string, body interface{}, out interface{}) error {
	u := c.base.ResolveReference(&url.URL{Path: path})
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	if int64(len(buf)) > c.cfg.MaxBodyBytes {
		return &registryrelay.RelayError{Reason: contract.ReasonRequestTooLarge}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
	if err != nil {
		return wrapTransportErr(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return wrapTransportErr(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.cfg.MaxBodyBytes+1))
	if err != nil {
		return wrapTransportErr(err)
	}
	if int64(len(data)) > c.cfg.MaxBodyBytes {
		return &registryrelay.RelayError{Reason: contract.ReasonRequestTooLarge}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return registryrelay.MapClientError(decodeRelayError(resp.StatusCode, data))
	}
	if out == nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return &registryrelay.RelayError{Reason: contract.ReasonAuthFailed}
	}
	if err := dec.Decode(&struct{}{}); err != nil && err != io.EOF {
		return &registryrelay.RelayError{Reason: contract.ReasonAuthFailed}
	}
	return nil
}

func wrapTransportErr(err error) error {
	if err == nil {
		return nil
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return fmt.Errorf("%w: %v", registryrelay.ErrRegistryUnavailable, err)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	return fmt.Errorf("%w: %v", registryrelay.ErrRegistryUnavailable, err)
}

// NewRelayScope builds a replay-bounded scope for the given node.
func (c *Client) NewRelayScope(nodeID string, ttl time.Duration) contract.RegistryRelayScope {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	now := time.Now().UTC()
	return contract.RegistryRelayScope{
		NodeID:    nodeID,
		IssuedAt:  now,
		ExpiresAt: now.Add(ttl),
		Nonce:     c.nextNonce(),
	}
}

func (c *Client) GetRuntimeNode(ctx context.Context, scope contract.RegistryRelayScope) (*contract.RuntimeNode, error) {
	var out runtimeNodeResponse
	if err := c.post(ctx, routeGetRuntimeNode, envelope{Scope: scope}, &out); err != nil {
		if errors.Is(err, registry.ErrRuntimeNodeNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return runtimeNodeFromWire(out.Node), nil
}

func (c *Client) GetRuntimeReplica(ctx context.Context, scope contract.RegistryRelayScope, replicaID string) (*contract.RuntimeReplica, error) {
	var out runtimeReplicaResponse
	req := getRuntimeReplicaRequest{Scope: scope, ReplicaID: replicaID}
	if err := c.post(ctx, routeGetRuntimeReplica, req, &out); err != nil {
		if errors.Is(err, registry.ErrRuntimeReplicaNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return runtimeReplicaFromWire(out.Replica), nil
}

func (c *Client) ValidateAgentAssignment(ctx context.Context, scope contract.RegistryRelayScope, cmdScope contract.CommandScope, now time.Time) (*contract.RuntimeReplica, error) {
	var out runtimeReplicaResponse
	req := validateAssignmentRequest{Scope: scope, CmdScope: cmdScope, Now: now.UTC()}
	if err := c.post(ctx, routeValidateAgentAssignment, req, &out); err != nil {
		if errors.Is(err, registry.ErrRuntimeReplicaNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return runtimeReplicaFromWire(out.Replica), nil
}

func (c *Client) ValidateAgentCleanupAssignment(ctx context.Context, scope contract.RegistryRelayScope, cmdScope contract.CommandScope) (*contract.RuntimeReplica, error) {
	var out runtimeReplicaResponse
	req := validateCleanupRequest{Scope: scope, CmdScope: cmdScope}
	if err := c.post(ctx, routeValidateAgentCleanupAssignment, req, &out); err != nil {
		if errors.Is(err, registry.ErrRuntimeReplicaNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return runtimeReplicaFromWire(out.Replica), nil
}

func (c *Client) ListRuntimeReplicasByNode(ctx context.Context, scope contract.RegistryRelayScope) ([]contract.RuntimeReplica, error) {
	var out replicasResponse
	if err := c.post(ctx, routeListRuntimeReplicasByNode, envelope{Scope: scope}, &out); err != nil {
		return nil, err
	}
	return runtimeReplicasFromWire(out.Replicas), nil
}

func (c *Client) RecordObservation(ctx context.Context, scope contract.RegistryRelayScope, obs registry.ReplicaObservation) error {
	return c.post(ctx, routeRecordObservation, recordObservationRequest{Scope: scope, Obs: obs}, nil)
}

func (c *Client) ClaimAgentCommand(ctx context.Context, scope contract.RegistryRelayScope, command registry.AgentCommand) (registry.AgentCommandClaim, error) {
	var out claimResponse
	if err := c.post(ctx, routeClaimAgentCommand, claimCommandRequest{Scope: scope, Command: command}, &out); err != nil {
		return registry.AgentCommandClaim{}, err
	}
	return out.Claim, nil
}

func (c *Client) RenewAgentCommandLease(ctx context.Context, scope contract.RegistryRelayScope, command registry.AgentCommand, expiry time.Time) error {
	return c.post(ctx, routeRenewAgentCommandLease, renewCommandRequest{Scope: scope, Command: command, Expiry: expiry.UTC()}, nil)
}

func (c *Client) CompleteAgentCommand(ctx context.Context, scope contract.RegistryRelayScope, command registry.AgentCommand) error {
	return c.post(ctx, routeCompleteAgentCommand, completeCommandRequest{Scope: scope, Command: command}, nil)
}

func (c *Client) RecordObservationAndCompleteAgentCommand(ctx context.Context, scope contract.RegistryRelayScope, obs registry.ReplicaObservation, command registry.AgentCommand) error {
	return c.post(ctx, routeRecordObservationAndComplete, recordAndCompleteRequest{Scope: scope, Obs: obs, Command: command}, nil)
}

func (c *Client) WithdrawReplica(ctx context.Context, scope contract.RegistryRelayScope, replicaID, projectID, versionID string, generation int64) error {
	return c.post(ctx, routeWithdrawReplica, withdrawRequest{
		Scope: scope, ReplicaID: replicaID, ProjectID: projectID, VersionID: versionID, Generation: generation,
	}, nil)
}

func (c *Client) TerminalizeReplica(ctx context.Context, scope contract.RegistryRelayScope, replicaID string, generation int64, state contract.ReplicaState) error {
	return c.post(ctx, routeTerminalizeReplica, terminalizeRequest{
		Scope: scope, ReplicaID: replicaID, Generation: generation, State: state,
	}, nil)
}

// GetVersionEnv reads worker env overrides for a version assigned to this node.
func (c *Client) GetVersionEnv(ctx context.Context, scope contract.RegistryRelayScope, projectID, versionID string) (map[string]string, error) {
	var out versionEnvResponse
	req := getVersionEnvRequest{Scope: scope, ProjectID: projectID, VersionID: versionID}
	if err := c.post(ctx, routeGetVersionEnv, req, &out); err != nil {
		return nil, err
	}
	if out.Env == nil {
		return map[string]string{}, nil
	}
	return out.Env, nil
}
