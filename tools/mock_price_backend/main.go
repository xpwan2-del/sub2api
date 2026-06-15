// Command mock_price_backend 是一个假上游定价服务，用于测试 sub2api 的
// 「上游价格同步」子系统（internal/service/upstream_price_sync_service.go）。
//
// 它模拟一个 one-api / new-api 系中转站的 /api/pricing 接口，支持：
//   - 三种 parser 格式：one_api（默认）/ litellm / custom —— 对同一组模型产出
//     完全相同的 per-token 价（统一从 model_ratio 推导），便于对比三种解析器。
//   - 动态改价触发 diff：预设场景（涨价/降价/新增/下架/大变动/微变动）+ 手动 upsert/删除。
//   - 模拟故障：强制返回指定状态码、人为延迟（测同步超时 30s / TestConnection 10s）。
//   - 可选 Bearer 校验：测 api_key 加解密链路（-token）。
//   - 内嵌 HTML 控制台 + 请求日志：浏览器点点就能测。
//
// 响应字节稳定性（关键）：SyncSource 用 sha256(raw) 判变，因此同一逻辑状态
// 每次序列化的字节必须完全相同。这里一律用 struct + 有序 slice（字段顺序固定、
// 元素顺序固定）保证 json.Marshal 输出稳定；litellm 格式天然是 map，靠 Go
// 对 string key 的排序输出保证稳定。
//
// 只依赖标准库，可独立运行：
//
//	cd tools/mock_price_backend && go run .
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// baseRatePerMillion 是 one-api 系 model_ratio 的计价基准（$2 / 1M token），
// 与 OneAPIParser 的换算完全一致：per_token_input = model_ratio * 2 / 1e6。
const baseRatePerMillion = 2.0

// model 是一个上游模型的定价定义。
//
// 注意：JSON tag 顺序固定（model_name → model_ratio → completion_ratio），
// 保证 one_api 格式的响应字节稳定，从而 sha256 判变正确。
type model struct {
	Name            string  `json:"model_name"`
	ModelRatio      float64 `json:"model_ratio"`
	CompletionRatio float64 `json:"completion_ratio"`
}

// perTokenIn 返回该模型的 per-token 输入价（USD），与 OneAPIParser 一致。
func (m model) perTokenIn() float64 { return m.ModelRatio * baseRatePerMillion / 1e6 }

// completion 返回 completion_ratio，空值回退为 1（与 OneAPIParser 一致）。
func (m model) completion() float64 {
	if m.CompletionRatio == 0 {
		return 1
	}
	return m.CompletionRatio
}

// perTokenOut 返回 per-token 输出价 = 输入价 × completion_ratio。
func (m model) perTokenOut() float64 { return m.perTokenIn() * m.completion() }

// behaviour 控制定价端点的故障行为（测超时/错误码）。
//
// 字段必须 exported（大写开头）且带 JSON tag：控制端点 body 用
// fail_status / delay_ms（蛇形）。小写字段是 unexported，json 会完全
// 忽略（既不 marshal 也不 unmarshal），导致故障永远不生效。
type behaviour struct {
	FailStatus int `json:"fail_status"` // 0=正常返回；非 0 则定价端点返回此状态码
	DelayMS    int `json:"delay_ms"`    // 定价端点人为延迟（毫秒），用于测同步超时
}

// reqLog 是一次定价请求的简要记录，供控制台展示「SyncService 确实来过」。
type reqLog struct {
	At    time.Time `json:"at"`
	Path  string    `json:"path"`
	Token string    `json:"token"` // Authorization 头的尾段（脱敏展示）
	Note  string    `json:"note"`  // ok / auth-fail / forced-xxx
}

// server 持有全部可变状态。所有字段经 mu 读写。
type server struct {
	mu     sync.RWMutex
	models []model
	token  string // 为空则不校验 Bearer
	behav  behaviour
	logs   []reqLog // 环形缓冲，最近 N 条
	counts map[string]int
}

