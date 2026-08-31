package orch

import (
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestCronShouldArm(t *testing.T) {
	vProd := "v-prod"
	vPreview := "v-preview"
	tests := []struct {
		name string
		proj *registry.Project
		vid  string
		want bool
	}{
		{"nil project", nil, vPreview, true},
		{"no prod yet", &registry.Project{ID: "p"}, vPreview, true},
		{"preview when prod set", &registry.Project{ID: "p", ProdVersionID: &vProd}, vPreview, false},
		{"prod version", &registry.Project{ID: "p", ProdVersionID: &vProd}, vProd, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CronShouldArm(tc.proj, tc.vid); got != tc.want {
				t.Fatalf("CronShouldArm() = %v, want %v", got, tc.want)
			}
		})
	}
}
