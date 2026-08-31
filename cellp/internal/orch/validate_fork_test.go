package orch

import "testing"

func TestValidateForkProdCases(t *testing.T) {
	prod := "v-prod"
	parent := prod
	if err := ValidateForkProd(&parent, &prod, "main"); err != nil {
		t.Fatal("main fork from prod ok")
	}
	if err := ValidateForkProd(&parent, &prod, "refs/pull/1/merge"); err == nil {
		t.Fatal("pr fork blocked")
	}
	if err := ValidateForkProd(&parent, &prod, "pr/123"); err == nil {
		t.Fatal("pr/ prefix blocked")
	}
	if err := ValidateForkProd(nil, &prod, "refs/pull/1"); err != nil {
		t.Fatal("nil parent ok")
	}
	other := "v-other"
	if err := ValidateForkProd(&other, &prod, "refs/pull/1"); err != nil {
		t.Fatal("non-prod parent ok")
	}
}