const maxLogs = 50

func newServer(token string, seed int) *server {
	s := &server{token: token, counts: map[string]int{}}
	s.resetLocked(seed)
	return s
}

// 种子模型名表。顺序固定，保证「reset」场景的字节稳定可复现。
var seedNames = []string{
	"gpt-4o", "gpt-4o-mini", "claude-3-5-sonnet",
	"claude-3-5-haiku", "gemini-1.5-pro", "deepseek-chat", "glm-4",
}

// resetLocked 重建种子模型集。调用方须持写锁。
func (s *server) resetLocked(n int) {
	s.models = s.models[:0]
	for i := 0; i < n && i < len(seedNames); i++ {
		s.models = append(s.models, model{
			Name:            seedNames[i],
			ModelRatio:      1.5 + float64(i)*0.5, // 1.5 / 2.0 / 2.5 ...
			CompletionRatio: 4.0,
		})
	}
}

// Reset 是 resetLocked 的加锁版本。
func (s *server) Reset(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetLocked(n)
}

// applyScenario 切换一个预设场景，返回人类可读说明。
func (s *server) applyScenario(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch name {
	case "reset":
		s.resetLocked(5)
		return "已重置为 5 个种子模型（基线）", nil
	case "hike": // 全量 +20% → 全是 price_up
		for i := range s.models {
			s.models[i].ModelRatio *= 1.2
		}
		return "全部模型 +20%（price_up）", nil
	case "cut": // 全量 -20% → 全是 price_down
		for i := range s.models {
			s.models[i].ModelRatio *= 0.8
		}
		return "全部模型 -20%（price_down）", nil
	case "add": // 追加 2 个模型 → new_model
		s.models = append(s.models,
			model{Name: "qwen-max", ModelRatio: 4.2, CompletionRatio: 2},
			model{Name: "o1-preview", ModelRatio: 15, CompletionRatio: 4},
		)
		return "新增 qwen-max、o1-preview（new_model）", nil
	case "remove": // 下架末位模型 → removed
		if len(s.models) > 0 {
			s.models = s.models[:len(s.models)-1]
		}
		return "下架末位模型（removed）", nil
	case "big": // 首模型 +50% → 触发 critical（>20%）
		if len(s.models) > 0 {
			s.models[0].ModelRatio *= 1.5
		}
		return "首模型 +50%（critical 告警 >20%）", nil
	case "tiny": // 首模型 +1% → 小幅变动（低于常见阈值，测过滤）
		if len(s.models) > 0 {
			s.models[0].ModelRatio *= 1.01
		}
		return "首模型 +1%（小幅变动，测 AlertThresholdPct 过滤）", nil
	default:
		return "", fmt.Errorf("未知场景 %q（可选: reset/hike/cut/add/remove/big/tiny）", name)
	}
}

// upsert 按名新增或更新模型。存在则改值（保持 slice 顺序稳定 → hash 只因数值变化而变）。
func (s *server) upsert(m model) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.models {
		if s.models[i].Name == m.Name {
			s.models[i] = m
			return
		}
	}
	s.models = append(s.models, m)
}

// remove 删除指定模型。
func (s *server) remove(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.models {
		if s.models[i].Name == name {
			s.models = append(s.models[:i], s.models[i+1:]...)
			return true
		}
	}
	return false
}

// setBehaviour 设置故障行为（fail_status / delay_ms）。
func (s *server) setBehaviour(b behaviour) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.behav = b
}

// snapshot 返回当前状态的只读快照（给控制台/state 接口用）。
func (s *server) snapshot() stateSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	models := make([]model, len(s.models))
	copy(models, s.models)
	view := make([]modelView, len(models))
	for i, m := range models {
		view[i] = modelView{
			Name:        m.Name,
			ModelRatio:  m.ModelRatio,
			Completion:  m.completion(),
			InPerToken:  m.perTokenIn(),
			OutPerToken: m.perTokenOut(),
		}
	}
	logs := make([]reqLog, len(s.logs))
	copy(logs, s.logs)
	counts := map[string]int{}
	for k, v := range s.counts {
		counts[k] = v
	}
	return stateSnapshot{
		Models:   view,
		Logs:     logs,
		Counts:   counts,
		Behav:    s.behav,
		NeedAuth: s.token != "",
	}
}

