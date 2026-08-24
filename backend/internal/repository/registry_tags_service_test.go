//go:build unit

package repository

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// harborStub 起一个最小 OCI Distribution registry 模拟：先对 tags/list 返回
// 401 + Bearer challenge，凭正确 token（或 Basic 凭据）重试成功后返回固定 tags
// （含 latest 别名与 semver tag，用于验证 CalVer 过滤）。
func harborStub(t *testing.T) *httptest.Server {
	t.Helper()
	const token = "stub-registry-token"
	mux := http.NewServeMux()
	mux.HandleFunc("/service/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"token":"` + token + `"}`))
	})
	mux.HandleFunc("/v2/topai/sub2api/tags/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token && r.Header.Get("Authorization") != "Basic dXNlcjpwYXNz" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+realmBase(r)+`/service/token",service="harbor-registry"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"topai/sub2api","tags":["latest","2026.08.23-aaaaaaaa","2026.08.24-84152dc8","0.1.146","2026.08.20-bbbbbbbb"]}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// realmBase 从请求推导 stub 的基地址（challenge 里的 realm 必须是绝对地址）。
func realmBase(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func TestRegistryTagsClientLatestBuildTagViaBearer(t *testing.T) {
	server := harborStub(t)
	client := NewRegistryTagsClient(server.URL, "topai/sub2api", "")
	require.NotNil(t, client)

	latest, err := client.LatestBuildTag(context.Background())

	require.NoError(t, err)
	require.Equal(t, "2026.08.24-84152dc8", latest, "应过滤 latest/semver tag 后取字典序最大的 CalVer")
}

func TestRegistryTagsClientBasicAuthPassThrough(t *testing.T) {
	server := harborStub(t)
	// 凭据直接命中 tags/list 的 Basic 分支（不依赖 token 端点）
	client := NewRegistryTagsClient(server.URL, "topai/sub2api", "user:pass")
	require.NotNil(t, client)

	latest, err := client.LatestBuildTag(context.Background())

	require.NoError(t, err)
	require.Equal(t, "2026.08.24-84152dc8", latest)
}

func TestRegistryTagsClientNoCalVerTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tags":["latest"]}`))
	}))
	t.Cleanup(server.Close)

	client := NewRegistryTagsClient(server.URL, "topai/sub2api", "")
	latest, err := client.LatestBuildTag(context.Background())

	require.NoError(t, err)
	require.Empty(t, latest, "仅 latest 别名时应返回空串")
}

func TestRegistryTagsClientUnauthorizedWithoutCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="harbor"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	client := NewRegistryTagsClient(server.URL, "topai/sub2api", "")
	_, err := client.LatestBuildTag(context.Background())

	require.Error(t, err, "Basic challenge 且未配置凭据时应报错而非死循环")
}

func TestRegistryTagsClientUnreachable(t *testing.T) {
	client := NewRegistryTagsClient("http://127.0.0.1:1", "topai/sub2api", "")
	_, err := client.LatestBuildTag(context.Background())
	require.Error(t, err)
}

func TestNewRegistryTagsClientRequiresConfig(t *testing.T) {
	require.Nil(t, NewRegistryTagsClient("", "topai/sub2api", ""), "缺 registry 返回 nil")
	require.Nil(t, NewRegistryTagsClient("harbor.example.com", "  ", ""), "缺 image 返回 nil")
}
