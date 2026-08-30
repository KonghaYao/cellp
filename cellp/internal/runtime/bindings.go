package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrNoWrangler is returned when neither wrangler.jsonc nor wrangler.json exists.
var ErrNoWrangler = errors.New("wrangler not found")

// Bindings is GET /v1/projects/{id}/versions/{vid}/bindings (DESIGN §8.4).
type Bindings struct {
	D1        []D1Binding       `json:"d1"`
	KV        []KVNamespace     `json:"kv"`
	Queues    []QueueBinding    `json:"queues"`
	Workflows []WorkflowBinding `json:"workflows"`
	R2        []R2Bucket        `json:"r2"`
	Crons     []string          `json:"crons"`
}

// wrangler d1_databases[] — celld: binding, database_name, database_id
type D1Binding struct {
	Binding      string `json:"binding"`
	DatabaseName string `json:"database_name"`
	DatabaseID   string `json:"database_id,omitempty"`
}

// wrangler kv_namespaces[] — celld: binding + id（verbatim；禁止发明 namespace_name）
// T2 路径 {ns} == ID
type KVNamespace struct {
	Binding string `json:"binding"`
	ID      string `json:"id"`
}

// 按 wrangler queues.*.queue 去重合并。
// producers: binding, queue（忽略 delivery_delay）
// consumers: queue, dead_letter_queue（忽略 max_batch_* / retry_delay / script_name）
type QueueBinding struct {
	Name            string  `json:"name"`                        // queues.producers[].queue 或 queues.consumers[].queue
	Binding         string  `json:"binding,omitempty"`           // 仅 producer 的 env 名
	Consumer        bool    `json:"consumer"`                    // 本 script 是否在 consumers[] 里声明了该 queue
	DeadLetterQueue *string `json:"dead_letter_queue,omitempty"` // 仅 consumer
}

// wrangler workflows[] — celld: binding, name, class_name（script_name 忽略）
// T3 路径 {name} == Name（资源名，不是 binding）
type WorkflowBinding struct {
	Binding   string `json:"binding"`
	Name      string `json:"name"`
	ClassName string `json:"class_name"`
}

// wrangler r2_buckets[] — celld: binding, bucket_name（拒绝 jurisdiction；清单也不输出）
type R2Bucket struct {
	Binding    string `json:"binding"`
	BucketName string `json:"bucket_name"`
}

type wranglerFile struct {
	D1Databases  []wranglerD1       `json:"d1_databases"`
	KVNamespaces []wranglerKV       `json:"kv_namespaces"`
	Queues       *wranglerQueues    `json:"queues"`
	Workflows    []wranglerWorkflow `json:"workflows"`
	R2Buckets    []wranglerR2       `json:"r2_buckets"`
	Vars         map[string]string  `json:"vars"`
	Triggers     *struct {
		Crons []string `json:"crons"`
	} `json:"triggers"`
}

type wranglerD1 struct {
	Binding      string `json:"binding"`
	DatabaseName string `json:"database_name"`
	DatabaseID   string `json:"database_id"`
}

type wranglerKV struct {
	Binding   string `json:"binding"`
	ID        string `json:"id"`
	PreviewID string `json:"preview_id"` // celld 接受但 deploy 忽略；清单不输出
}

type wranglerQueues struct {
	Producers []struct {
		Binding string `json:"binding"`
		Queue   string `json:"queue"`
	} `json:"producers"`
	Consumers []struct {
		Queue           string  `json:"queue"`
		DeadLetterQueue *string `json:"dead_letter_queue"`
	} `json:"consumers"`
}

type wranglerWorkflow struct {
	Binding    string `json:"binding"`
	Name       string `json:"name"`
	ClassName  string `json:"class_name"`
	ScriptName string `json:"script_name"`
}

type wranglerR2 struct {
	Binding    string `json:"binding"`
	BucketName string `json:"bucket_name"`
}

