const tabs = document.querySelectorAll("nav button");
const sections = document.querySelectorAll(".tab");

function apiToken() {
  return localStorage.getItem("router_token") || "local";
}

async function api(path, opts = {}) {
  const headers = Object.assign({}, opts.headers || {});
  headers.Authorization = "Bearer " + apiToken();
  if (opts.body && !headers["Content-Type"]) {
    headers["Content-Type"] = "application/json";
  }
  const r = await fetch(path, Object.assign({}, opts, { headers }));
  if (!r.ok) {
    const text = await r.text();
    throw new Error(text || r.statusText);
  }
  const ct = r.headers.get("content-type") || "";
  if (ct.includes("application/json")) return r.json();
  return r.text();
}

tabs.forEach((btn) => {
  btn.addEventListener("click", () => {
    tabs.forEach((b) => b.classList.remove("active"));
    sections.forEach((s) => s.classList.remove("active"));
    btn.classList.add("active");
    document.getElementById(btn.dataset.tab).classList.add("active");
    if (btn.dataset.tab === "usage") loadOverview();
    if (btn.dataset.tab === "quality") loadQuality();
    if (btn.dataset.tab === "recommendations") loadRecs();
    if (btn.dataset.tab === "settings") loadSettings();
  });
});

function fmt(n) {
  return (n ?? 0).toLocaleString("ru-RU");
}

function cell(w) {
  if (!w) return "<td>—</td>";
  return `<td>${fmt(w.prompt_tokens)} / ${fmt(w.cached_tokens)} / ${fmt(w.completion_tokens)} / ${fmt(w.requests)} / ${Number(w.share_pct || 0).toFixed(1)}%</td>`;
}

async function loadRecs() {
  const vram = document.getElementById("rec-vram").value;
  const ram = document.getElementById("rec-ram").value;
  const res = await api("/api/benchmarks/recommend", {
    method: "POST",
    body: JSON.stringify({ vram: Number(vram), ram: Number(ram) }),
  });
  const recs = res.recommendations || [];
  let html = `<table><thead><tr><th>Модель</th><th>Параметры (B)</th></tr></thead><tbody>`;
  for (const m of recs) {
    html += `<tr><td>${m.DisplayName}</td><td>${m.Params}</td></tr>`;
  }
  html += `</tbody></table>`;
  document.getElementById("rec-results").innerHTML = html || "<p>Подходящих моделей не найдено</p>";
}

document.getElementById("rec-btn").addEventListener("click", loadRecs);

async function loadOverview() {
  const ov = await api("/api/stats/overview");
  const byId = {};
  for (const row of ov.day || []) byId[row.ModelID || row.model_id] = { day: row };
  // Go json encodes exported fields as-is (ModelID). Normalize:
  const norm = (arr, key) => {
    for (const row of arr || []) {
      const id = row.ModelID || row.model_id;
      byId[id] = byId[id] || {};
      byId[id][key] = {
        prompt_tokens: row.PromptTokens ?? row.prompt_tokens,
        cached_tokens: row.CachedTokens ?? row.cached_tokens,
        completion_tokens: row.CompletionTokens ?? row.completion_tokens,
        requests: row.Requests ?? row.requests,
        share_pct: row.SharePct ?? row.share_pct,
        provider: row.ProviderID || row.provider_id,
      };
    }
  };
  norm(ov.day, "day");
  norm(ov.week, "week");
  norm(ov.month, "month");
  const ids = Object.keys(byId).sort();
  let html = `<table><thead><tr><th>Модель</th><th>День (in/cache/out/reqs/share)</th><th>Неделя</th><th>Месяц</th></tr></thead><tbody>`;
  for (const id of ids) {
    const r = byId[id];
    html += `<tr class="clickable" data-id="${id}"><td>${id}</td>${cell(r.day)}${cell(r.week)}${cell(r.month)}</tr>`;
  }
  html += `</tbody></table>`;
  document.getElementById("overview").innerHTML = html || "<p>Пока нет данных</p>";
  document.querySelectorAll("tr.clickable").forEach((tr) => {
    tr.addEventListener("click", () => loadDetail(tr.dataset.id));
  });
}

