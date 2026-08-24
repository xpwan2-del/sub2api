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

// withUpdatesEnabled 在测试期间翻转 updatesEnabled 包级开关并自动还原。
// 生产默认 false（fork 止血）；测上游在线更新/回退语义的用例需显式翻回 true。
func withUpdatesEnabled(t *testing.T, enabled bool) {
	t.Helper()
	old := updatesEnabled
	updatesEnabled = enabled
	t.Cleanup(func() { updatesEnabled = old })
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	withUpdatesEnabled(t, true)
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
		nil,
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
}

// TestUpdateServiceCheckUpdateForkDisabled 验证自研 fork 止血：updatesEnabled=false 时，
// 即便上游 GitHub 存在更高版本（v9.9.9），CheckUpdate 也必须返回 HasUpdate=false，
// 且不查询上游 GitHub（避免 fork 信息泄露，以及 PerformUpdate 覆盖自研二进制的风险）；
// 同时标记 managed_externally=true 并透传部署方注入的升级指引。
func TestUpdateServiceCheckUpdateForkDisabled(t *testing.T) {
	client := &updateServiceGitHubClientStub{
		release: &GitHubRelease{TagName: "v9.9.9"}, // 模拟上游版本远高于当前基线
	}
	upgradeGuide := &OpsGuide{
		Title:    "升级由部署仓库管理",
		Commands: []string{"./ops upgrade -i sub2api"},
	}
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		client,
		"0.1.138",        // 上游基线
		"2026.06.30-abc", // 自研 CalVer
		"release",
		&UpdateGuides{Upgrade: upgradeGuide},
	)

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.NotNil(t, info)
	require.False(t, info.HasUpdate, "fork 止血：即便上游版本更高，hasUpdate 也必须为 false")
	require.True(t, info.ManagedExternally, "禁用态必须标记 managed_externally")
	require.Equal(t, upgradeGuide, info.Guide, "禁用态应透传升级指引")
	require.False(t, client.fetchCalled, "fork 止血：不应查询上游 GitHub")
}

// TestUpdateServicePerformUpdateForkDisabled 验证禁用态 PerformUpdate 直接拒绝
// （ErrUpdatesDisabled），而非借道 CheckUpdate 伪装成「已是最新」——防止在线
// 二进制替换路径被任何调用方式触达。
func TestUpdateServicePerformUpdateForkDisabled(t *testing.T) {
	client := &updateServiceGitHubClientStub{
		release: &GitHubRelease{TagName: "v9.9.9"},
	}
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		client,
		"0.1.138",
		"2026.06.30-abc",
		"release",
		nil,
	)

	err := svc.PerformUpdate(context.Background())

	require.ErrorIs(t, err, ErrUpdatesDisabled)
	require.False(t, client.fetchCalled, "禁用态不应查询上游 GitHub")
}

// newRollbackTestService 构造 rollback 测试用 UpdateService：把给定 releases 注入为
// FetchRecentReleases 的返回，current 作为当前版本基线。guides 为可选的运维指引。
func newRollbackTestService(current string, releases []*GitHubRelease, guides *UpdateGuides) *UpdateService {
	return NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentReleases: releases},
		current,
		"2026.06.30-test",
		"release",
		guides,
	)
}

// TestUpdateServiceListRollbackVersionsFiltersAndCaps 验证候选过滤与封顶：排除
// newer/equal/prerelease/draft/重复，仅保留 strict older，降序取前 maxRollbackVersions(=3)。
func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	withUpdatesEnabled(t, true)
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
	svc := newRollbackTestService("0.1.147", releases, nil)
	result, err := svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.False(t, result.ManagedExternally)
	require.Nil(t, result.Guide)
	require.Len(t, result.Versions, 3)
	require.Equal(t, "0.1.146", result.Versions[0].Version)
	require.Equal(t, "0.1.144", result.Versions[1].Version)
	require.Equal(t, "0.1.143", result.Versions[2].Version)
}

// TestUpdateServiceListRollbackVersionsSortsUnorderedInput 验证无序输入按版本降序排列。
func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	withUpdatesEnabled(t, true)
	releases := []*GitHubRelease{
		{TagName: "v0.1.144"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
	}
	svc := newRollbackTestService("0.1.147", releases, nil)
	result, err := svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.Len(t, result.Versions, 3)
	require.Equal(t, "0.1.146", result.Versions[0].Version)
	require.Equal(t, "0.1.145", result.Versions[1].Version)
	require.Equal(t, "0.1.144", result.Versions[2].Version)
}

// TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder 验证无更旧版本时返回空切片。
func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	withUpdatesEnabled(t, true)
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"}, // current: excluded
		{TagName: "v0.1.148"}, // newer: excluded
	}
	svc := newRollbackTestService("0.1.147", releases, nil)
	result, err := svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.Empty(t, result.Versions)
}

// TestUpdateServiceListRollbackVersionsPropagatesFetchError 验证 FetchRecentReleases
// 的错误向上冒泡（不吞错）。
func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	withUpdatesEnabled(t, true)
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"0.1.147",
		"2026.06.30-test",
		"release",
		nil,
	)
	_, err := svc.ListRollbackVersions(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}

