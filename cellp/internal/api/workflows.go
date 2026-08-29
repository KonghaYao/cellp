package api

import (
	"errors"
	"net/http"

	"github.com/cellp/cellp/internal/runtime"
	"github.com/go-chi/chi/v5"
)

type workflowListItem struct {
	Binding      string `json:"binding"`
	WorkflowName string `json:"workflow_name"`
	ClassName    string `json:"class_name"`
}

type workflowListResp struct {
	Workflows []workflowListItem `json:"workflows"`
}

func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveReadyOperator(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}

	bindings, err := runtime.ParseBindings(ctx.projectDir)
	if err != nil {
		if errors.Is(err, runtime.ErrNoWrangler) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "bindings_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	items := make([]workflowListItem, 0, len(bindings.Workflows))
	for _, wf := range bindings.Workflows {
		items = append(items, workflowListItem{
			Binding:      wf.Binding,
			WorkflowName: wf.Name,
			ClassName:    wf.ClassName,
		})
	}
	writeJSON(w, http.StatusOK, workflowListResp{Workflows: items})
}

func (s *Server) handleListWorkflowInstances(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveReadyOperator(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	name := chi.URLParam(r, "name")

	result, err := s.runtime.ListWorkflowInstances(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, name)
	if err != nil {
		if errors.Is(err, runtime.ErrNoWrangler) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "bindings_not_found"})
			return
		}
		if errors.Is(err, runtime.ErrWorkflowNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow_not_found"})
			return
		}
		if errors.Is(err, runtime.ErrCelldCellListFailed) {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "celld_cell_list_failed"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
