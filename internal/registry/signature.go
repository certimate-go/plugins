package registry

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/certimate-go/plugins/internal/providerrelease"
)

const (
	IndexFormatVersion = 1
	IndexDomain        = "certimate:plugin-market:index:v1\x00"
	MaxIndexPayload    = 2 << 20
	MaxIndexEnvelope   = 3 << 20
	MaxIndexLifetime   = 7 * 24 * time.Hour
)

var keyIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)

type IndexMetadata struct {
	Epoch        uint32    `json:"epoch"`
	Generation   uint64    `json:"generation"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	SourceCommit string    `json:"source_commit"`
}

type IndexPayload struct {
	FormatVersion uint32 `json:"format_version"`
	IndexMetadata
	Catalog
}

type IndexEnvelope struct {
	KeyID     string `json:"key_id"`
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"`
}

func SignIndex(snapshot Snapshot, metadata IndexMetadata, keyID string, seed []byte) (IndexEnvelope, error) {
	if !keyIDPattern.MatchString(keyID) || len(seed) != ed25519.SeedSize {
		return IndexEnvelope{}, errors.New("invalid signing key ID or Ed25519 seed")
	}
	if metadata.Epoch == 0 || metadata.Generation == 0 || !commitPattern.MatchString(metadata.SourceCommit) {
		return IndexEnvelope{}, errors.New("invalid index epoch, generation or source commit")
	}
	metadata.IssuedAt = metadata.IssuedAt.UTC()
	metadata.ExpiresAt = metadata.ExpiresAt.UTC()
	if !validIndexWindow(metadata) {
		return IndexEnvelope{}, errors.New("invalid index issuance or expiration time")
	}
	if snapshot.Catalog.Plugins == nil {
		return IndexEnvelope{}, errors.New("snapshot must contain a generated catalog")
	}
	payload, err := json.Marshal(IndexPayload{IndexFormatVersion, metadata, snapshot.Catalog})
	if err != nil {
		return IndexEnvelope{}, err
	}
	if len(payload) > MaxIndexPayload {
		return IndexEnvelope{}, errors.New("index payload exceeds size limit")
	}
	key := ed25519.NewKeyFromSeed(seed)
	defer clear(key)
	signature := ed25519.Sign(key, append([]byte(IndexDomain), payload...))
	return IndexEnvelope{KeyID: keyID, Payload: payload, Signature: signature}, nil
}

func validIndexWindow(metadata IndexMetadata) bool {
	return metadata.IssuedAt.Year() >= 2000 && metadata.ExpiresAt.After(metadata.IssuedAt) &&
		metadata.ExpiresAt.Sub(metadata.IssuedAt) <= MaxIndexLifetime
}

func VerifyIndex(body []byte, keys map[string]ed25519.PublicKey) (IndexPayload, string, error) {
	var envelope IndexEnvelope
	if len(body) == 0 || len(body) > MaxIndexEnvelope {
		return IndexPayload{}, "", errors.New("invalid market index envelope size")
	}
	if err := decodeJSON(body, &envelope); err != nil {
		return IndexPayload{}, "", err
	}
	key := keys[envelope.KeyID]
	if !keyIDPattern.MatchString(envelope.KeyID) || len(key) != ed25519.PublicKeySize || len(envelope.Payload) == 0 ||
		len(envelope.Payload) > MaxIndexPayload ||
		!ed25519.Verify(key, append([]byte(IndexDomain), envelope.Payload...), envelope.Signature) {
		return IndexPayload{}, "", errors.New("invalid market index signing key or signature")
	}
	var payload IndexPayload
	if err := decodeJSON(envelope.Payload, &payload); err != nil {
		return IndexPayload{}, "", err
	}
	if payload.FormatVersion != IndexFormatVersion || payload.Epoch == 0 || payload.Generation == 0 ||
		!commitPattern.MatchString(payload.SourceCommit) || !validIndexWindow(payload.IndexMetadata) || payload.Plugins == nil {
		return IndexPayload{}, "", errors.New("invalid signed index metadata")
	}
	seen := make(map[string]bool, len(payload.Plugins))
	for _, plugin := range payload.Plugins {
		if err := checkIdentity(plugin.Identity); err != nil {
			return IndexPayload{}, "", err
		}
		if providerrelease.ValidateRepository(plugin.Repository) != nil ||
			!commitPattern.MatchString(plugin.ReleaseCommit) || seen[plugin.ProviderType] {
			return IndexPayload{}, "", errors.New("invalid or duplicate signed plugin summary")
		}
		seen[plugin.ProviderType] = true
	}
	return payload, envelope.KeyID, nil
}