func emptyBindings() *Bindings {
	return &Bindings{
		D1:        make([]D1Binding, 0),
		KV:        make([]KVNamespace, 0),
		Queues:    make([]QueueBinding, 0),
		Workflows: make([]WorkflowBinding, 0),
		R2:        make([]R2Bucket, 0),
		Crons:     make([]string, 0),
	}
}

// ParseBindings reads wrangler.jsonc / wrangler.json and returns the DESIGN §8.4 bindings list.
func ParseBindings(projectDir string) (*Bindings, error) {
	raw, err := readWranglerConfig(projectDir)
	if err != nil {
		return nil, err
	}
	var cfg wranglerFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse wrangler: %w", err)
	}

	out := emptyBindings()

	for _, db := range cfg.D1Databases {
		if db.Binding == "" {
			continue
		}
		out.D1 = append(out.D1, D1Binding{
			Binding:      db.Binding,
			DatabaseName: db.DatabaseName,
			DatabaseID:   db.DatabaseID,
		})
	}

	for _, ns := range cfg.KVNamespaces {
		if ns.Binding == "" || ns.ID == "" {
			continue
		}
		out.KV = append(out.KV, KVNamespace{
			Binding: ns.Binding,
			ID:      ns.ID,
		})
	}

	if cfg.Queues != nil {
		idx := make(map[string]int)
		for _, p := range cfg.Queues.Producers {
			if p.Binding == "" || p.Queue == "" {
				continue
			}
			if i, ok := idx[p.Queue]; ok {
				if out.Queues[i].Binding == "" {
					out.Queues[i].Binding = p.Binding
				}
				continue
			}
			out.Queues = append(out.Queues, QueueBinding{
				Name:     p.Queue,
				Binding:  p.Binding,
				Consumer: false,
			})
			idx[p.Queue] = len(out.Queues) - 1
		}
		for _, c := range cfg.Queues.Consumers {
			if c.Queue == "" {
				continue
			}
			if i, ok := idx[c.Queue]; ok {
				out.Queues[i].Consumer = true
				if out.Queues[i].DeadLetterQueue == nil && c.DeadLetterQueue != nil && *c.DeadLetterQueue != "" {
					dlq := *c.DeadLetterQueue
					out.Queues[i].DeadLetterQueue = &dlq
				}
				continue
			}
			qb := QueueBinding{
				Name:     c.Queue,
				Consumer: true,
			}
			if c.DeadLetterQueue != nil && *c.DeadLetterQueue != "" {
				dlq := *c.DeadLetterQueue
				qb.DeadLetterQueue = &dlq
			}
			out.Queues = append(out.Queues, qb)
			idx[c.Queue] = len(out.Queues) - 1
		}
	}

	for _, wf := range cfg.Workflows {
		if wf.Binding == "" || wf.Name == "" || wf.ClassName == "" {
			continue
		}
		out.Workflows = append(out.Workflows, WorkflowBinding{
			Binding:   wf.Binding,
			Name:      wf.Name,
			ClassName: wf.ClassName,
		})
	}

	for _, bucket := range cfg.R2Buckets {
		if bucket.Binding == "" || bucket.BucketName == "" {
			continue
		}
		out.R2 = append(out.R2, R2Bucket{
			Binding:    bucket.Binding,
			BucketName: bucket.BucketName,
		})
	}

	if cfg.Triggers != nil && cfg.Triggers.Crons != nil {
		out.Crons = append(out.Crons, cfg.Triggers.Crons...)
	}

	return out, nil
}

// ParseWranglerVars returns wrangler.json vars (plain_text Worker env).
func ParseWranglerVars(projectDir string) (map[string]string, error) {
	raw, err := readWranglerConfig(projectDir)
	if err != nil {
		return nil, err
	}
	var cfg wranglerFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse wrangler: %w", err)
	}
	out := map[string]string{}
	for k, v := range cfg.Vars {
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out, nil
}
