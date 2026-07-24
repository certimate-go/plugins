package main

import "encoding/json"

const (
	providerType = "webhook-deployer"
	accessType   = "webhook"

	displayNameKey = "plugin.webhook-deployer.name"

	schemaVersion = "form/v1"
)

type envelope struct {
	SchemaVersion string `json:"schemaVersion"`
	Provider      string `json:"provider"`
	Category      string `json:"category"`
	Schema        body   `json:"schema"`
}

type body struct {
	Columns []column `json:"columns"`
}

type column struct {
	Name           string   `json:"name"`
	ValueType      string   `json:"valueType"`
	LabelKey       string   `json:"labelKey,omitempty"`
	PlaceholderKey string   `json:"placeholderKey,omitempty"`
	TooltipKey     string   `json:"tooltipKey,omitempty"`
	Secret         bool     `json:"secret,omitempty"`
	Required       bool     `json:"required,omitempty"`
	Default        any      `json:"default,omitempty"`
	Options        []option `json:"options,omitempty"`
}

type option struct {
	Value    string `json:"value"`
	LabelKey string `json:"labelKey,omitempty"`
}

func envelopeJSON(provider, category string, cols []column) []byte {
	env := envelope{SchemaVersion: schemaVersion, Provider: provider, Category: category, Schema: body{Columns: cols}}
	b, _ := json.Marshal(env)
	return b
}

func accessSchemaJSON() []byte {
	return nil
}

func deploySchemaColumns() []column {
	return []column{
		{
			Name:      "method",
			ValueType: "select",
			LabelKey:  "plugin.webhook-deployer.deploy.method.label",
			Default:   "POST",
			Required:  true,
			Options: []option{
				{Value: "POST", LabelKey: "plugin.webhook-deployer.deploy.method.post"},
				{Value: "PUT", LabelKey: "plugin.webhook-deployer.deploy.method.put"},
			},
		},
		{Name: "path", ValueType: "text", LabelKey: "plugin.webhook-deployer.deploy.path.label", PlaceholderKey: "plugin.webhook-deployer.deploy.path.placeholder"},
	}
}

func deploySchemaJSON() []byte {
	return envelopeJSON(providerType, "deploy", deploySchemaColumns())
}

func i18nResources() map[string]map[string]string {
	return map[string]map[string]string{
		"zh": {
			"plugin.webhook-deployer.name":                    "Webhook 部署",
			"plugin.webhook-deployer.deploy.method.label":     "HTTP 方法",
			"plugin.webhook-deployer.deploy.method.post":      "POST",
			"plugin.webhook-deployer.deploy.method.put":       "PUT",
			"plugin.webhook-deployer.deploy.path.label":       "路径",
			"plugin.webhook-deployer.deploy.path.placeholder": "/cert",
		},
		"en": {
			"plugin.webhook-deployer.name":                    "Webhook Deployer",
			"plugin.webhook-deployer.deploy.method.label":     "HTTP Method",
			"plugin.webhook-deployer.deploy.method.post":      "POST",
			"plugin.webhook-deployer.deploy.method.put":       "PUT",
			"plugin.webhook-deployer.deploy.path.label":       "Path",
			"plugin.webhook-deployer.deploy.path.placeholder": "/cert",
		},
	}
}
