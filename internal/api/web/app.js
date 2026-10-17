const tabs = document.querySelectorAll("nav button");
const sections = document.querySelectorAll(".tab");
tabs.forEach((btn) => {
  btn.addEventListener("click", () => {
    tabs.forEach((b) => b.classList.remove("active"));
    sections.forEach((s) => s.classList.remove("active"));
    btn.classList.add("active");
    document.getElementById(btn.dataset.tab).classList.add("active");
    if (btn.dataset.tab === "usage") loadOverview();
    if (btn.dataset.tab === "quality") loadQuality();
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

async function loadOverview() {
  const ov = await fetch("/api/stats/overview").then((r) => r.json());
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
  const d = await fetch("/api/stats/models/" + encodeURIComponent(id)).then((r) => r.json());
  const lat = d.latency || {};
  document.getElementById("detail").innerHTML = `
    <h3>${id}</h3>
    <p>Latency ms — p50: ${lat.p50 ?? "—"}, p75: ${lat.p75 ?? "—"}, p90: ${lat.p90 ?? "—"}, p95: ${lat.p95 ?? "—"} (n=${lat.n ?? 0})</p>
    <pre>${JSON.stringify(d.series || [], null, 2)}</pre>`;
}

async function loadQuality() {
  const q = await fetch("/api/eval/summary").then((r) => r.json());
  document.getElementById("quality-box").textContent = JSON.stringify(q, null, 2);
}

async function loadSettings() {
  const st = await fetch("/api/settings").then((r) => r.json());
  const form = document.getElementById("settings-form");
  form.classifier.value = st.classifier || "nimble";
  form.nimble_model.value = st.nimble_model || "";
  form.nimble_base_url.value = st.nimble_base_url || "";
  form.sticky_ttl_minutes.value = st.sticky_ttl_minutes || 30;
  form.keep_alive.value = st.keep_alive || "60m";
  const inv = await fetch("/api/inventory").then((r) => r.json());
  document.getElementById("inventory").textContent = JSON.stringify(inv, null, 2);
}

document.getElementById("settings-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const f = e.target;
  const body = {
    classifier: f.classifier.value,
    nimble_model: f.nimble_model.value,
    nimble_base_url: f.nimble_base_url.value,
    sticky_ttl_minutes: Number(f.sticky_ttl_minutes.value || 30),
    keep_alive: f.keep_alive.value,
    confidence_min: 0.4,
  };
  await fetch("/api/settings", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  await loadSettings();
});

document.getElementById("sync-btn").addEventListener("click", async () => {
  await fetch("/api/inventory/sync", { method: "POST" });
  await loadSettings();
});

loadOverview();
