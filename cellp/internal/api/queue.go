package api

import (
	"os"
	"strconv"
)

const defaultQueueMax = 10000

// QueueMax returns deploy queue depth limit from CELLP_QUEUE_MAX (default 10000).
func QueueMax() int {
	if v := os.Getenv("CELLP_QUEUE_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultQueueMax
}