// TestUpdateServiceRollbackToVersionRejectsDisallowedTargets 验证不在允许列表
// （ListRollbackVersions 返回的 strict-older 前 N 个）的目标一律拒绝。
func TestUpdateServiceRollbackToVersionRejectsDisallowedTargets(t *testing.T) {
	withUpdatesEnabled(t, true)
	releases := []*GitHubRelease{
		{TagName: "v0.1.148"},
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
		{TagName: "v0.1.144"},
		{TagName: "v0.1.143"},
		{TagName: "v0.1.142"},
	}
	svc := newRollbackTestService("0.1.147", releases, nil)
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
	withUpdatesEnabled(t, true)
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"}, // 候选（strict older），但无 Assets
	}
	svc := newRollbackTestService("0.1.147", releases, nil)
	err := svc.RollbackToVersion(context.Background(), "v0.1.146")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRollbackVersionNotAllowed)
	require.Contains(t, err.Error(), "no compatible release found")
}

// TestUpdateServiceRollbackForkDisabled 验证自研 fork 止血对回退路径的覆盖：
// updatesEnabled=false（生产默认）时——
//   - ListRollbackVersions 不查询上游 GitHub，返回空列表 + managed_externally=true
//   - 透传部署方注入的指引（供前端渲染部署工具操作指引）；
//   - RollbackToVersion / Rollback 一律拒绝（ErrUpdatesDisabled），防止从上游下载
//     官方二进制覆盖自研二进制。
func TestUpdateServiceRollbackForkDisabled(t *testing.T) {
	withUpdatesEnabled(t, false)
	rollbackGuide := &OpsGuide{
		Title:    "版本回退由部署仓库管理",
		Note:     "降级只回退镜像, 不回滚数据库",
		Commands: []string{"./ops rollback sub2api", "./ops rollback sub2api --confirm"},
	}
	client := &updateServiceGitHubClientStub{
		recentReleases: []*GitHubRelease{{TagName: "v0.1.146"}},
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.1.147", "2026.06.30-test", "release",
		&UpdateGuides{Rollback: rollbackGuide})

	result, err := svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.True(t, result.ManagedExternally, "禁用态必须标记 managed_externally")
	require.Empty(t, result.Versions, "禁用态不得返回在线回退列表")
	require.Equal(t, rollbackGuide, result.Guide, "禁用态应透传部署指引")

	err = svc.RollbackToVersion(context.Background(), "0.1.146")
	require.ErrorIs(t, err, ErrUpdatesDisabled)

	err = svc.Rollback()
	require.ErrorIs(t, err, ErrUpdatesDisabled)

	require.False(t, client.fetchCalled, "禁用态不应查询上游 GitHub")
}

// TestOpsGuideFromEnv 验证 {ROLLBACK,UPGRADE}_GUIDE_* 环境变量的解析：多行命令
// 按行拆分并 trim、去空行；全部未配置时返回 nil；两个 prefix 互不串扰。
func TestOpsGuideFromEnv(t *testing.T) {
	t.Run("all unset returns nil", func(t *testing.T) {
		require.Nil(t, opsGuideFromEnv("ROLLBACK"))
		require.Nil(t, opsGuideFromEnv("UPGRADE"))
	})

	t.Run("parses rollback guide", func(t *testing.T) {
		t.Setenv("ROLLBACK_GUIDE_TITLE", "  版本回退由部署仓库管理  ")
		t.Setenv("ROLLBACK_GUIDE_NOTE", "降级只回退镜像, 不回滚数据库")
		t.Setenv("ROLLBACK_GUIDE_COMMANDS", "./ops rollback sub2api\n\n  ./ops rollback sub2api --confirm  \n")

		guide := opsGuideFromEnv("ROLLBACK")
		require.NotNil(t, guide)
		require.Equal(t, "版本回退由部署仓库管理", guide.Title, "title 应被 trim")
		require.Equal(t, "降级只回退镜像, 不回滚数据库", guide.Note)
		require.Equal(t, []string{"./ops rollback sub2api", "./ops rollback sub2api --confirm"}, guide.Commands,
			"命令按行拆分并 trim，空行剔除")
	})

	t.Run("parses upgrade guide independently", func(t *testing.T) {
		t.Setenv("UPGRADE_GUIDE_TITLE", "升级由部署仓库管理")
		t.Setenv("UPGRADE_GUIDE_COMMANDS", "cd <deployment repo>\n./ops upgrade -i sub2api")

		guide := opsGuideFromEnv("UPGRADE")
		require.NotNil(t, guide)
		require.Equal(t, "升级由部署仓库管理", guide.Title)
		require.Empty(t, guide.Note)
		require.Equal(t, []string{"cd <deployment repo>", "./ops upgrade -i sub2api"}, guide.Commands)

		require.Nil(t, opsGuideFromEnv("ROLLBACK"), "ROLLBACK 未配置应为 nil，不受 UPGRADE 影响")
	})

	t.Run("commands only still returns guide", func(t *testing.T) {
		t.Setenv("ROLLBACK_GUIDE_COMMANDS", "./ops rollback sub2api")

		guide := opsGuideFromEnv("ROLLBACK")
		require.NotNil(t, guide)
		require.Empty(t, guide.Title)
		require.Equal(t, []string{"./ops rollback sub2api"}, guide.Commands)
	})
}
