package runtime

import (
	"context"
	"testing"
)

func TestExecCelldSuccess(t *testing.T) {
	installFakeOperatorCelld(t)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	out, err := m.execCelld(context.Background(), "demo", "v1", []string{"queue", "info", "tasks"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("expected output")
	}
}

func TestCelldErrPrefixCases(t *testing.T) {
	if celldErrPrefix([]string{"kv", "get"}) != "celld kv get" {
		t.Fatal()
	}
	if celldErrPrefix([]string{"deploy"}) != "celld deploy" {
		t.Fatal()
	}
	if celldErrPrefix(nil) != "celld" {
		t.Fatal()
	}
}
