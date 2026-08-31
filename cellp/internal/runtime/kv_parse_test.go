package runtime

import (
	"testing"
)

func TestParseKVListNDJSONInvalid(t *testing.T) {
	_, err := parseKVListNDJSON([]byte(`{not json}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsKVNoKey(t *testing.T) {
	if !isKVNoKey(errKV("celld kv get: no key here")) {
		t.Fatal("expected no key")
	}
	if isKVNoKey(nil) {
		t.Fatal("nil")
	}
}

type errKV string

func (e errKV) Error() string { return string(e) }
