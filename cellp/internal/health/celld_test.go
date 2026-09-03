package health_test

import (
	"testing"

	"github.com/cellp/cellp/internal/health"
)

func TestCelldHealthResponseOK(t *testing.T) {
	if !health.CelldHealthResponseOK(200, []byte(`{"ok":true}`)) {
		t.Fatal("expected ok")
	}
	if health.CelldHealthResponseOK(200, []byte{}) {
		t.Fatal("empty body must not pass")
	}
	if health.CelldHealthResponseOK(200, []byte(`{"ok":false}`)) {
		t.Fatal("ok:false must not pass")
	}
}
