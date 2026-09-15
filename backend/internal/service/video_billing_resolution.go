package service

import (
	"strconv"
	"strings"
)

const (
	VideoBillingResolution480P  = "480p"
	VideoBillingResolution720P  = "720p"
	VideoBillingResolution1080P = "1080p"
	VideoBillingResolution4K    = "4k"
)

// xAI/grok-imagine-video 视频生成按秒计费，duration 请求参数允许 1-15 秒；未指定时上游默认生成 8 秒。
// 计费时长必须与上游实际消耗对齐，否则用户可通过拉长 duration 套利（提交时长由用户控制）。
// 通用视频模型（非 xAI）不受 15 秒上游规格约束，使用宽松 sanity 上限防止解析异常或恶意天价计费。
const (
	VideoBillingMinDurationSeconds        = 1
	VideoBillingMaxDurationSeconds        = 15 // xAI/grok-imagine-video 上游允许的时长上限
	VideoBillingDefaultDurationSeconds    = 8
	VideoBillingGenericMaxDurationSeconds = 600 // 通用视频模型计费时长的 sanity 上限
)

// normalizeVideoBillingDurationSeconds 归一化计费用视频时长：
// 未指定（<=0）按默认 8 秒；低于下限按 1 秒；超过 maxDuration 按上限收敛。
// xAI 系列传 VideoBillingMaxDurationSeconds，通用模型传 VideoBillingGenericMaxDurationSeconds。
func normalizeVideoBillingDurationSeconds(durationSeconds, maxDuration int) int {
	if durationSeconds <= 0 {
		return VideoBillingDefaultDurationSeconds
	}
	if durationSeconds < VideoBillingMinDurationSeconds {
		return VideoBillingMinDurationSeconds
	}
	if maxDuration > 0 && durationSeconds > maxDuration {
		return maxDuration
	}
	return durationSeconds
}

// NormalizeVideoBillingDurationSecondsOrDefault 保持 xAI 口径（1-15 秒，默认 8 秒），
// 供 grok 请求解析与历史调用方使用；通用视频模型计费改用 normalizeVideoBillingDurationSeconds。
func NormalizeVideoBillingDurationSecondsOrDefault(durationSeconds int) int {
	return normalizeVideoBillingDurationSeconds(durationSeconds, VideoBillingMaxDurationSeconds)
}

// videoBillingDurationCapForModel 返回模型对应的计费时长上限：
// xAI/grok-imagine-video 系列受上游 15 秒规格约束；其他通用视频模型用宽松 sanity 上限。
func videoBillingDurationCapForModel(model string) int {
	if isGrokVideoBillingModel(model) {
		return VideoBillingMaxDurationSeconds
	}
	return VideoBillingGenericMaxDurationSeconds
}

// LookupVideoBillingResolution 归一化分辨率并报告是否为已知档位。
// 已知档位（480p/720p/1080p/4k 及可按短边归档的 WxH 宽高）返回 (档位, true)；
// 无法识别的返回 ("", false)，配置解析路径据此报错，避免把高分辨率单价静默挂到低档。
func LookupVideoBillingResolution(resolution string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "480", "480p", "sd":
		return VideoBillingResolution480P, true
	case "720", "720p", "hd":
		return VideoBillingResolution720P, true
	case "1080", "1080p", "full_hd", "full-hd", "fhd":
		return VideoBillingResolution1080P, true
	case "4k", "2160", "2160p", "uhd":
		return VideoBillingResolution4K, true
	default:
		// 兜底前尝试 WxH 宽高格式（如 "1920x1080"/"1080x1920"），按短边归档。
		if w, h, ok := parseVideoResolutionDims(resolution); ok {
			return videoResolutionByShortSide(w, h), true
		}
		return "", false
	}
}

// parseVideoResolutionDims 解析 "WxH" 形式的分辨率字符串（兼容 x / × / * 分隔符）。
// 解析失败或维度非正时返回 ok=false。
func parseVideoResolutionDims(resolution string) (int, int, bool) {
	s := strings.TrimSpace(resolution)
	idx := strings.IndexAny(s, "x×*")
	if idx <= 0 {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(s[:idx]))
	h, err2 := strconv.Atoi(strings.TrimSpace(s[idx+1:]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// videoResolutionByShortSide 按短边像素将宽高归入计费档位（横竖屏通用）。
func videoResolutionByShortSide(w, h int) string {
	short := w
	if h < short {
		short = h
	}
	switch {
	case short >= 2160:
		return VideoBillingResolution4K
	case short >= 1080:
		return VideoBillingResolution1080P
	case short >= 720:
		return VideoBillingResolution720P
	default:
		return VideoBillingResolution480P
	}
}

// NormalizeVideoBillingResolutionOrDefault 用于运行时计费：上游回传的分辨率
// 缺失或无法识别时按最低档兜底，保证请求仍可计费。
func NormalizeVideoBillingResolutionOrDefault(resolution string) string {
	if normalized, ok := LookupVideoBillingResolution(resolution); ok {
		return normalized
	}
	return VideoBillingResolution480P
}
