package notifier

import (
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"

	"github.com/certimate-go/certimate/pkg/plugin"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"
)

func NewClient() *resty.Client {
	return resty.New().
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", "Certimate")
}

func DecodeAccess(req *plugin.NotifyRequest, config any) error {
	var access map[string]any
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return fmt.Errorf("invalid notification access configuration: %w", err)
	}
	if err := xmaps.Populate(access, config); err != nil {
		return fmt.Errorf("invalid notification access configuration: %w", err)
	}
	return nil
}

func Destination(req *plugin.NotifyRequest, field, fallback string) string {
	var config map[string]any
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &config); err != nil {
		return fallback
	}
	return xmaps.GetOrDefaultString(config, field, fallback)
}

func WebhookPayload(req *plugin.NotifyRequest, custom string, fallback map[string]any) (map[string]any, error) {
	if custom == "" {
		return fallback, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(custom), &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal custom payload: %w", err)
	}
	if payload == nil {
		return nil, fmt.Errorf("custom payload must be a JSON object")
	}
	xmaps.DeepReplaceValue(payload, "${CERTIMATE_NOTIFIER_SUBJECT}", req.Subject)
	xmaps.DeepReplaceValue(payload, "${CERTIMATE_NOTIFIER_MESSAGE}", req.Message)
	xmaps.DeepReplaceValue(payload, "${SUBJECT}", req.Subject)
	xmaps.DeepReplaceValue(payload, "${MESSAGE}", req.Message)
	return payload, nil
}
