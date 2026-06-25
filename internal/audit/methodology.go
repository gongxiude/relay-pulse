package audit

import "strings"

type MethodologyDimension struct {
	Key         string `json:"key"`
	Weight      int    `json:"weight"`
	Group       string `json:"group"`
	Description string `json:"description"`
	Implemented bool   `json:"implemented"`
	Active      bool   `json:"active"`
	Phase       string `json:"phase"`
}

type MethodologySpec struct {
	Version           string                 `json:"version"`
	WeightsHash       string                 `json:"weights_hash"`
	TotalWeight       int                    `json:"total_weight"`
	Dimensions        []MethodologyDimension `json:"dimensions"`
	ImplementedCount  int                    `json:"implemented_count"`
	ActiveCount       int                    `json:"active_count"`
	ImplementedWeight int                    `json:"implemented_weight"`
	ActiveWeight      int                    `json:"active_weight"`
}

func CurrentMethodologySpec() MethodologySpec {
	dimensions := []MethodologyDimension{
		{Key: "cache_hit_ratio_match", Weight: 20, Group: "模型与缓存", Description: "比较 cache_read / total_input 与官方基线的接近程度", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "cache_continuity_intra", Weight: 14, Group: "模型与缓存", Description: "判断相邻步骤上下文与缓存连续性", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "model_match", Weight: 14, Group: "模型与缓存", Description: "比较请求模型与响应模型族是否一致", Implemented: true, Active: true, Phase: "phase1"},
		{Key: "cache_sliding_correctness", Weight: 13, Group: "模型与缓存", Description: "判断 5 分钟 sliding cache 行为是否接近基线", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "cache_ttl_consistency", Weight: 15, Group: "模型与缓存", Description: "检查 5m/1h cache 字段与表面行为是否一致", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "self_identity_consistency", Weight: 8, Group: "身份与知识", Description: "比较结构化身份自报与会话内自我描述是否一致", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "envelope_self_report_match", Weight: 3, Group: "身份与知识", Description: "比较响应 envelope 中自报信息与正文身份是否一致", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "thinking_present", Weight: 4, Group: "身份与知识", Description: "检查是否暴露 thinking 块与对应信号", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "thinking_volume_match", Weight: 6, Group: "身份与知识", Description: "比较 thinking 体量与基线的接近程度", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "tier_thinking_volume_match", Weight: 8, Group: "身份与知识", Description: "比较不同档位模型的 thinking 体量差异", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "identity_structured_match", Weight: 7, Group: "身份与知识", Description: "解析 vendor / brand / model 三行身份自报", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "cutoff_match", Weight: 7, Group: "身份与知识", Description: "比较知识截止月份与基线的接近程度", Implemented: true, Active: true, Phase: "phase1"},
		{Key: "identity_free_clean", Weight: 7, Group: "身份与知识", Description: "检测自由身份回答中是否暴露 wrapper 身份", Implemented: true, Active: true, Phase: "phase1"},
		{Key: "knowledge_recall_match", Weight: 12, Group: "身份与知识", Description: "固定事实题对照官方基线", Implemented: true, Active: true, Phase: "phase1"},
		{Key: "world_knowledge_tier_match", Weight: 12, Group: "身份与知识", Description: "比较世界知识题表现与档位是否相符", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "instruction_following_lang", Weight: 4, Group: "身份与知识", Description: "计算 CJK 占比，判断是否遵循中文回答约束", Implemented: true, Active: true, Phase: "phase1"},
		{Key: "anthropic_msg_id_format", Weight: 8, Group: "上游协议表面", Description: "检查 message_id 是否保持原生 msg_01 形态", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "service_tier_present", Weight: 6, Group: "上游协议表面", Description: "检查 usage.service_tier 等字段是否存在", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "inference_geo_present", Weight: 5, Group: "上游协议表面", Description: "检查 inference geo 字段是否存在", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "system_prompt_clean", Weight: 8, Group: "上游协议表面", Description: "比较候选与基线的 step0 输入体积差异", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "anthropic_request_id_passthrough", Weight: 4, Group: "上游协议表面", Description: "检查 request_id 链中是否保留原生 req_01", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "stop_reason_present", Weight: 3, Group: "上游协议表面", Description: "检查 stop_reason 字段是否存在且合法", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "sdk_consistency", Weight: 2, Group: "上游协议表面", Description: "检查多步请求的 sdk_name 是否稳定一致", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "buffer_dump_match", Weight: 5, Group: "流式投递", Description: "比较可见文本投递跨度与基线是否接近", Implemented: false, Active: false, Phase: "phase2"},
		{Key: "latency_baseline_match", Weight: 5, Group: "延迟", Description: "比较 HTTP TTFB 与 TTFT 相对基线的接近程度", Implemented: true, Active: true, Phase: "phase1"},
	}

	spec := MethodologySpec{
		Version:     "v3.24.1",
		WeightsHash: "about-v3.24.1-total-188",
		TotalWeight: 188,
		Dimensions:  dimensions,
	}
	for _, dimension := range dimensions {
		if dimension.Implemented {
			spec.ImplementedCount++
			spec.ImplementedWeight += dimension.Weight
		}
		if dimension.Active {
			spec.ActiveCount++
			spec.ActiveWeight += dimension.Weight
		}
	}
	return spec
}

func IsMethodologyDimensionImplemented(key string) bool {
	key = strings.TrimSpace(key)
	for _, dimension := range CurrentMethodologySpec().Dimensions {
		if dimension.Key == key {
			return dimension.Implemented
		}
	}
	return false
}
