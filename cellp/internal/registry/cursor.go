package registry

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

const cursorSep = "\x00"

// EncodeCursor encodes created_at and id into an opaque pagination cursor.
func EncodeCursor(createdAt time.Time, id string) string {
	payload := createdAt.UTC().Format(time.RFC3339Nano) + cursorSep + id
	return base64.URLEncoding.EncodeToString([]byte(payload))
}

// DecodeCursor decodes a pagination cursor into created_at and id.
func DecodeCursor(cursor string) (time.Time, string, error) {
	if cursor == "" {
		return time.Time{}, "", fmt.Errorf("empty cursor")
	}
	raw, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor encoding: %w", err)
	}
	parts := strings.SplitN(string(raw), cursorSep, 2)
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", fmt.Errorf("invalid cursor payload")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor timestamp: %w", err)
	}
	return createdAt, parts[1], nil
}