type modelView struct {
	Name        string  `json:"name"`
	ModelRatio  float64 `json:"model_ratio"`
	Completion  float64 `json:"completion_ratio"`
	InPerToken  float64 `json:"in_per_token"`
	OutPerToken float64 `json:"out_per_token"`
}

type stateSnapshot struct {
	Models   []modelView    `json:"models"`
	Logs     []reqLog       `json:"logs"`
	Counts   map[string]int `json:"counts"`
	Behav    behaviour      `json:"behaviour"`
	NeedAuth bool           `json:"need_auth"`
}

// note 记录一条请求日志 + 计数。调用方不持锁（内部加写锁）。
func (s *server) note(r *http.Request, note string) {
	token := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		tail := strings.TrimPrefix(h, "Bearer ")
		// 脱敏：只保留首尾各 2 字符，中间打码。
		if len(tail) > 6 {
			token = tail[:2] + "***" + tail[len(tail)-2:]
		} else {
			token = "***"
		}
	}
	s.mu.Lock()
	s.logs = append(s.logs, reqLog{At: time.Now(), Path: r.URL.Path, Token: token, Note: note})
	if len(s.logs) > maxLogs {
		s.logs = s.logs[len(s.logs)-maxLogs:]
	}
	s.counts[r.URL.Path]++
	s.mu.Unlock()
}

// authOK 校验 Bearer token（token 为空时放行）。
func (s *server) authOK(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	return r.Header.Get("Authorization") == "Bearer "+s.token
}

// pricingJSON 按 format 序列化当前模型集。调用方不持锁（内部加读锁）。
func (s *server) pricingJSON(format string) ([]byte, error) {
	s.mu.RLock()
	models := make([]model, len(s.models))
	copy(models, s.models)
	b := s.behav
	s.mu.RUnlock()

	// 故障注入（在生成响应前生效）。
	if b.DelayMS > 0 {
		time.Sleep(time.Duration(b.DelayMS) * time.Millisecond)
	}

	switch format {
	case "one_api", "":
		// one_api 格式：{"data":[{"model_name","model_ratio","completion_ratio"}]}
		// struct + slice → 字节稳定。
		resp := struct {
			Data []model `json:"data"`
		}{Data: models}
		return json.Marshal(resp)

	case "litellm":
		// litellm 格式：{"<model>":{"input_cost_per_token",...}}
		// per-token 价与 one_api 推导结果一致，便于对比两种 parser。
		m := make(map[string]map[string]any, len(models))
		for _, mm := range models {
			m[mm.Name] = map[string]any{
				"input_cost_per_token":            mm.perTokenIn(),
				"output_cost_per_token":           mm.perTokenOut(),
				"cache_creation_input_token_cost": mm.perTokenIn() * 0.5,
				"cache_read_input_token_cost":     mm.perTokenIn() * 0.1,
			}
		}
		return json.Marshal(m)

	case "custom":
		// custom 格式：{"data":[{"model","in","out"}]}
		// 字段名严格对齐 CustomJSONPathParser（读 item.Get("in")/Get("out")）。
		type row struct {
			Model string  `json:"model"`
			In    float64 `json:"in"`
			Out   float64 `json:"out"`
		}
		rows := make([]row, 0, len(models))
		for _, mm := range models {
			rows = append(rows, row{Model: mm.Name, In: mm.perTokenIn(), Out: mm.perTokenOut()})
		}
		resp := struct {
			Data []row `json:"data"`
		}{Data: rows}
		return json.Marshal(resp)
	}
	return nil, fmt.Errorf("未知格式 %q", format)
}

