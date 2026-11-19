package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func TestGetConfigSchema_ReturnsParseableDeploySchema(t *testing.T) {
	d := &myDeployer{}
	cs, err := d.GetConfigSchema(context.Background())
	if err != nil {
		t.Fatalf("GetConfigSchema: %v", err)
	}
	if cs.ProviderSchemaJSON == nil {
		t.Fatal("expected non-nil deploy schema")
	}
	var env struct {
		SchemaVersion string `json:"schemaVersion"`
		Provider      string `json:"provider"`
		Category      string `json:"category"`
		Schema        struct {
			Columns []struct {
				Name           string `json:"name"`
				ValueType      string `json:"valueType"`
				LabelKey       string `json:"labelKey,omitempty"`
				PlaceholderKey string `json:"placeholderKey,omitempty"`
				TooltipKey     string `json:"tooltipKey,omitempty"`
				Required       bool   `json:"required,omitempty"`
			} `json:"columns"`
		} `json:"schema"`
	}
	if err := json.Unmarshal(cs.ProviderSchemaJSON, &env); err != nil {
		t.Fatalf("deploy schema not valid JSON: %v", err)
	}
	if env.SchemaVersion != "form/v1" {
		t.Errorf("expected schemaVersion form/v1, got %q", env.SchemaVersion)
	}
	if len(env.Schema.Columns) == 0 {
		t.Error("expected at least one column in deploy schema")
	}
}

func TestProcess_ReturnsDeployResult(t *testing.T) {
	d := &myDeployer{}
	res, err := d.Process(context.Background(), &plugin.Request{
		Capability: plugin.CapabilityDeploy,
		Deploy:     &plugin.DeployPayload{},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.Deploy == nil {
		t.Fatal("expected a deploy result")
	}
}
