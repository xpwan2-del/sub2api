package main

// consoleHTML 是内嵌的 HTML 控制台（GET /）。
//
// 约束：这是 Go raw string literal（反引号包裹），因此内部不得出现反引号——
// 故 JS 一律用字符串拼接（'...'），不用 ES6 模板字符串（`...`）。
//
// 模型名经 encodeURIComponent 编码后放进 data-del 属性，点击时解码，
// 避免用户输入的模型名含引号打断 onclick（事件委托 + 编码，而非内联 onclick）。
const consoleHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>mock_price_backend 控制台</title>
<style>
  * { box-sizing: border-box; }
  body { margin:0; font-family: -apple-system, "PingFang SC", system-ui, sans-serif;
         background:#0f172a; color:#e2e8f0; }
  .wrap { max-width: 1100px; margin:0 auto; padding:24px; }
  h1 { font-size:20px; margin:0 0 4px; }
  .sub { color:#94a3b8; font-size:13px; margin-bottom:20px; min-height:18px; }
  .card { background:#1e293b; border:1px solid #334155; border-radius:10px; padding:16px; margin-bottom:16px; }
  .card h2 { font-size:14px; margin:0 0 12px; color:#cbd5e1; font-weight:600; }
  .btn { background:#2563eb; color:#fff; border:0; border-radius:6px; padding:8px 12px;
         font-size:13px; cursor:pointer; margin:0 6px 6px 0; }
  .btn:hover { background:#1d4ed8; }
  .btn.warn { background:#b45309; } .btn.warn:hover { background:#92400e; }
  .btn.danger { background:#b91c1c; } .btn.danger:hover { background:#991b1b; }
  .btn.ghost { background:#334155; } .btn.ghost:hover { background:#475569; }
  input { background:#0f172a; border:1px solid #334155; color:#e2e8f0; border-radius:6px;
          padding:7px 10px; font-size:13px; width:140px; }
  label { font-size:12px; color:#94a3b8; margin-right:4px; }
  table { width:100%; border-collapse:collapse; font-size:13px; }
  th, td { text-align:left; padding:8px 10px; border-bottom:1px solid #334155; }
  th { color:#94a3b8; font-weight:500; font-size:12px; }
  td.num { font-family: ui-monospace, "SF Mono", monospace; }
  .pill { background:#334155; padding:2px 8px; border-radius:99px; font-size:11px; color:#cbd5e1; margin-left:8px; }
  .pill.on { background:#166534; color:#bbf7d0; }
  .logs { max-height:240px; overflow:auto; }
  .logs td { font-size:12px; }
  .muted { color:#64748b; }
  .ok { color:#4ade80; } .fail { color:#f87171; }
  .row { display:flex; gap:8px; flex-wrap:wrap; align-items:center; margin-bottom:8px; }
  p.hint { font-size:12px; margin:10px 0 0; }
</style>
</head>
<body>
<div class="wrap">
  <h1>mock_price_backend 控制台</h1>
  <div class="sub" id="sub">假上游定价服务 · 用于测试 sub2api 上游价格同步</div>

  <div class="card">
    <h2>预设场景（一键触发 diff）</h2>
    <button class="btn" onclick="scenario('reset')">reset 基线</button>
    <button class="btn" onclick="scenario('hike')">hike 全涨 20%</button>
    <button class="btn" onclick="scenario('cut')">cut 全降 20%</button>
    <button class="btn" onclick="scenario('add')">add 新增 2 个</button>
    <button class="btn" onclick="scenario('remove')">remove 下架末位</button>
    <button class="btn" onclick="scenario('big')">big 首模型 +50%</button>
    <button class="btn" onclick="scenario('tiny')">tiny 首模型 +1%</button>
    <p class="hint muted">典型流程：reset → 去 sub2api 同步一次建立基线 → 切场景 → 再同步 → 在 changes 页看变动。</p>
  </div>

  <div class="card">
    <h2>故障注入</h2>
    <div class="row">
      <label>失败状态码</label><input id="fail" placeholder="如 500">
      <label>延迟(毫秒)</label><input id="delay" placeholder="如 30000">
      <button class="btn warn" onclick="setBehav()">应用</button>
      <button class="btn ghost" onclick="clearBehav()">清除</button>
    </div>
    <p class="hint muted">delay_ms ≥ 30000 可触发 SyncSource 超时（30s）；fail_status 非 2xx 触发同步失败（last_sync_status=failed）。</p>
  </div>

  <div class="card">
    <h2>手动增改模型</h2>
    <div class="row">
      <input id="mName" placeholder="模型名">
      <input id="mRatio" placeholder="价格倍率">
      <input id="mComp" placeholder="补全比（默认 1）">
      <button class="btn" onclick="upsert()">提交</button>
    </div>
    <p class="hint muted">价格倍率（model_ratio）是相对 $2/1M token 的倍率；每 token 输入价 = model_ratio × 2 / 1e6。</p>
  </div>

  <div class="card">
    <h2>当前模型集 <span class="pill" id="authTag"></span></h2>
    <table>
      <thead><tr><th>模型名</th><th>价格倍率</th><th>补全比</th><th>输入价 / token</th><th>输出价 / token</th><th>操作</th></tr></thead>
      <tbody id="models"></tbody>
    </table>
  </div>

  <div class="card">
    <h2>请求日志（SyncSource 拉取记录）<span class="pill" id="cnt"></span></h2>
    <div class="logs"><table>
      <thead><tr><th>时间</th><th>请求路径</th><th>令牌</th><th>结果</th></tr></thead>
      <tbody id="logs"></tbody>
    </table></div>
  </div>
</div>
<script>
function post(url, body){
  return fetch(url, {method:'POST', headers:{'Content-Type':'application/json'}, body: body ? JSON.stringify(body) : undefined});
}
function scenario(n){
  post('/admin/scenario/' + encodeURIComponent(n)).then(function(r){return r.json();}).then(function(m){ flash(m.message || m.error); load(); });
}
function upsert(){
  var name = document.getElementById('mName').value.trim();
  var ratio = parseFloat(document.getElementById('mRatio').value);
  var comp = parseFloat(document.getElementById('mComp').value) || 1;
  if(!name || !(ratio > 0)){ alert('模型名 与 价格倍率(>0) 必填'); return; }
  post('/admin/models', {model_name:name, model_ratio:ratio, completion_ratio:comp})
    .then(function(r){return r.json();}).then(function(m){ flash(m.message || m.error); load(); });
}
function del(name){ fetch('/admin/models/' + encodeURIComponent(name), {method:'DELETE'}).then(load); }
function setBehav(){
  var f = parseInt(document.getElementById('fail').value) || 0;
  var d = parseInt(document.getElementById('delay').value) || 0;
  post('/admin/behaviour', {fail_status:f, delay_ms:d}).then(function(){ flash('已应用故障行为'); load(); });
}
function clearBehav(){ post('/admin/behaviour', {fail_status:0, delay_ms:0}).then(function(){ flash('已清除故障行为'); load(); }); }
function flash(s){ document.getElementById('sub').textContent = '› ' + s; }
function sci(n){ return Number(n).toExponential(3); }
function esc(s){ var m={'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}; return String(s).replace(/[&<>"]/g, function(c){ return m[c]; }); }
function load(){
  fetch('/admin/state').then(function(r){return r.json();}).then(function(st){
    var rows = (st.models || []).map(function(m){
      var enc = encodeURIComponent(m.name);
      return '<tr><td>' + esc(m.name) + '</td><td class="num">' + m.model_ratio.toFixed(4) + '</td>'
        + '<td class="num">' + m.completion_ratio.toFixed(2) + '</td>'
        + '<td class="num">' + sci(m.in_per_token) + '</td>'
        + '<td class="num">' + sci(m.out_per_token) + '</td>'
        + '<td><button class="btn danger" data-del="' + enc + '">下架</button></td></tr>';
    }).join('');
    document.getElementById('models').innerHTML = rows;

    var lg = (st.logs || []).slice().reverse().map(function(l){
      var cls = l.note === 'ok' ? 'ok' : (l.note.indexOf('fail') >= 0 ? 'fail' : '');
      return '<tr><td class="muted">' + l.at.replace('T',' ').slice(0,19) + '</td>'
        + '<td>' + esc(l.path) + '</td>'
        + '<td class="muted">' + esc(l.token || '-') + '</td>'
        + '<td class="' + cls + '">' + esc(l.note) + '</td></tr>';
    }).join('');
    document.getElementById('logs').innerHTML = lg;

    document.getElementById('authTag').textContent = st.need_auth ? '需 Bearer' : '无鉴权';
    document.getElementById('authTag').className = 'pill ' + (st.need_auth ? '' : 'on');

    var c = st.counts || {};
    var total = (c['/api/pricing'] || 0) + (c['/api/pricing/litellm'] || 0) + (c['/api/pricing/custom'] || 0);
    document.getElementById('cnt').textContent = '累计定价请求 ' + total;
  });
}
// 事件委托：处理「下架」按钮（模型名经编码放进 data-del，安全）。
document.addEventListener('click', function(e){
  var t = e.target;
  var name = t && t.getAttribute && t.getAttribute('data-del');
  if(name){ del(decodeURIComponent(name)); }
});
load();
setInterval(load, 2000);
</script>
</body>
</html>`
