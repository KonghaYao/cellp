package config

import (
	"os"
	"testing"

	"github.com/cellp/cellp/internal/elastic/contract"
)

func TestLoadServingDefaults_scaleToZeroOffForcesMinOne(t *testing.T) {
	t.Setenv(envServingScaleToZero, "0")
	t.Setenv(envServingDefaultMinReplicas, "0")
	d := LoadServingDefaults()
	if d.ScaleToZeroEnabled {
		t.Fatal("expected scale-to-zero disabled")
	}
	if got := d.EffectiveDefaultMinReplicas(); got != 1 {
		t.Fatalf("min=%d want 1", got)
	}
}

func TestLoadServingDefaults_idleAfterDeploy(t *testing.T) {
	t.Setenv(envServingIdleAfterDeploy, "false")
	d := LoadServingDefaults()
	if d.IdleAfterDeploy {
		t.Fatal("expected idle after deploy off")
	}
}

func TestLoadServingDefaults_backgroundResidentForcesMin(t *testing.T) {
	t.Setenv(envServingDefaultBackground, "resident_required")
	t.Setenv(envServingDefaultMinReplicas, "0")
	t.Setenv(envServingScaleToZero, "1")
	d := LoadServingDefaults()
	if d.DefaultBackground != contract.BackgroundModeResidentRequired {
		t.Fatalf("bg=%q", d.DefaultBackground)
	}
	if got := d.EffectiveDefaultMinReplicas(); got != 1 {
		t.Fatalf("min=%d want 1", got)
	}
}

func TestLoadServingDefaults_prodScaleToZero(t *testing.T) {
	os.Unsetenv(envServingProdScaleToZero)
	d := LoadServingDefaults()
	if d.ProdScaleToZero {
		t.Fatal("default prod scale-to-zero should be false")
	}
	t.Setenv(envServingProdScaleToZero, "1")
	d2 := LoadServingDefaults()
	if !d2.ProdScaleToZero {
		t.Fatal("expected prod scale-to-zero on")
	}
}
