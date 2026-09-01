package gateway

import (
	"context"

	"github.com/cellp/cellp/internal/registry"
)

type invalidatingStore struct {
	registry.Store
	gw *Gateway
}

// WrapStore returns a registry.Store that invalidates gateway route cache entries on writes.
func WrapStore(store registry.Store, gw *Gateway) registry.Store {
	if gw == nil {
		return store
	}
	return &invalidatingStore{Store: store, gw: gw}
}

func (s *invalidatingStore) SetRoute(ctx context.Context, route registry.Route) error {
	if err := s.Store.SetRoute(ctx, route); err != nil {
		return err
	}
	s.gw.InvalidateRoute(route.ProjectID, route.VersionID)
	return nil
}

func (s *invalidatingStore) SetRouteActive(ctx context.Context, projectID, versionID string, active bool) error {
	if err := s.Store.SetRouteActive(ctx, projectID, versionID, active); err != nil {
		return err
	}
	s.gw.InvalidateRoute(projectID, versionID)
	return nil
}

func (s *invalidatingStore) DeleteRoute(ctx context.Context, projectID, versionID string) error {
	if err := s.Store.DeleteRoute(ctx, projectID, versionID); err != nil {
		return err
	}
	s.gw.InvalidateRoute(projectID, versionID)
	return nil
}

func (s *invalidatingStore) SetProdVersion(ctx context.Context, projectID, versionID string) error {
	if err := s.Store.SetProdVersion(ctx, projectID, versionID); err != nil {
		return err
	}
	s.gw.InvalidateProd(projectID)
	return nil
}

func (s *invalidatingStore) SetProdVersionCAS(ctx context.Context, projectID, expected, new string) error {
	if err := s.Store.SetProdVersionCAS(ctx, projectID, expected, new); err != nil {
		return err
	}
	s.gw.InvalidateProd(projectID)
	if expected != "" {
		s.gw.InvalidateRoute(projectID, expected)
	}
	if new != "" {
		s.gw.InvalidateRoute(projectID, new)
	}
	return nil
}

func (s *invalidatingStore) UpsertIngressBinding(ctx context.Context, b registry.IngressBinding) error {
	if err := s.Store.UpsertIngressBinding(ctx, b); err != nil {
		return err
	}
	if b.Host != nil {
		s.gw.InvalidateIngressHost(*b.Host)
	}
	if b.Role == registry.IngressRoleProd {
		s.gw.InvalidateProd(b.ProjectID)
	}
	return nil
}

func (s *invalidatingStore) SetIngressBindingActive(ctx context.Context, bindingID string, active bool) error {
	b, _ := s.Store.GetIngressBinding(ctx, bindingID)
	if err := s.Store.SetIngressBindingActive(ctx, bindingID, active); err != nil {
		return err
	}
	if b != nil && b.Host != nil {
		s.gw.InvalidateIngressHost(*b.Host)
	}
	if b != nil && b.Role == registry.IngressRoleProd {
		s.gw.InvalidateProd(b.ProjectID)
	}
	return nil
}
