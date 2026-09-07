package runtime

import "testing"

func TestCelldListenFlagUsesBindHost(t *testing.T) {
	if celldListenFlag("10.0.0.5", 8803) != "10.0.0.5:8803" {
		t.Fatal("listen flag must use bind host only")
	}
}
