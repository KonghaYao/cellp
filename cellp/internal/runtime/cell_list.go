package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const workflowClassPrefix = "__Workflow."

// WorkflowFilterLimitation is returned when a version declares ≥2 workflows and
// celld cell list cannot attribute instances to one wrangler workflow (AD-7).
const WorkflowFilterLimitation = "celld `cell list` 只给出 `__Workflow.{script}:<64-hex>`；实例内部名 `{workflow_name}/{instance_id}` 被哈希，无法按 workflow 名精确过滤。本响应返回该 version / 该 script 下全部 Workflow cell，并附 wrangler 声明的 workflow 名。精确归属需等 `celld workflow`。"

// ErrWorkflowNotFound is returned when path {name} is not a wrangler workflow.
var ErrWorkflowNotFound = errors.New("workflow_not_found")

// ErrCelldCellListFailed is returned when celld cell list exits non-zero.
var ErrCelldCellListFailed = errors.New("celld_cell_list_failed")

// CellRecord is one NDJSON object from `celld cell list --json`.
type CellRecord struct {
	Scope    string `json:"scope"`
	Class    string `json:"class"`
	ID       string `json:"id"`
	Reserved bool   `json:"reserved"`
}

// WorkflowInstances is GET .../workflows/{name}/instances.
type WorkflowInstances struct {
	WorkflowName      string       `json:"workflow_name"`
	Binding           string       `json:"binding"`
	ScriptName        string       `json:"script_name"`
	Filter            string       `json:"filter"`
	Limitation        *string      `json:"limitation"`
	WranglerWorkflows []string     `json:"wrangler_workflows"`
	Instances         []CellRecord `json:"instances"`
}

type cellListLine struct {
	Scope    string  `json:"scope"`
	Class    *string `json:"class"`
	ID       string  `json:"id"`
	Reserved bool    `json:"reserved"`
}

// ListCells runs `celld cell list [CLASS] --all --json` against a version bucket.
// Missing celld (dev) returns an empty list, matching Health().
func (m *Manager) ListCells(ctx context.Context, project, version, class string) ([]CellRecord, error) {
	celldBin, err := exec.LookPath("celld")
	if err != nil {
		return emptyCells(), nil
	}

	args := []string{"cell", "list"}
	if class != "" {
		args = append(args, class)
	}
	args = append(args,
		"--all",
		"--json",
		"--bucket", m.versionBucket(project, version),
		"--endpoint", m.endpoint,
		"--region", m.region,
	)
	cmd := exec.CommandContext(ctx, celldBin, args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("CELLD_VAR_PROJECT_ID=%s", project),
		fmt.Sprintf("CELLD_VAR_VERSION_ID=%s", version),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", m.accessKey),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", m.secretKey),
		fmt.Sprintf("AWS_REGION=%s", m.region),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%w: %s", ErrCelldCellListFailed, msg)
	}

	return parseCellListNDJSON(stdout.Bytes())
}

// ListWorkflowInstances lists reserved __Workflow.* cells for a wrangler workflow name.
// Multiple wrangler workflows use script-level fallback (filter=script); never 500.
func (m *Manager) ListWorkflowInstances(ctx context.Context, project, version, projectDir, name string) (*WorkflowInstances, error) {
	bindings, err := ParseBindings(projectDir)
	if err != nil {
		return nil, err
	}
	wf := findWorkflowBinding(bindings.Workflows, name)
	if wf == nil {
		return nil, ErrWorkflowNotFound
	}

	scriptName, err := workerName(projectDir)
	if err != nil {
		return nil, err
	}

	class := ""
	if scriptName != "" {
		class = workflowClassPrefix + scriptName
	}
	records, err := m.ListCells(ctx, project, version, class)
	if err != nil {
		return nil, err
	}

	names := wranglerWorkflowNames(bindings.Workflows)
	out := &WorkflowInstances{
		WorkflowName:      wf.Name,
		Binding:           wf.Binding,
		ScriptName:        scriptName,
		WranglerWorkflows: names,
		Instances:         records,
	}
	if len(bindings.Workflows) >= 2 {
		out.Filter = "script"
		lim := WorkflowFilterLimitation
		out.Limitation = &lim
	} else {
		out.Filter = "workflow"
	}
	return out, nil
}

func parseCellListNDJSON(stdout []byte) ([]CellRecord, error) {
	out := emptyCells()
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec cellListLine
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("parse celld json row: %w", err)
		}
		if rec.Class == nil || !rec.Reserved {
			continue
		}
		if !strings.HasPrefix(*rec.Class, workflowClassPrefix) {
			continue
		}
		out = append(out, CellRecord{
			Scope:    rec.Scope,
			Class:    *rec.Class,
			ID:       rec.ID,
			Reserved: rec.Reserved,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func findWorkflowBinding(wfs []WorkflowBinding, name string) *WorkflowBinding {
	for i := range wfs {
		if wfs[i].Name == name {
			return &wfs[i]
		}
	}
	for i := range wfs {
		if wfs[i].Binding == name {
			return &wfs[i]
		}
	}
	return nil
}

func wranglerWorkflowNames(wfs []WorkflowBinding) []string {
	out := make([]string, 0, len(wfs))
	for _, wf := range wfs {
		n := wf.Name
		if n == "" {
			n = wf.Binding
		}
		out = append(out, n)
	}
	return out
}

func workerName(projectDir string) (string, error) {
	raw, err := readWranglerConfig(projectDir)
	if err != nil {
		return "", err
	}
	var cfg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("parse wrangler: %w", err)
	}
	return strings.TrimSpace(cfg.Name), nil
}

func emptyCells() []CellRecord {
	return make([]CellRecord, 0)
}
