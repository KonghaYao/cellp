package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
)

// ClientConfig configures the node-side registration HTTPS client.
type ClientConfig struct {
	BaseURL               string
	NodeIdentityURI       string
	ControllerIdentityURI string
	TLS                   agenttransport.TLSMaterials
	Timeout               time.Duration
	MaxBodyBytes          int64
}

// Client talks to the active controller node-registration API.
type Client struct {
	cfg        ClientConfig
	base       *url.URL
	httpClient *http.Client
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

// Activate registers the node generation on the controller.
func (c *Client) Activate(ctx context.Context, req contract.ActivateNodeRequest) (contract.ActivateNodeResponse, error) {
	var out contract.ActivateNodeResponse
	if err := c.post(ctx, routeActivate, req, &out); err != nil {
		return contract.ActivateNodeResponse{}, err
	}
	return out, nil
}

// Heartbeat renews the node lease.
func (c *Client) Heartbeat(ctx context.Context, req contract.HeartbeatNodeRequest) error {
	return c.post(ctx, routeHeartbeat, req, nil)
}

// Cordon marks the node cordoned at the controller.
func (c *Client) Cordon(ctx context.Context, req contract.CordonNodeRequest) error {
	return c.post(ctx, routeCordon, req, nil)
}

// Release releases the node lease.
func (c *Client) Release(ctx context.Context, req contract.ReleaseNodeRequest) error {
	return c.post(ctx, routeRelease, req, nil)
}

// Status reads registry node metadata.
func (c *Client) Status(ctx context.Context, req contract.StatusNodeRequest) (contract.StatusNodeResponse, error) {
	var out contract.StatusNodeResponse
	if err := c.post(ctx, routeStatus, req, &out); err != nil {
		return contract.StatusNodeResponse{}, err
	}
	return out, nil
}

func (c *Client) post(ctx context.Context, path string, body interface{}, out interface{}) error {
	u := c.base.ResolveReference(&url.URL{Path: path})
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		if int64(len(b)) > c.cfg.MaxBodyBytes {
			return errors.New("body too large")
		}
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, c.cfg.MaxBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if int64(len(data)) > c.cfg.MaxBodyBytes {
		return errors.New("response too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeClientError(resp.StatusCode, data, resp.Header.Get("Retry-After"))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}
