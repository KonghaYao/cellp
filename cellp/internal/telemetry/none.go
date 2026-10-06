package telemetry

import "context"

type NoneBackend struct{}

func (NoneBackend) Context(_ context.Context, _, _ string) (ContextResponse, error) {
	return ContextResponse{Enabled: false, Backend: "none"}, nil
}

func (NoneBackend) GetTrace(_ context.Context, _, _, _ string) (TraceResponse, error) {
	return TraceResponse{}, ErrTraceNotFound
}

func (NoneBackend) Search(_ context.Context, _, _ string, _ SearchRequest) ([]SearchHit, error) {
	return nil, nil
}

func (NoneBackend) DeepLink(_, _, _ string) string { return "" }
