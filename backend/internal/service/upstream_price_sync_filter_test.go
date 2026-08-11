package service

import "testing"

// TestModelGroupEnabled 覆盖 SyncNow 按上游分组过滤模型的核心判定。
// 注意:本包存在预存的 testConfig 重名冲突(upstream_price_sync_service_test.go
// 与 gateway_multiplatform_test.go),-tags=unit 下编译失败(非本测试引入)。
// 该冲突修复后本测试即可运行。
func TestModelGroupEnabled(t *testing.T) {
	tests := []struct {
		name         string
		enableGroups []string
		target       string
		want         bool
	}{
		{"target 为空 = 不过滤,恒保留", []string{"vip"}, "", true},
		{"enableGroups 为 nil = 全分组可用", nil, "default", true},
		{"enableGroups 为空切片 = 全分组可用", []string{}, "default", true},
		{"target 命中 enableGroups", []string{"default", "vip"}, "vip", true},
		{"target 未在 enableGroups 中", []string{"default", "vip"}, "enterprise", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modelGroupEnabled(tt.enableGroups, tt.target); got != tt.want {
				t.Errorf("modelGroupEnabled(%v, %q) = %v, want %v", tt.enableGroups, tt.target, got, tt.want)
			}
		})
	}
}
