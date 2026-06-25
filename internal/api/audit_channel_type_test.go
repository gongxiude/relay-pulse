package api

import "testing"

func TestResolveAuditChannelType(t *testing.T) {
	tests := []struct {
		name      string
		service   string
		channel   string
		raw       map[string]any
		wantType  string
		wantLabel string
	}{
		{
			name:      "official by direct keyword",
			service:   "openai",
			channel:   "80:alan-官key直连",
			raw:       map[string]any{"Name": "alan-官key直连", "Group": "openai"},
			wantType:  "official",
			wantLabel: "官方直连",
		},
		{
			name:      "reverse by reverse prefix",
			service:   "anthropic",
			channel:   "91:R-my-channel",
			raw:       map[string]any{"Name": "R-my-channel", "Group": "anthropic"},
			wantType:  "reverse",
			wantLabel: "逆向",
		},
		{
			name:      "mixed by mixed prefix",
			service:   "openai",
			channel:   "92:M-mixed-fallback",
			raw:       map[string]any{"Name": "M-mixed-fallback", "Group": "openai"},
			wantType:  "mixed",
			wantLabel: "混合",
		},
		{
			name:      "unknown fallback",
			service:   "anthropic",
			channel:   "81:alan-号池",
			raw:       map[string]any{"Name": "alan-号池", "Group": "模型渠道测试分组,anthropic"},
			wantType:  "unknown",
			wantLabel: "未知",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotLabel := resolveAuditChannelType(tc.service, tc.channel, tc.raw)
			if gotType != tc.wantType || gotLabel != tc.wantLabel {
				t.Fatalf("got (%q, %q), want (%q, %q)", gotType, gotLabel, tc.wantType, tc.wantLabel)
			}
		})
	}
}
