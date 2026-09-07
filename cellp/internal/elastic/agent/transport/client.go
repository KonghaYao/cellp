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
	"strconv"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/elastic/contract"
)

// Client calls Node Agent commands over HTTPS+mTLS.
type Client struct {
	cfg        ClientConfig
	baseURL    *url.URL
	httpClient *http.Client
}

// NewClient validates config and constructs an HTTPS-only client (no redirects).
func NewClient(cfg ClientConfig) (*Client, error) {
	cfg = normalizeClientConfig(cfg)
	if err := ValidateClientConfig(cfg); err != nil {
		return nil, err
	}
	u, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil {
		return nil, err
	}
	tlsCfg := buildClientTLSConfig(cfg.TLS, cfg.ExpectedNodeURI, cfg.ControllerIdentityURI)
	hc := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
			Proxy:           nil,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return errors.New("redirect forbidden")
		},
	}
	return &Client{cfg: cfg, baseURL: u, httpClient: hc}, nil
}

// StartReplica issues ActionStartReplica over mTLS.
func (c *Client) StartReplica(ctx context.Context, spec contract.StartReplicaSpec, idempotencyKey string) (contract.RuntimeReplica, error) {
	var out startResponse
	if err := c.post(ctx, routeStart, startRequest{Spec: spec}, idempotencyKey, &out); err != nil {
		return contract.RuntimeReplica{}, err
	}
	return out.Replica, nil
}

// ProbeReplica issues ActionProbeReplica over mTLS.
func (c *Client) ProbeReplica(ctx context.Context, scope contract.CommandScope) (agent.ProbeResult, error) {
	var out probeResponse
	if err := c.post(ctx, routeProbe, scopeRequest{Scope: scope}, "", &out); err != nil {
		return agent.ProbeResult{}, err
	}
	return agent.ProbeResult{
		ReplicaID:  out.Result.ReplicaID,
		State:      out.Result.State,
		Generation: out.Result.Generation,
	}, nil
}

// StopReplica issues ActionStopReplica over mTLS.
func (c *Client) StopReplica(ctx context.Context, scope contract.CommandScope) (contract.RuntimeReplica, error) {
	var out stopResponse
	if err := c.post(ctx, routeStop, scopeRequest{Scope: scope}, "", &out); err != nil {
		return contract.RuntimeReplica{}, err
	}
	return out.Replica, nil
}

// DrainReplica issues ActionDrainReplica over mTLS.
func (c *Client) DrainReplica(ctx context.Context, scope contract.CommandScope, deadline time.Time) (contract.RuntimeReplica, error) {
	req := drainRequest{Scope: scope}
	if !deadline.IsZero() {
		if !deadline.UTC().After(time.Now().UTC()) {
			return contract.RuntimeReplica{}, &agent.CommandError{Reason: contract.ReasonAuthFailed}
		}
		req.Deadline = deadline.UTC().Format(time.RFC3339)
	}
	var out drainResponse
	if err := c.post(ctx, routeDrain, req, "", &out); err != nil {
		return contract.RuntimeReplica{}, err
	}
	return out.Replica, nil
}

// ListReplicas issues ActionListReplicas over mTLS.
func (c *Client) ListReplicas(ctx context.Context, scope contract.CommandScope) ([]contract.RuntimeReplica, error) {
	var out listResponse
	if err := c.post(ctx, routeList, scopeRequest{Scope: scope}, "", &out); err != nil {
		return nil, err
	}
	return out.Replicas, nil
}

func (c *Client) post(ctx context.Context, path string, body interface{}, idempotencyKey string, out interface{}) error {
	u := *c.baseURL
	u.Path = path
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set(headerIdempotencyKey, idempotencyKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	maxBody := c.cfg.MaxBodyBytes
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maxBody {
		return &agent.CommandError{Reason: contract.ReasonRequestTooLarge}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeCommandError(resp.StatusCode, data, maxBody)
	}
	if err := decodeStrictJSON(data, out); err != nil {
		var ew errWire
		if errors.As(err, &ew) {
			return &agent.CommandError{Reason: ew.reason}
		}
		return &agent.CommandError{Reason: contract.ReasonAuthFailed}
	}
	return nil
}

func decodeStrictJSON(data []byte, out interface{}) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return errBadJSON
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errTrailingJSON
	}
	return nil
}

func decodeCommandError(status int, data []byte, maxBody int64) error {
	if int64(len(data)) > maxBody {
		return &agent.CommandError{Reason: contract.ReasonRequestTooLarge}
	}
	var we wireError
	if err := decodeStrictJSON(data, &we); err == nil && we.Reason != "" {
		return &agent.CommandError{Reason: contract.NormalizeWireReason(we.Reason)}
	}
	return &agent.CommandError{Reason: contract.ReasonAuthFailed}
}

// CommandErrorFromResponse exposes wire reason decoding for tests.
func CommandErrorFromResponse(status int, data []byte) error {
	return decodeCommandError(status, data, defaultMaxBodyBytes)
}

// BaseURLHost returns the configured host (for tests).
func (c *Client) BaseURLHost() string {
	return c.baseURL.Host
}

// ClientHTTPTransport returns the inner transport (for tests).
func (c *Client) ClientHTTPTransport() *http.Transport {
	tr, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		return nil
	}
	return tr
}

// JoinBaseURL builds an https base URL string.
func JoinBaseURL(host string, port int) string {
	return fmt.Sprintf("https://%s", netJoinHostPort(host, port))
}

func netJoinHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}
