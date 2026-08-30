package runtime

import (
	"encoding/json"
	"fmt"
	"os"
)

// CopyBindingIdentitiesFromParent aligns child wrangler binding ids with the parent
// (D1 database_id, kv_namespaces[].id, r2_buckets[].bucket_name, queue names).
func CopyBindingIdentitiesFromParent(parentDir, childDir string) error {
	parent, err := ParseBindings(parentDir)
	if err != nil {
		return fmt.Errorf("parent wrangler: %w", err)
	}
	child, err := ParseBindings(childDir)
	if err != nil {
		return fmt.Errorf("child wrangler: %w", err)
	}

	if len(parent.D1) > 0 {
		if len(child.D1) == 0 {
			return fmt.Errorf("child has no d1_databases but parent declares D1")
		}
		if err := SetD1DatabaseID(childDir, parent.D1[0].DatabaseID); err != nil {
			return err
		}
	}

	if len(parent.KV) > 0 {
		if len(child.KV) != len(parent.KV) {
			return fmt.Errorf("child kv_namespaces count %d != parent %d", len(child.KV), len(parent.KV))
		}
		if err := setKVNamespaceIDs(childDir, parent.KV); err != nil {
			return err
		}
	}

	if len(parent.R2) > 0 {
		if len(child.R2) != len(parent.R2) {
			return fmt.Errorf("child r2_buckets count %d != parent %d", len(child.R2), len(parent.R2))
		}
		if err := setR2BucketNames(childDir, parent.R2); err != nil {
			return err
		}
	}

	if len(parent.Queues) > 0 {
		parentNames := queueBranchNames(parent.Queues)
		childNames := queueBranchNames(child.Queues)
		if len(childNames) != len(parentNames) {
			return fmt.Errorf("child queue count %d != parent %d", len(childNames), len(parentNames))
		}
		for i := range parentNames {
			if childNames[i] != parentNames[i] {
				return fmt.Errorf("child queue %q != parent %q", childNames[i], parentNames[i])
			}
		}
		if err := setQueueNames(childDir, parent.Queues); err != nil {
			return err
		}
	}

	return nil
}

func queueBranchNames(queues []QueueBinding) []string {
	seen := make(map[string]struct{})
	var names []string
	for _, q := range queues {
		if q.Name == "" {
			continue
		}
		if _, ok := seen[q.Name]; ok {
			continue
		}
		seen[q.Name] = struct{}{}
		names = append(names, q.Name)
	}
	return names
}

func setKVNamespaceIDs(projectDir string, namespaces []KVNamespace) error {
	path, raw, err := readWranglerConfigFile(projectDir)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse wrangler: %w", err)
	}
	kvs, ok := cfg["kv_namespaces"].([]any)
	if !ok || len(kvs) == 0 {
		return fmt.Errorf("wrangler has no kv_namespaces in %s", projectDir)
	}
	for i, ns := range namespaces {
		if i >= len(kvs) {
			break
		}
		entry, ok := kvs[i].(map[string]any)
		if !ok {
			return fmt.Errorf("kv_namespaces[%d] is not an object", i)
		}
		entry["id"] = ns.ID
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func setR2BucketNames(projectDir string, buckets []R2Bucket) error {
	path, raw, err := readWranglerConfigFile(projectDir)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse wrangler: %w", err)
	}
	r2s, ok := cfg["r2_buckets"].([]any)
	if !ok || len(r2s) == 0 {
		return fmt.Errorf("wrangler has no r2_buckets in %s", projectDir)
	}
	for i, b := range buckets {
		if i >= len(r2s) {
			break
		}
		entry, ok := r2s[i].(map[string]any)
		if !ok {
			return fmt.Errorf("r2_buckets[%d] is not an object", i)
		}
		entry["bucket_name"] = b.BucketName
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func setQueueNames(projectDir string, queues []QueueBinding) error {
	path, raw, err := readWranglerConfigFile(projectDir)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse wrangler: %w", err)
	}
	queuesObj, ok := cfg["queues"].(map[string]any)
	if !ok {
		return fmt.Errorf("wrangler has no queues in %s", projectDir)
	}
	nameByBinding := make(map[string]string)
	for _, q := range queues {
		if q.Binding != "" && q.Name != "" {
			nameByBinding[q.Binding] = q.Name
		}
	}
	if producers, ok := queuesObj["producers"].([]any); ok {
		for _, p := range producers {
			entry, ok := p.(map[string]any)
			if !ok {
				continue
			}
			binding, _ := entry["binding"].(string)
			if name, ok := nameByBinding[binding]; ok {
				entry["queue"] = name
			}
		}
	}
	if consumers, ok := queuesObj["consumers"].([]any); ok {
		for _, c := range consumers {
			entry, ok := c.(map[string]any)
			if !ok {
				continue
			}
			if qn, ok := entry["queue"].(string); ok {
				for _, pq := range queues {
					if pq.Name == qn {
						entry["queue"] = pq.Name
					}
				}
			}
		}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
