// bundle_errors.go 套餐捆绑销售模块错误定义
// 统一定义套餐模块所有业务错误，使用 infraerrors 包确保与全局错误处理一致。

package service

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	// ErrBundleNotFound 套餐订阅不存在
	ErrBundleNotFound          = infraerrors.NotFound("BUNDLE_NOT_FOUND", "bundle subscription not found")
	// ErrBundlePlanNotFound 套餐计划不存在
	ErrBundlePlanNotFound      = infraerrors.NotFound("BUNDLE_PLAN_NOT_FOUND", "bundle plan not found")
	// ErrBundleExpired 套餐订阅已过期
	ErrBundleExpired           = infraerrors.Forbidden("BUNDLE_EXPIRED", "bundle subscription has expired")
	// ErrBundleConflict 用户已存在该套餐的活跃订阅
	ErrBundleConflict          = infraerrors.Conflict("BUNDLE_CONFLICT", "bundle subscription already exists for this user and plan")
	// ErrBundlePlanDisabled 套餐计划已停用，无法购买
	ErrBundlePlanDisabled      = infraerrors.BadRequest("BUNDLE_PLAN_DISABLED", "bundle plan is disabled and cannot be purchased")
	// ErrBundleModelNotIncluded 请求的模型不在套餐包含的范围内
	ErrBundleModelNotIncluded  = infraerrors.Forbidden("BUNDLE_MODEL_NOT_INCLUDED", "requested model is not included in the bundle plan")
	// ErrBundleGroupQuotaExceeded 套餐渠道组额度已用尽
	ErrBundleGroupQuotaExceeded = infraerrors.Forbidden("BUNDLE_GROUP_QUOTA_EXCEEDED", "bundle group quota has been exceeded")
	// ErrBundleConcurrencyExceeded 套餐并发数超限
	ErrBundleConcurrencyExceeded = infraerrors.TooManyRequests("BUNDLE_CONCURRENCY_EXCEEDED", "bundle concurrency limit exceeded")
	// ErrBundleRPMExceeded 套餐 RPM（每分钟请求数）超限
	ErrBundleRPMExceeded       = infraerrors.TooManyRequests("BUNDLE_RPM_EXCEEDED", "bundle RPM limit exceeded")
)