async function loadDetail(id) {
  const d = await api("/api/stats/models/" + encodeURIComponent(id));
  const lat = d.latency || {};
  document.getElementById("detail").innerHTML = `
    <h3>${id}</h3>
    <p>Latency ms — p50: ${lat.p50 ?? "—"}, p75: ${lat.p75 ?? "—"}, p90: ${lat.p90 ?? "—"}, p95: ${lat.p95 ?? "—"} (n=${lat.n ?? 0})</p>
    <pre>${JSON.stringify(d.series || [], null, 2)}</pre>`;
}

async function loadQuality() {
  const q = await api("/api/eval/summary");
  const c = await api("/api/eval/confusion");

  document.getElementById("quality-summary").innerHTML = `
    <table>
      <thead>
        <tr><th>Метрика</th><th>Значение</th></tr>
      </thead>
      <tbody>
        <tr><td>Всего запросов</td><td>${fmt(q.total)}</td></tr>
        <tr><td>Local share</td><td>${Number(q.local_share_pct || 0).toFixed(1)}%</td></tr>
        <tr><td>Escalations</td><td>${fmt(q.escalations)}</td></tr>
        <tr><td>Overrides</td><td>${fmt(q.overrides)}</td></tr>
        <tr><td>Sticky breaks</td><td>${fmt(q.sticky_breaks)}</td></tr>
      </tbody>
    </table>`;

  const labels = c.labels || [];
  const matrix = c.matrix || [];
  if (!labels.length) {
    document.getElementById("quality-confusion").innerHTML = "<p>Нет данных judge за период</p>";
    return;
  }
  let html = `<table><thead><tr><th>pred \\ gold</th>`;
  for (const label of labels) {
    html += `<th>${label}</th>`;
  }
  html += `</tr></thead><tbody>`;
  for (let i = 0; i < labels.length; i++) {
    html += `<tr><th scope="row">${labels[i]}</th>`;
    for (let j = 0; j < labels.length; j++) {
      const v = (matrix[i] && matrix[i][j]) || 0;
      html += `<td${i === j ? ' class="diag"' : ""}>${fmt(v)}</td>`;
    }
    html += `</tr>`;
  }
  html += `</tbody></table>`;
  document.getElementById("quality-confusion").innerHTML = html;
}

async function loadSettings() {
  const st = await api("/api/settings");
  const form = document.getElementById("settings-form");
  form.classifier.value = st.classifier || "gliner";
  form.gliner_base_url.value = st.gliner_base_url || "http://127.0.0.1:8090";
  form.nimble_model.value = st.nimble_model || "";
  form.nimble_base_url.value = st.nimble_base_url || "";
  form.sticky_ttl_minutes.value = st.sticky_ttl_minutes || 30;
  form.keep_alive.value = st.keep_alive || "60m";
  const inv = await api("/api/inventory");
  document.getElementById("inventory").textContent = JSON.stringify(inv, null, 2);
}

document.getElementById("settings-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const f = e.target;
  const body = {
    classifier: f.classifier.value,
    gliner_base_url: f.gliner_base_url.value,
    nimble_model: f.nimble_model.value,
    nimble_base_url: f.nimble_base_url.value,
    sticky_ttl_minutes: Number(f.sticky_ttl_minutes.value || 30),
    keep_alive: f.keep_alive.value,
    confidence_min: 0.4,
  };
  await api("/api/settings", { method: "PUT", body: JSON.stringify(body) });
  await loadSettings();
});

document.getElementById("sync-btn").addEventListener("click", async () => {
  await api("/api/inventory/sync", { method: "POST" });
  await loadSettings();
});

document.getElementById("judge-btn").addEventListener("click", async () => {
  const r = await api("/api/eval/judge-run", { method: "POST" });
  alert("Judged: " + (r.judged ?? 0));
  await loadQuality();
});

loadOverview();
