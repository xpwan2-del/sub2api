package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeVideoBillingResolutionOrDefault(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// 原有档位不回归
		{"480p", "480p", VideoBillingResolution480P},
		{"480", "480", VideoBillingResolution480P},
		{"sd", "SD", VideoBillingResolution480P},
		{"720p", "720p", VideoBillingResolution720P},
		{"720", "720", VideoBillingResolution720P},
		{"hd", "HD", VideoBillingResolution720P},
		{"1080p", "1080p", VideoBillingResolution1080P},
		{"1080", "1080", VideoBillingResolution1080P},
		{"full_hd", "full_hd", VideoBillingResolution1080P},
		{"full-hd", "full-hd", VideoBillingResolution1080P},
		{"fhd", "FHD", VideoBillingResolution1080P},
		// 4K 档（前端已开放、后端本轮补齐）
		{"4k lower", "4k", VideoBillingResolution4K},
		{"4K upper", "4K", VideoBillingResolution4K},
		{"2160", "2160", VideoBillingResolution4K},
		{"2160p", "2160p", VideoBillingResolution4K},
		{"uhd", "UHD", VideoBillingResolution4K},
		// WxH 宽高格式（横屏）
		{"1920x1080", "1920x1080", VideoBillingResolution1080P},
		{"3840x2160", "3840x2160", VideoBillingResolution4K},
		{"1280x720", "1280x720", VideoBillingResolution720P},
		{"640x480", "640x480", VideoBillingResolution480P},
		// WxH 竖屏（短边判档）
		{"1080x1920 portrait", "1080x1920", VideoBillingResolution1080P},
		{"2160x3840 portrait", "2160x3840", VideoBillingResolution4K},
		// 兜底
		{"empty", "", VideoBillingResolution480P},
		{"unknown", "unknown", VideoBillingResolution480P},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, NormalizeVideoBillingResolutionOrDefault(tc.in))
		})
	}
}

func TestNormalizeVideoBillingDurationSeconds(t *testing.T) {
	cases := []struct {
		name string
		dur  int
		max  int
		want int
	}{
		{"zero → default", 0, VideoBillingMaxDurationSeconds, VideoBillingDefaultDurationSeconds},
		{"negative → default", -3, VideoBillingMaxDurationSeconds, VideoBillingDefaultDurationSeconds},
		{"in range", 5, VideoBillingMaxDurationSeconds, 5},
		{"grok over 15 capped", 20, VideoBillingMaxDurationSeconds, VideoBillingMaxDurationSeconds},
		{"grok boundary 15", 15, VideoBillingMaxDurationSeconds, 15},
		{"generic 60s not capped", 60, VideoBillingGenericMaxDurationSeconds, 60},
		{"generic boundary 600", 600, VideoBillingGenericMaxDurationSeconds, VideoBillingGenericMaxDurationSeconds},
		{"generic over 600 sanity-capped", 900, VideoBillingGenericMaxDurationSeconds, VideoBillingGenericMaxDurationSeconds},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, normalizeVideoBillingDurationSeconds(tc.dur, tc.max))
		})
	}
}

func TestVideoBillingDurationCapForModel(t *testing.T) {
	require.Equal(t, VideoBillingMaxDurationSeconds, videoBillingDurationCapForModel("grok-imagine-video"))
	require.Equal(t, VideoBillingMaxDurationSeconds, videoBillingDurationCapForModel("grok-imagine-video-1.5"))
	require.Equal(t, VideoBillingGenericMaxDurationSeconds, videoBillingDurationCapForModel("kling-video"))
	require.Equal(t, VideoBillingGenericMaxDurationSeconds, videoBillingDurationCapForModel("sora-2"))
	require.Equal(t, VideoBillingGenericMaxDurationSeconds, videoBillingDurationCapForModel(""))
}