// ===== HTTP handlers =====

// pricingHandler 返回指定格式的定价响应。
func (s *server) pricingHandler(format string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authOK(r) {
			s.note(r, "auth-fail")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s.mu.RLock()
		fail := s.behav.FailStatus
		s.mu.RUnlock()
		if fail != 0 {
			s.note(r, fmt.Sprintf("forced-%d", fail))
			http.Error(w, fmt.Sprintf("forced status %d", fail), fail)
			return
		}
		s.note(r, "ok")
		body, err := s.pricingJSON(format)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// stateHandler 返回当前状态快照（控制台 + 调试用）。
func (s *server) stateHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.snapshot())
}

// scenarioHandler 切换预设场景。
func (s *server) scenarioHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/admin/scenario/")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少场景名"})
		return
	}
	msg, err := s.applyScenario(name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"scenario": name, "message": msg})
}

// modelsHandler 处理模型的 upsert。
func (s *server) modelsHandler(w http.ResponseWriter, r *http.Request) {
	var m model
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效 JSON: " + err.Error()})
		return
	}
	if m.Name == "" || m.ModelRatio <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model_name 和 model_ratio(>0) 必填"})
		return
	}
	if m.CompletionRatio == 0 {
		m.CompletionRatio = 1
	}
	s.upsert(m)
	writeJSON(w, http.StatusOK, map[string]string{"message": "upserted", "model": m.Name})
}

// modelsDeleteHandler 下架模型。
func (s *server) modelsDeleteHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/admin/models/")
	if !s.remove(name) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "模型不存在"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "removed", "model": name})
}

// behaviourHandler 设置故障行为。
//   - {"fail_status": 500}        定价端点恒返回 500
//   - {"delay_ms": 30000}         定价端点延迟 30s（测同步超时）
//   - {}                          清除故障
func (s *server) behaviourHandler(w http.ResponseWriter, r *http.Request) {
	var b behaviour
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效 JSON"})
		return
	}
	s.setBehaviour(b)
	writeJSON(w, http.StatusOK, map[string]any{"message": "behaviour updated", "behaviour": b})
}

// consoleHandler 返回内嵌的 HTML 控制台。
func consoleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(consoleHTML))
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9999", "监听地址")
	token := flag.String("token", "", "Bearer 校验 token（空=不校验；设了则 /api/pricing 需带 Authorization: Bearer <token>）")
	seed := flag.Int("seed", 5, "初始种子模型数量")
	flag.Parse()

	s := newServer(*token, *seed)

	mux := http.NewServeMux()

	// 被同步端点（受 token 校验）—— 与 sub2api 的三种 parser 对应。
	mux.HandleFunc("/api/pricing", s.pricingHandler("one_api"))
	mux.HandleFunc("/api/pricing/litellm", s.pricingHandler("litellm"))
	mux.HandleFunc("/api/pricing/custom", s.pricingHandler("custom"))

	// 健康检查（无鉴权）。
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// 控制台 + 控制端点（无鉴权，本地用）。
	mux.HandleFunc("/", consoleHandler)
	mux.HandleFunc("/admin/state", s.stateHandler)
	mux.HandleFunc("/admin/scenario/", s.scenarioHandler) // /admin/scenario/:name
	mux.HandleFunc("/admin/models", s.modelsHandler)      // POST upsert
	mux.HandleFunc("/admin/models/", s.modelsDeleteHandler)
	mux.HandleFunc("/admin/behaviour", s.behaviourHandler)

	log.Printf("mock_price_backend listening on http://%s", *addr)
	if *token != "" {
		log.Printf("  Bearer 校验已启用，token=%q", *token)
	}
	log.Printf("  定价端点: GET /api/pricing | /api/pricing/litellm | /api/pricing/custom")
	log.Printf("  控制台:   http://%s/", *addr)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
