package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"monitor/internal/storage"
)

type TargetStore interface {
	ReplaceAuditTargets([]storage.AuditTarget) error
	SaveChannelSnapshot(*storage.ChannelSnapshot) error
}

type ChannelSpec struct {
	ID           int
	Type         int
	Status       int
	Name         string
	Models       string
	Group        string
	Weight       *uint
	Priority     *int64
	ModelMapping *string
	Other        json.RawMessage
}

func BuildAuditTargets(channels []ChannelSpec) []storage.AuditTarget {
	targets := make([]storage.AuditTarget, 0, len(channels))
	for _, ch := range channels {
		provider := resolveProvider(ch)
		service := resolveService(ch)
		channelID := fmt.Sprintf("%d", ch.ID)
		channelName := strings.TrimSpace(ch.Name)
		channelKey := channelID
		if channelName != "" {
			channelKey = channelID + ":" + channelName
		}
		models := expandChannelModels(ch)
		enabled := ch.Status == 1
		for _, pair := range models {
			targets = append(targets, storage.AuditTarget{
				Provider:     provider,
				Service:      service,
				Channel:      channelKey,
				Model:        pair.DisplayModel,
				RequestModel: pair.RequestModel,
				Group:        strings.TrimSpace(ch.Group),
				Weight:       int(derefUint(ch.Weight)),
				Priority:     int(derefInt64(ch.Priority)),
				Enabled:      enabled,
			})
		}
	}
	sort.SliceStable(targets, func(i, j int) bool {
		if targets[i].Provider != targets[j].Provider {
			return targets[i].Provider < targets[j].Provider
		}
		if targets[i].Service != targets[j].Service {
			return targets[i].Service < targets[j].Service
		}
		if targets[i].Channel != targets[j].Channel {
			return targets[i].Channel < targets[j].Channel
		}
		if targets[i].Model != targets[j].Model {
			return targets[i].Model < targets[j].Model
		}
		return targets[i].RequestModel < targets[j].RequestModel
	})
	return targets
}

func SyncTargets(_ context.Context, channels []ChannelSpec, store TargetStore) ([]storage.AuditTarget, error) {
	if store == nil {
		return nil, fmt.Errorf("target store is nil")
	}
	targets := BuildAuditTargets(channels)
	if err := store.ReplaceAuditTargets(targets); err != nil {
		return nil, err
	}
	for _, ch := range channels {
		snap := &storage.ChannelSnapshot{
			NewAPIChannelID: int64(ch.ID),
			SnapshotAt:      nowUnix(),
			Provider:        resolveProvider(ch),
			Service:         resolveService(ch),
			Channel:         fmt.Sprintf("%d:%s", ch.ID, strings.TrimSpace(ch.Name)),
			Model:           strings.Join(uniqueModels(expandChannelModels(ch)), ","),
			Enabled:         ch.Status == 1,
			Raw:             mustMarshalChannel(ch),
		}
		if err := store.SaveChannelSnapshot(snap); err != nil {
			return nil, err
		}
	}
	return targets, nil
}

type modelPair struct {
	RequestModel string
	DisplayModel string
}

func expandChannelModels(ch ChannelSpec) []modelPair {
	models := splitModels(ch.Models)
	mapping := parseModelMapping(ch.ModelMapping)
	out := make([]modelPair, 0, len(models)+len(mapping))

	seen := make(map[string]struct{})
	for _, m := range models {
		if req, ok := mapping[m]; ok {
			out = append(out, modelPair{RequestModel: m, DisplayModel: req})
			seen[m] = struct{}{}
			seen[req] = struct{}{}
			continue
		}
		out = append(out, modelPair{RequestModel: m, DisplayModel: m})
		seen[m] = struct{}{}
	}
	for req, display := range mapping {
		if _, ok := seen[req]; ok {
			continue
		}
		if display == "" {
			display = req
		}
		out = append(out, modelPair{RequestModel: req, DisplayModel: display})
	}
	return out
}

func parseModelMapping(raw *string) map[string]string {
	if raw == nil {
		return nil
	}
	text := strings.TrimSpace(*raw)
	if text == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		return nil
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func splitModels(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', '\n', '\t', ' ':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func uniqueModels(pairs []modelPair) []string {
	out := make([]string, 0, len(pairs))
	seen := make(map[string]struct{})
	for _, p := range pairs {
		if _, ok := seen[p.DisplayModel]; ok {
			continue
		}
		seen[p.DisplayModel] = struct{}{}
		out = append(out, p.DisplayModel)
	}
	return out
}

func resolveProvider(ch ChannelSpec) string {
	if v := strings.TrimSpace(extractStringField(ch.Other, "provider", "provider_name", "supplier", "vendor")); v != "" {
		return v
	}
	if v := strings.TrimSpace(ch.Name); v != "" {
		return v
	}
	if ch.Type != 0 {
		return "type-" + strconv.Itoa(ch.Type)
	}
	return "unknown"
}

func resolveService(ch ChannelSpec) string {
	if v := strings.TrimSpace(extractStringField(ch.Other, "service", "service_type")); v != "" {
		return v
	}
	for _, candidate := range []string{ch.Group, ch.Models, ch.Name} {
		if service := inferAuditService(candidate); service != "" {
			return service
		}
	}
	if ch.Type != 0 {
		return "type-" + strconv.Itoa(ch.Type)
	}
	return "default"
}

func inferAuditService(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return ""
	}
	switch {
	case strings.Contains(text, "anthropic"), strings.Contains(text, "claude"):
		return "anthropic"
	case strings.Contains(text, "openai"), strings.Contains(text, "gpt"), strings.Contains(text, "chatgpt"):
		return "openai"
	case strings.Contains(text, "gemini"), strings.Contains(text, "google"):
		return "gemini"
	}
	return ""
}

func extractStringField(raw json.RawMessage, keys ...string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch vv := v.(type) {
			case string:
				return vv
			case fmt.Stringer:
				return vv.String()
			}
		}
	}
	return ""
}

func mustMarshalChannel(ch ChannelSpec) json.RawMessage {
	b, err := json.Marshal(ch)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

func derefUint(v *uint) uint {
	if v == nil {
		return 0
	}
	return *v
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func nowUnix() int64 {
	return timeNow().Unix()
}

var timeNow = func() time.Time { return time.Now() }
