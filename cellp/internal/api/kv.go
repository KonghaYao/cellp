package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
	"github.com/go-chi/chi/v5"
)

type operatorError struct {
	status int
	code   string
}

func (e *operatorError) Error() string { return e.code }

type operatorContext struct {
	projectID  string
	versionID  string
	projectDir string
}

type kvContext struct {
	operatorContext
	ns string
}

func (s *Server) resolveReadyOperator(r *http.Request) (*operatorContext, error) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")

	v, err := s.store.GetVersion(r.Context(), projectID, versionID)
	if err != nil {
		return nil, &operatorError{status: http.StatusInternalServerError, code: err.Error()}
	}
	if v == nil {
		return nil, &operatorError{status: http.StatusNotFound, code: "version_not_found"}
	}
	if v.Status != registry.StatusReady {
		return nil, &operatorError{status: http.StatusNotFound, code: "version_not_ready"}
	}
	return &operatorContext{
		projectID:  projectID,
		versionID:  versionID,
		projectDir: filepath.Join(s.cfg.ArtifactsDir, projectID, versionID),
	}, nil
}

func (s *Server) resolveKVContext(r *http.Request) (*kvContext, error) {
	base, err := s.resolveReadyOperator(r)
	if err != nil {
		return nil, err
	}
	ns := chi.URLParam(r, "ns")
	if !runtime.HasKVNamespace(base.projectDir, ns) {
		return nil, &operatorError{status: http.StatusNotFound, code: "kv_namespace_not_found"}
	}
	return &kvContext{operatorContext: *base, ns: ns}, nil
}

func writeOperatorError(w http.ResponseWriter, err error) {
	var op *operatorError
	if errors.As(err, &op) {
		writeJSON(w, op.status, map[string]string{"error": op.code})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func writeOperatorExecError(w http.ResponseWriter, err error) {
	if errors.Is(err, runtime.ErrCelldUnavailable) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "celld_unavailable"})
		return
	}
	if errors.Is(err, runtime.ErrKVKeyNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "key_not_found"})
		return
	}
	if errors.Is(err, runtime.ErrTTLTooSmall) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ttl_too_small"})
		return
	}
	if errors.Is(err, runtime.ErrMetadataTooLarge) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "metadata_too_large"})
		return
	}
	msg := err.Error()
	if _, rest, ok := strings.Cut(msg, ": "); ok && strings.HasPrefix(msg, "celld ") {
		msg = rest
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}

type kvNamespaceItem struct {
	ID      string `json:"id"`
	Binding string `json:"binding"`
}

func (s *Server) handleListKVNamespaces(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveReadyOperator(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	bindings, err := runtime.ParseBindings(ctx.projectDir)
	if err != nil {
		if errors.Is(err, runtime.ErrNoWrangler) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "wrangler_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	ns := make([]kvNamespaceItem, 0, len(bindings.KV))
	for _, k := range bindings.KV {
		ns = append(ns, kvNamespaceItem{ID: k.ID, Binding: k.Binding})
	}
	writeJSON(w, http.StatusOK, map[string]any{"namespaces": ns})
}

func (s *Server) handleGetKVInfo(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveKVContext(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	info, err := s.runtime.KvInfo(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, ctx.ns)
	if err != nil {
		writeOperatorExecError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleListKVKeys(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveKVContext(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	q := r.URL.Query()
	limit := 0
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			limit = n
		}
	}
	result, err := s.runtime.KvList(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, ctx.ns, q.Get("prefix"), q.Get("cursor"), limit)
	if err != nil {
		writeOperatorExecError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func kvKeyParam(r *http.Request) string {
	if k := chi.URLParam(r, "key"); k != "" {
		return k
	}
	return strings.TrimPrefix(chi.URLParam(r, "*"), "/")
}

func (s *Server) handleGetKVValue(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveKVContext(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	key := kvKeyParam(r)
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key_required"})
		return
	}
	raw, err := s.runtime.KvGet(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, ctx.ns, key)
	if err != nil {
		writeOperatorExecError(w, err)
		return
	}
	resp := map[string]string{"key": key}
	if utf8.Valid(raw) && bytes.IndexByte(raw, 0) < 0 {
		resp["value"] = string(raw)
		resp["encoding"] = "utf-8"
	} else {
		resp["value"] = base64.StdEncoding.EncodeToString(raw)
		resp["encoding"] = "base64"
	}
	writeJSON(w, http.StatusOK, resp)
}

type putKVReq struct {
	Value    *string         `json:"value"`
	TTL      *float64        `json:"ttl"`
	Metadata json.RawMessage `json:"metadata"`
	Base64   bool            `json:"base64"`
	Binary   bool            `json:"binary"`
}

func (s *Server) handlePutKVValue(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveKVContext(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	key := kvKeyParam(r)
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key_required"})
		return
	}
	var req putKVReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Value == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "value_required"})
		return
	}

	in := runtime.KvPutInput{Value: []byte(*req.Value)}
	if req.Base64 || req.Binary {
		decoded, err := base64.StdEncoding.DecodeString(*req.Value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_base64"})
			return
		}
		in.Value = decoded
		in.Binary = true
	}
	if req.TTL != nil {
		n := int64(*req.TTL)
		if n < 60 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ttl_too_small"})
			return
		}
		in.TTL = &n
	}
	if len(req.Metadata) > 0 && string(req.Metadata) != "null" {
		var v any
		if err := json.Unmarshal(req.Metadata, &v); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		switch t := v.(type) {
		case string:
			in.Metadata = t
		default:
			b, err := json.Marshal(t)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			in.Metadata = string(b)
		}
		if len(in.Metadata) > 1024 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "metadata_too_large"})
			return
		}
	}

	if err := s.runtime.KvPut(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, ctx.ns, key, in); err != nil {
		writeOperatorExecError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteKVValue(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveKVContext(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	key := kvKeyParam(r)
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key_required"})
		return
	}
	if err := s.runtime.KvDelete(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, ctx.ns, key); err != nil {
		writeOperatorExecError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
