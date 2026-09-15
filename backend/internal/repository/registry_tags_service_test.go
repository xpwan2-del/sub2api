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
	require.Equal(t, "2026.08.24-84152dc8", latest, "artifacts API 404（stub 未注册）应降级到 tags/list 字典序，过滤 latest/semver tag")
}

// harborArtifactsStub 在 harborStub 基础上补 /api/v2.0 artifacts 端点，push_time
// 顺序与 tag 字典序相反：push 晚的 2026.08.24-1aaaaaaa 字典序小于 push 早的
// 2026.08.24-9bbbbbbb，复刻 2026-08-24 同日双版本（90587ecd 字典序压过更晚
// 构建的 2a9222fd）的线上事故形态。
func harborArtifactsStub(t *testing.T) *httptest.Server {
	t.Helper()
	const token = "stub-registry-token"
	authorized := func(r *http.Request) bool {
		auth := r.Header.Get("Authorization")
		return auth == "Bearer "+token || auth == "Basic dXNlcjpwYXNz"
	}
	challenge := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="`+realmBase(r)+`/service/token",service="harbor-registry"`)
		w.WriteHeader(http.StatusUnauthorized)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/service/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"token":"` + token + `"}`))
	})
	mux.HandleFunc("/api/v2.0/projects/topai/repositories/sub2api/artifacts", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r) {
			challenge(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"digest":"sha256:old","push_time":"2026-08-24T02:00:00.123Z","tags":[{"name":"2026.08.24-9bbbbbbb"}]},
			{"digest":"sha256:new","push_time":"2026-08-24T08:30:00.123Z","tags":[{"name":"latest"},{"name":"2026.08.24-1aaaaaaa"}]}
		]`))
	})
	mux.HandleFunc("/v2/topai/sub2api/tags/list", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r) {
			challenge(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"topai/sub2api","tags":["latest","2026.08.24-9bbbbbbb","2026.08.24-1aaaaaaa"]}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestRegistryTagsClientLatestBuildTagPrefersPushTime(t *testing.T) {
	server := harborArtifactsStub(t)
	client := NewRegistryTagsClient(server.URL, "topai/sub2api", "")
	require.NotNil(t, client)

	latest, err := client.LatestBuildTag(context.Background())

	require.NoError(t, err)
	require.Equal(t, "2026.08.24-1aaaaaaa", latest, "同日多版本应按 push_time 取最新，而非 short SHA 字典序")
}

func TestRegistryTagsClientLatestBuildTagPrefersPushTimeBasicAuth(t *testing.T) {
	server := harborArtifactsStub(t)
	client := NewRegistryTagsClient(server.URL, "topai/sub2api", "user:pass")
	require.NotNil(t, client)

	latest, err := client.LatestBuildTag(context.Background())

	require.NoError(t, err)
	require.Equal(t, "2026.08.24-1aaaaaaa", latest, "Basic 凭据应同样命中 artifacts 端点并按 push_time 取最新")
}

// push_time 缺失的 artifact 不可比，应跳过并继续比较其余候选（tags/list 里的
// 9ccccccc 字典序更大，但绝不能胜出）。
func TestRegistryTagsClientSkipsArtifactsWithoutPushTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"digest":"sha256:notime","tags":[{"name":"2026.08.24-9ccccccc"}]},
			{"digest":"sha256:new","push_time":"2026-08-24T08:00:00Z","tags":[{"name":"2026.08.24-1aaaaaaa"}]}
		]`))
	}))
	t.Cleanup(server.Close)

	client := NewRegistryTagsClient(server.URL, "topai/sub2api", "")
	latest, err := client.LatestBuildTag(context.Background())

	require.NoError(t, err)
	require.Equal(t, "2026.08.24-1aaaaaaa", latest, "缺 push_time 的 artifact 应被跳过")
}

func TestSplitImage(t *testing.T) {
	project, repo, err := splitImage("topai/sub2api")
	require.NoError(t, err)
	require.Equal(t, "topai", project)
	require.Equal(t, "sub2api", repo)

	project, repo, err = splitImage("topai/team/sub2api")
	require.NoError(t, err)
	require.Equal(t, "topai", project)
	require.Equal(t, "team/sub2api", repo)

	_, _, err = splitImage("sub2api")
	require.Error(t, err, "无项目前缀的镜像名无法构造 Harbor API 地址")
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
