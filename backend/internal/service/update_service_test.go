//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release        *GitHubRelease
	fetchCalled    bool
	recentReleases []*GitHubRelease
	recentErr      error
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(context.Context, string) (*GitHubRelease, error) {
	s.fetchCalled = true
	return s.release, nil
}

func (s *updateServiceGitHubClientStub) FetchRecentReleases(context.Context, string, int) ([]*GitHubRelease, error) {
	return s.recentReleases, s.recentErr
}

func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	panic("DownloadFile should not be called when no update is available")
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	panic("FetchChecksumFile should not be called when no update is available")
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v0.1.132",
				Name:    "v0.1.132",
			},
		},
		"0.1.132",
		"2026.06.24-test",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
}

// TestUpdateServiceCheckUpdateForkDisabled 验证自研 fork 止血：updatesEnabled=false 时，
// 即便上游 GitHub 存在更高版本（v9.9.9），CheckUpdate 也必须返回 HasUpdate=false，
// 且不查询上游 GitHub（避免 fork 信息泄露，以及 PerformUpdate 覆盖自研二进制的风险）。
func TestUpdateServiceCheckUpdateForkDisabled(t *testing.T) {
	client := &updateServiceGitHubClientStub{
		release: &GitHubRelease{TagName: "v9.9.9"}, // 模拟上游版本远高于当前基线
	}
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		client,
		"0.1.138",        // 上游基线
		"2026.06.30-abc", // 自研 CalVer
		"release",
	)

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.NotNil(t, info)
	require.False(t, info.HasUpdate, "fork 止血：即便上游版本更高，hasUpdate 也必须为 false")
	require.False(t, client.fetchCalled, "fork 止血：不应查询上游 GitHub")
}

// newRollbackTestService 构造 rollback 测试用 UpdateService：把给定 releases 注入为
// FetchRecentReleases 的返回，current 作为当前版本基线。
func newRollbackTestService(current string, releases []*GitHubRelease) *UpdateService {
	return NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentReleases: releases},
		current,
		"2026.06.30-test",
		"release",
	)
}

// TestUpdateServiceListRollbackVersionsFiltersAndCaps 验证候选过滤与封顶：排除
// newer/equal/prerelease/draft/重复，仅保留 strict older，降序取前 maxRollbackVersions(=3)。
func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148", PublishedAt: "2026-07-09T00:00:00Z"},                       // newer than current: excluded
		{TagName: "v0.1.147", PublishedAt: "2026-07-08T00:00:00Z"},                       // current: excluded
		{TagName: "v0.1.146-rc1", PublishedAt: "2026-07-07T12:00:00Z", Prerelease: true}, // prerelease: excluded
		{TagName: "v0.1.146", PublishedAt: "2026-07-07T00:00:00Z"},
		{TagName: "v0.1.145", PublishedAt: "2026-07-06T00:00:00Z", Draft: true}, // draft: excluded
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"}, // duplicate: excluded
		{TagName: "v0.1.143", PublishedAt: "2026-07-04T00:00:00Z"},
		{TagName: "v0.1.142", PublishedAt: "2026-07-03T00:00:00Z"}, // beyond cap of 3: excluded
	}
	svc := newRollbackTestService("0.1.147", releases)
	versions, err := svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.144", versions[1].Version)
	require.Equal(t, "0.1.143", versions[2].Version)
}

// TestUpdateServiceListRollbackVersionsSortsUnorderedInput 验证无序输入按版本降序排列。
func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.144"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
	}
	svc := newRollbackTestService("0.1.147", releases)
	versions, err := svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.145", versions[1].Version)
	require.Equal(t, "0.1.144", versions[2].Version)
}

// TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder 验证无更旧版本时返回空切片。
func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"}, // current: excluded
		{TagName: "v0.1.148"}, // newer: excluded
	}
	svc := newRollbackTestService("0.1.147", releases)
	versions, err := svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.Empty(t, versions)
}

// TestUpdateServiceListRollbackVersionsPropagatesFetchError 验证 FetchRecentReleases
// 的错误向上冒泡（不吞错）。
func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"0.1.147",
		"2026.06.30-test",
		"release",
	)
	_, err := svc.ListRollbackVersions(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}

// TestUpdateServiceRollbackToVersionRejectsDisallowedTargets 验证不在允许列表
// （ListRollbackVersions 返回的 strict-older 前 N 个）的目标一律拒绝。
func TestUpdateServiceRollbackToVersionRejectsDisallowedTargets(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148"},
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
		{TagName: "v0.1.144"},
		{TagName: "v0.1.143"},
		{TagName: "v0.1.142"},
	}
	svc := newRollbackTestService("0.1.147", releases)
	for _, target := range []string{
		"",         // empty
		"0.1.147",  // current version
		"v0.1.147", // current version with prefix
		"0.1.148",  // newer than current
		"0.1.142",  // older than the 3 most recent (beyond cap)
		"9.9.9",    // nonexistent
	} {
		err := svc.RollbackToVersion(context.Background(), target)
		require.ErrorIs(t, err, ErrRollbackVersionNotAllowed, "target %q should be rejected", target)
	}
}

// TestUpdateServiceRollbackToVersionAcceptsVPrefix 验证带 v 前缀的目标版本通过允许列表
// 校验：目标在候选中匹配，但因 release 无平台 asset 在后续 asset 查找阶段失败
// （"no compatible release found"），证明版本本身被接受、错误非 ErrRollbackVersionNotAllowed。
func TestUpdateServiceRollbackToVersionAcceptsVPrefix(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"}, // 候选（strict older），但无 Assets
	}
	svc := newRollbackTestService("0.1.147", releases)
	err := svc.RollbackToVersion(context.Background(), "v0.1.146")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRollbackVersionNotAllowed)
	require.Contains(t, err.Error(), "no compatible release found")
}
