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
	if cs.DeploySchemaJSON == nil {
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
	if err := json.Unmarshal(cs.DeploySchemaJSON, &env); err != nil {
		t.Fatalf("deploy schema not valid JSON: %v", err)
	}
	if env.SchemaVersion != "form/v1" {
		t.Errorf("expected schemaVersion form/v1, got %q", env.SchemaVersion)
	}
	if len(env.Schema.Columns) == 0 {
		t.Error("expected at least one column in deploy schema")
	}
}

func TestGetConfigSchema_I18nCoverage(t *testing.T) {
	d := &myDeployer{}
	cs, err := d.GetConfigSchema(context.Background())
	if err != nil {
		t.Fatalf("GetConfigSchema: %v", err)
	}

	var env struct {
		Schema struct {
			Columns []struct {
				LabelKey       string `json:"labelKey,omitempty"`
				PlaceholderKey string `json:"placeholderKey,omitempty"`
				TooltipKey     string `json:"tooltipKey,omitempty"`
			} `json:"columns"`
		} `json:"schema"`
	}
	if err := json.Unmarshal(cs.DeploySchemaJSON, &env); err != nil {
		t.Fatalf("deploy schema not valid JSON: %v", err)
	}

	locales := []string{"zh", "en"}
	for _, col := range env.Schema.Columns {
		for _, key := range []string{col.LabelKey, col.PlaceholderKey, col.TooltipKey} {
			if key == "" {
				continue
			}
			for _, locale := range locales {
				if cs.I18n[locale] == nil {
					t.Errorf("locale %q not found in i18n", locale)
					continue
				}
				if cs.I18n[locale][key] == "" {
					t.Errorf("i18n key %q missing for locale %q", key, locale)
				}
			}
		}
	}
}

func TestDeploy_ReturnsNotImplemented(t *testing.T) {
	d := &myDeployer{}
	_, err := d.Deploy(context.Background(), &plugin.DeployRequest{})
	if err == nil {
		t.Fatal("expected not-implemented error")
	}
}
