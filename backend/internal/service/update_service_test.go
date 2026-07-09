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
	release     *GitHubRelease
	fetchCalled bool
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(context.Context, string) (*GitHubRelease, error) {
	s.fetchCalled = true
	return s.release, nil
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
