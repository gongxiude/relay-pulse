package onboarding

import (
	"context"
	"strings"
	"testing"
)

// TestValidateChannelTypeSource 锁定「通道类型↔来源类别」自洽校验：
// 来源须在 service 词表内，且其 Category 落在该 channelType 的允许集合中。
func TestValidateChannelTypeSource(t *testing.T) {
	cases := []struct {
		name        string
		channelType string
		service     string
		source      string
		wantErr     bool
	}{
		{"O+max 订阅合法", "O", "cc", "max", false},
		{"O+api 官方合法", "O", "cc", "api", false},
		{"O+aws 云合法", "O", "cc", "aws", false},
		{"O+kiro 逆向不合法", "O", "cc", "kiro", true},
		{"O+mix 混合不合法", "O", "cc", "mix", true},
		{"R+kiro 逆向合法", "R", "cc", "kiro", false},
		{"R+max 订阅不合法", "R", "cc", "max", true},
		{"M+mix 混合合法", "M", "cc", "mix", false},
		{"M+max 订阅不合法", "M", "cc", "max", true},
		{"M+kiro 逆向不合法", "M", "cc", "kiro", true},
		{"非法 source 先被词表拒绝", "O", "cc", "zzz", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateChannelTypeSource(tc.channelType, tc.service, tc.source)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望拒绝 %s+%s，却通过返回 %q", tc.channelType, tc.source, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("期望通过 %s+%s，却报错: %v", tc.channelType, tc.source, err)
			}
			if got != tc.source {
				t.Errorf("规范化 source 应为 %q，实际 %q", tc.source, got)
			}
		})
	}
}

// TestCatalogCategoriesAllReachable 守护「类型↔来源」映射与词表不漂移：
// ChannelSourceCatalog 里每个来源的 Category 必须落在某个 channelType 的允许集合中，
// 否则该来源永远无法被任何类型选中（死项）。新增来源 category 写错时此测试即报警。
func TestCatalogCategoriesAllReachable(t *testing.T) {
	reachable := make(map[string]bool)
	for _, cats := range channelTypeAllowedCategories {
		for _, c := range cats {
			reachable[c] = true
		}
	}
	for service, opts := range ChannelSourceCatalog {
		for _, opt := range opts {
			if !reachable[opt.Category] {
				t.Errorf("service=%q 来源 %q 的 category=%q 不在任何 channelType 允许集合中（死项）；"+
					"请检查 channelTypeAllowedCategories 或该来源的 Category", service, opt.Value, opt.Category)
			}
		}
	}
}

// TestSubmit_RejectsUnacceptedAgreement 验证未确认协议的提交在前置环节即被拒，
// 早于 provider/来源校验与 IP 限流 / proof 校验，不消耗配额也不暴露后续字段细节。
func TestSubmit_RejectsUnacceptedAgreement(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Submit(context.Background(), &SubmitRequest{
		AgreementAccepted: false,
	}, "1.2.3.4")
	if err == nil || !strings.Contains(err.Error(), "入驻须知") {
		t.Fatalf("未确认协议应被拒绝，实际 err=%v", err)
	}
}

// TestSubmit_RejectsTypeSourceMismatch 验证 Submit 路径也接入类型↔来源自洽校验：
// 官方通道(O)选逆向来源(kiro) 在 validateChannelTypeSource 处被拒（早于 IP/proof）。
func TestSubmit_RejectsTypeSourceMismatch(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Submit(context.Background(), &SubmitRequest{
		AgreementAccepted: true,
		ProviderName:      "Prov",
		ServiceType:       "cc",
		ChannelType:       "O",
		ChannelSource:     "kiro",
	}, "1.2.3.4")
	if err == nil {
		t.Fatalf("O+kiro 应在类型↔来源自洽校验处被拒")
	}
}
