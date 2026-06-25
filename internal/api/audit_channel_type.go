package api

import (
	"encoding/json"
	"strings"
)

func resolveAuditChannelType(service, channel string, raw map[string]any) (string, string) {
	channelName := stripAuditChannelID(channel)
	name := stringValue(raw["Name"])
	group := stringValue(raw["Group"])
	tag := stringValue(raw["Tag"])
	other := extractAuditOtherText(raw["Other"])

	switch {
	case hasAuditChannelPrefix(channelName, "O-"), hasAuditChannelPrefix(name, "O-"):
		return "official", "官方直连"
	case hasAuditChannelPrefix(channelName, "M-"), hasAuditChannelPrefix(name, "M-"):
		return "mixed", "混合"
	case hasAuditChannelPrefix(channelName, "R-"), hasAuditChannelPrefix(name, "R-"):
		return "reverse", "逆向"
	}

	haystack := strings.ToLower(strings.Join([]string{
		service,
		channel,
		channelName,
		name,
		group,
		tag,
		other,
	}, " "))

	switch {
	case containsAny(haystack, "官key直连", "官方直连", "official"):
		return "official", "官方直连"
	case containsAny(haystack, "混合", "mixed"):
		return "mixed", "混合"
	case containsAny(haystack, "逆向", "reverse"):
		return "reverse", "逆向"
	default:
		return "unknown", "未知"
	}
}

func stripAuditChannelID(channel string) string {
	text := strings.TrimSpace(channel)
	if idx := strings.Index(text, ":"); idx >= 0 {
		text = text[idx+1:]
	}
	return strings.TrimSpace(text)
}

func hasAuditChannelPrefix(value, prefix string) bool {
	text := strings.ToUpper(strings.TrimSpace(value))
	return strings.HasPrefix(text, strings.ToUpper(prefix))
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, strings.ToLower(strings.TrimSpace(needle))) {
			return true
		}
	}
	return false
}

func stringValue(v any) string {
	switch vv := v.(type) {
	case string:
		return strings.TrimSpace(vv)
	case json.RawMessage:
		return strings.TrimSpace(string(vv))
	default:
		return ""
	}
}

func extractAuditOtherText(v any) string {
	switch vv := v.(type) {
	case string:
		return parseAuditOtherString(vv)
	case map[string]any:
		return strings.Join([]string{
			stringValue(vv["provider"]),
			stringValue(vv["provider_name"]),
			stringValue(vv["service"]),
			stringValue(vv["service_type"]),
			stringValue(vv["tag"]),
		}, " ")
	default:
		return ""
	}
}

func parseAuditOtherString(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		return text
	}
	return strings.Join([]string{
		stringValue(m["provider"]),
		stringValue(m["provider_name"]),
		stringValue(m["service"]),
		stringValue(m["service_type"]),
		stringValue(m["tag"]),
	}, " ")
}
