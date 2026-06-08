package service

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrBundleNotFound          = infraerrors.NotFound("BUNDLE_NOT_FOUND", "bundle subscription not found")
	ErrBundlePlanNotFound      = infraerrors.NotFound("BUNDLE_PLAN_NOT_FOUND", "bundle plan not found")
	ErrBundleExpired           = infraerrors.Forbidden("BUNDLE_EXPIRED", "bundle subscription has expired")
	ErrBundleConflict          = infraerrors.Conflict("BUNDLE_CONFLICT", "bundle subscription already exists for this user and plan")
	ErrBundlePlanDisabled      = infraerrors.BadRequest("BUNDLE_PLAN_DISABLED", "bundle plan is disabled and cannot be purchased")
	ErrBundleModelNotIncluded  = infraerrors.Forbidden("BUNDLE_MODEL_NOT_INCLUDED", "requested model is not included in the bundle plan")
	ErrBundleGroupQuotaExceeded = infraerrors.Forbidden("BUNDLE_GROUP_QUOTA_EXCEEDED", "bundle group quota has been exceeded")
	ErrBundleConcurrencyExceeded = infraerrors.TooManyRequests("BUNDLE_CONCURRENCY_EXCEEDED", "bundle concurrency limit exceeded")
	ErrBundleRPMExceeded       = infraerrors.TooManyRequests("BUNDLE_RPM_EXCEEDED", "bundle RPM limit exceeded")
)
