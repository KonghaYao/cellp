package agentrun

import (
	"fmt"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/agent"
	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	noderegtransport "github.com/cellp/cellp/internal/elastic/nodereg/transport"
	registryrelaytransport "github.com/cellp/cellp/internal/elastic/registryrelay/transport"
	"github.com/cellp/cellp/internal/runtime"
)

// Deps wires production defaults for tests and cellp agent.
type Deps struct {
	NewManager func(platformCfg config.Config, elasticCfg config.ElasticConfig) (*runtime.Manager, error)
	NewBackend func(m *runtime.Manager) agent.LifecycleBackend
	LoadTLS    func(cfg config.ElasticConfig) (TLSBundle, error)
	// Optional dials for deterministic orchestration tests (production uses HTTPS+mTLS clients).
	DialNodeReg       func(elasticCfg config.ElasticConfig, clientTLS agenttransport.TLSMaterials) (nodeRegistrationClient, error)
	DialRegistryRelay func(elasticCfg config.ElasticConfig, clientTLS agenttransport.TLSMaterials) (*registryrelaytransport.Client, error)
	// When set, replaces agentstore over registry relay (relay may be nil if unused).
	LifecycleStores func(relay *registryrelaytransport.Client, elasticCfg config.ElasticConfig) (agent.NodeStore, agent.LifecycleStore)
}

func (d Deps) nodeRegClient(elasticCfg config.ElasticConfig, clientTLS agenttransport.TLSMaterials) (nodeRegistrationClient, error) {
	if d.DialNodeReg != nil {
		return d.DialNodeReg(elasticCfg, clientTLS)
	}
	return noderegtransport.NewClient(noderegtransport.ClientConfig{
		BaseURL: elasticCfg.NodeRegBaseURL, NodeIdentityURI: elasticCfg.NodeIdentityURI,
		ControllerIdentityURI: elasticCfg.ControllerIdentityURI, TLS: clientTLS,
		MaxBodyBytes: elasticCfg.MaxBodyBytes,
	})
}

func (d Deps) registryRelayClient(elasticCfg config.ElasticConfig, clientTLS agenttransport.TLSMaterials) (*registryrelaytransport.Client, error) {
	if d.DialRegistryRelay != nil {
		return d.DialRegistryRelay(elasticCfg, clientTLS)
	}
	c, err := registryrelaytransport.NewClient(registryrelaytransport.ClientConfig{
		BaseURL: elasticCfg.RegistryRelayBaseURL, NodeIdentityURI: elasticCfg.NodeIdentityURI,
		ControllerIdentityURI: elasticCfg.ControllerIdentityURI, TLS: clientTLS,
		MaxBodyBytes: elasticCfg.MaxBodyBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("registry relay client: %w", err)
	}
	return c, nil
}

func (d Deps) manager(platformCfg config.Config, elasticCfg config.ElasticConfig) (*runtime.Manager, error) {
	if d.NewManager != nil {
		return d.NewManager(platformCfg, elasticCfg)
	}
	return DefaultManager(platformCfg, elasticCfg), nil
}

func (d Deps) backend(m *runtime.Manager) agent.LifecycleBackend {
	if d.NewBackend != nil {
		return d.NewBackend(m)
	}
	return agent.ManagerBackend{Manager: m}
}

// DefaultManager builds a runtime manager without registry access.
func DefaultManager(platformCfg config.Config, elasticCfg config.ElasticConfig) *runtime.Manager {
	m := runtime.New(platformCfg.CelldBasePort, platformCfg.S3Endpoint, platformCfg.S3Region, platformCfg.CelldBucket, platformCfg.S3AccessKey, platformCfg.S3SecretKey)
	if elasticCfg.Enabled {
		m.SetReplicaHostConfig(runtime.ReplicaHostConfig{
			BindHost:      elasticCfg.CelldBindHost,
			AdvertiseHost: elasticCfg.CelldAdvertiseHost,
		})
	}
	return m
}
