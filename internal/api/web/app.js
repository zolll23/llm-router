const tabs = document.querySelectorAll(".app-nav [data-tab]");
const sections = document.querySelectorAll(".tab");

const TAB_PATH = {
  usage: "/usage",
  quality: "/quality",
  models: "/models",
  recommendations: "/recommendations",
  settings: "/settings",
};
const PATH_TAB = {
  "/": "usage",
  "/usage": "usage",
  "/quality": "quality",
  "/models": "models",
  "/recommendations": "recommendations",
  "/settings": "settings",
};
const TAB_TITLE = {
  usage: "Usage",
  quality: "Качество",
  models: "Модели",
  recommendations: "Рекомендации",
  settings: "Настройки",
};

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

function tabFromPath(pathname) {
  return PATH_TAB[pathname] || "usage";
}

function activateTab(tab, { push = false } = {}) {
  if (!TAB_PATH[tab]) tab = "usage";
  tabs.forEach((b) => b.classList.toggle("active", b.dataset.tab === tab));
  sections.forEach((s) => s.classList.toggle("active", s.id === tab));
  const path = TAB_PATH[tab];
  document.title = `Router · ${TAB_TITLE[tab] || tab}`;
  if (push && location.pathname !== path) {
    history.pushState({ tab }, "", path);
  } else if (!push && location.pathname === "/" && path !== location.pathname) {
    history.replaceState({ tab }, "", path);
  }
  if (tab === "usage") {
    loadSankey();
    loadOverview();
  } else if (tab === "quality") {
    loadQuality();
  } else if (tab === "models") {
    loadModelsPage();
  } else if (tab === "recommendations") {
    loadRecs();
  } else if (tab === "settings") {
    loadSettings();
  }
}

tabs.forEach((el) => {
  el.addEventListener("click", (e) => {
    e.preventDefault();
    activateTab(el.dataset.tab, { push: true });
  });
});

window.addEventListener("popstate", () => {
  activateTab(tabFromPath(location.pathname), { push: false });
});

const SANKEY_COLORS = [
  "#0f6b4c", "#2a7cb8", "#c47a2c", "#8b5bb0",
  "#c45c6a", "#3d8f6e", "#5a9bc4", "#b8843a",
  "#6b9e8a", "#9b6bb0", "#a8b87e", "#7a8ab8",
];

const CHART_COLORS = ["#0f6b4c", "#2a7cb8", "#c47a2c", "#8b5bb0", "#c45c6a", "#3d8f6e"];
const CX_ORDER = ["trivial", "simple", "medium", "hard"];
const CAT_TITLE = {
  coding: "Кодинг",
  chat: "Чат",
  reasoning: "Reasoning",
  creative: "Creative",
};
let chartCategories = null;
let chartComplexity = null;
let chartByCategory = [];
let modelInfoModal = null;
let lastRecPayload = null;

function esc(s) {
  return String(s ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function fmtDate(iso) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("ru-RU", {
      day: "2-digit",
      month: "short",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch (_) {
    return iso;
  }
}

function destroyCharts() {
  if (chartCategories) {
    chartCategories.destroy();
    chartCategories = null;
  }
  if (chartComplexity) {
    chartComplexity.destroy();
    chartComplexity = null;
  }
  for (const ch of chartByCategory) ch.destroy();
  chartByCategory = [];
}

function doughnutOpts() {
  return {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        position: "bottom",
        labels: { boxWidth: 12, font: { family: "Outfit", size: 12 } },
      },
    },
  };
}

function makeDoughnut(canvas, labels, values) {
  const empty = !labels.length;
  return new Chart(canvas, {
    type: "doughnut",
    data: {
      labels: empty ? ["нет данных"] : labels,
      datasets: [{
        data: empty ? [1] : values,
        backgroundColor: empty ? ["#d7e0db"] : labels.map((_, i) => CHART_COLORS[i % CHART_COLORS.length]),
        borderWidth: 0,
      }],
    },
    options: doughnutOpts(),
  });
}

function orderedComplexityLabels(byCx) {
  const present = new Set(Object.keys(byCx));
  const ordered = CX_ORDER.filter((k) => present.has(k));
  for (const k of present) {
    if (!ordered.includes(k)) ordered.push(k);
  }
  return ordered;
}

function catTitle(cat) {
  return CAT_TITLE[cat] || cat;
}

function renderLoadCharts(profile) {
  destroyCharts();
  const cells = profile || [];
  const byCat = {};
  const byCx = {};
  const byCatCx = {};
  for (const c of cells) {
    const cat = c.category || "chat";
    const cx = c.complexity || "simple";
    const n = Number(c.requests || 0);
    byCat[cat] = (byCat[cat] || 0) + n;
    byCx[cx] = (byCx[cx] || 0) + n;
    if (!byCatCx[cat]) byCatCx[cat] = {};
    byCatCx[cat][cx] = (byCatCx[cat][cx] || 0) + n;
  }
  const catLabels = Object.keys(byCat).sort((a, b) => byCat[b] - byCat[a]);
  const cxLabels = orderedComplexityLabels(byCx);
  const catCanvas = document.getElementById("chart-categories");
  const cxCanvas = document.getElementById("chart-complexity");
  if (!catCanvas || !cxCanvas || typeof Chart === "undefined") return;

  chartCategories = makeDoughnut(catCanvas, catLabels, catLabels.map((k) => byCat[k]));
  chartComplexity = makeDoughnut(cxCanvas, cxLabels, cxLabels.map((k) => byCx[k]));

  const wrap = document.getElementById("chart-by-category");
  if (!wrap) return;
  wrap.innerHTML = "";
  const cats = catLabels.length ? catLabels : [];
  if (!cats.length) {
    wrap.innerHTML = `<div class="col-12"><p class="text-muted mb-0">Нет данных по категориям за период</p></div>`;
    return;
  }
  for (const cat of cats) {
    const col = document.createElement("div");
    col.className = "col-md-6 col-lg-4";
    const total = byCat[cat] || 0;
    col.innerHTML = `
      <div class="panel chart-panel">
        <h3 class="section-subtitle">${esc(catTitle(cat))} <span class="priority-meta">· ${fmt(total)}</span></h3>
        <div class="chart-box"><canvas></canvas></div>
      </div>`;
    wrap.appendChild(col);
    const canvas = col.querySelector("canvas");
    const cxMap = byCatCx[cat] || {};
    const labels = orderedComplexityLabels(cxMap);
    chartByCategory.push(makeDoughnut(canvas, labels, labels.map((k) => cxMap[k])));
  }
}

function showModelInfo(sectionIdx, modelIdx) {
  const sec = lastRecPayload?.priority_sections?.[sectionIdx];
  const m = sec?.models?.[modelIdx];
  if (!m) return;
  document.getElementById("modelInfoTitle").textContent = m.display_name || m.model_id || "Модель";
  const chars = m.characteristics || {};
  const charRows = Object.keys(chars)
    .map((k) => `<tr><td>${esc(k)}</td><td>${esc(chars[k])}</td></tr>`)
    .join("");
  const criteria = (m.criteria || []).map((c) => `<li>${esc(c)}</li>`).join("");
  document.getElementById("modelInfoBody").innerHTML = `
    <p class="mb-2"><strong>Качество (score):</strong> ${m.score != null ? Number(m.score).toFixed(0) : "—"}
      · <strong>Params:</strong> ${m.params || "—"}B
      · <strong>Mem≈:</strong> ${m.mem_req_gb != null ? m.mem_req_gb + " GB" : "—"}
      · <strong>Tier≤:</strong> ${m.tier_budget_gb != null ? m.tier_budget_gb + " GB" : "—"}</p>
    <p class="mb-3 d-flex flex-wrap align-items-center gap-2">
      ${m.ollama_pull ? `<code class="mb-0">${esc(m.ollama_pull)}</code>
      <button type="button" class="btn btn-sm btn-outline-accent btn-copy-modal" data-pull="${esc(m.ollama_pull)}">Копировать</button>` : `<span class="text-muted">Нет проверенного тега для ollama pull</span>`}
      ${m.ollama_url ? `<a class="btn btn-sm btn-outline-accent" href="${esc(m.ollama_url)}" target="_blank" rel="noopener">ollama.com</a>` : ""}
    </p>
    <h6>Характеристики</h6>
    <div class="table-wrap mb-3"><table class="table table-sm mb-0"><tbody>${charRows || "<tr><td>—</td></tr>"}</tbody></table></div>
    <h6>Почему выбрана</h6>
    <ul class="criteria-list">${criteria || "<li>—</li>"}</ul>`;
  if (!modelInfoModal) {
    modelInfoModal = new bootstrap.Modal(document.getElementById("modelInfoModal"));
  }
  const copyBtn = document.querySelector("#modelInfoBody .btn-copy-modal");
  if (copyBtn) {
    copyBtn.addEventListener("click", async () => {
      const cmd = copyBtn.dataset.pull || "";
      try {
        await navigator.clipboard.writeText(cmd);
        copyBtn.textContent = "Скопировано";
        setTimeout(() => { copyBtn.textContent = "Копировать"; }, 1400);
      } catch (_) {
        window.prompt("Скопируй команду:", cmd);
      }
    });
  }
  modelInfoModal.show();
}

function renderPrioritySections(sections) {
  const root = document.getElementById("rec-priority");
  if (!sections?.length) {
    root.innerHTML = "<p class=\"text-muted\">Нет секций приоритета — нажми «Подобрать».</p>";
    return;
  }
  let html = "";
  sections.forEach((sec, si) => {
    const gapBadge = sec.gap
      ? `<span class="badge badge-gap">не хватает в inventory</span>`
      : `<span class="badge badge-ok">есть кандидаты</span>`;
    html += `<section class="panel priority-block">
      <div class="priority-head">
        <h3>#${sec.priority || si + 1} · ${esc(sec.title || (sec.category + " — " + sec.complexity))}</h3>
        <div class="priority-meta">
          ${gapBadge}
          · ${fmt(sec.requests || 0)} запр.
          · ${sec.share_pct != null ? Number(sec.share_pct).toFixed(1) + "%" : "—"}
          · tier ≤ ${sec.tier_budget_gb ?? "—"} GB
          · рейтинг: ${fmtDate(sec.updated_at)}
        </div>
      </div>
      <div class="table-wrap"><table class="table table-sm align-middle mb-0">
        <thead><tr>
          <th>Модель</th><th>Размер</th><th>Mem≈</th><th>Качество</th><th>Есть?</th><th></th>
        </tr></thead><tbody>`;
    for (let mi = 0; mi < (sec.models || []).length; mi++) {
      const m = sec.models[mi];
      const size = m.params ? m.params + "B" : "—";
      const has = m.in_inventory
        ? `<span class="badge badge-ok">да</span>`
        : `<span class="badge badge-gap">нет</span>`;
      const pull = m.ollama_pull || "";
      const lib = m.ollama_url
        ? `<a href="${esc(m.ollama_url)}" target="_blank" rel="noopener" title="Страница на ollama.com"><i class="bi bi-box-arrow-up-right"></i></a>`
        : "";
      const ollama = pull
        ? `<button type="button" class="btn-copy-pull" title="Скопировать: ${esc(pull)}" data-pull="${esc(pull)}"><i class="bi bi-terminal"></i></button>`
        : "";
      const bench = m.benchmark_url
        ? `<a href="${esc(m.benchmark_url)}" target="_blank" rel="noopener" title="Бенчмарк"><i class="bi bi-bar-chart-line"></i></a>`
        : "";
      html += `<tr>
        <td>${esc(m.display_name || m.model_id)}</td>
        <td>${size}${m.mlx ? ' <span class="text-muted">MLX</span>' : ""}</td>
        <td>${m.mem_req_gb != null ? m.mem_req_gb + " GB" : "—"}</td>
        <td>${m.score != null ? Number(m.score).toFixed(0) : "—"}</td>
        <td>${has}</td>
        <td><div class="icon-actions">
          ${ollama}${lib}${bench}
          <button type="button" title="Информация" data-sec="${si}" data-model="${mi}" class="btn-info-model"><i class="bi bi-info-circle"></i></button>
        </div></td>
      </tr>`;
    }
    if (!(sec.models || []).length) {
      html += `<tr><td colspan="6" class="text-muted">Нет подходящих моделей в каталоге для этого tier</td></tr>`;
    }
    html += `</tbody></table></div></section>`;
  });
  root.innerHTML = html;
  root.querySelectorAll(".btn-info-model").forEach((btn) => {
    btn.addEventListener("click", () => {
      showModelInfo(Number(btn.dataset.sec), Number(btn.dataset.model));
    });
  });
  root.querySelectorAll(".btn-copy-pull").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const cmd = btn.dataset.pull || "";
      try {
        await navigator.clipboard.writeText(cmd);
        const prev = btn.getAttribute("title");
        btn.classList.add("copied");
        btn.setAttribute("title", "Скопировано");
        btn.innerHTML = '<i class="bi bi-check2"></i>';
        setTimeout(() => {
          btn.classList.remove("copied");
          btn.setAttribute("title", prev || cmd);
          btn.innerHTML = '<i class="bi bi-terminal"></i>';
        }, 1400);
      } catch (_) {
        window.prompt("Скопируй команду:", cmd);
      }
    });
  });
}

function colorFor(id) {
  let h = 0;
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) >>> 0;
  return SANKEY_COLORS[h % SANKEY_COLORS.length];
}

async function loadSankey() {
  const svg = d3.select("#sankey");
  const empty = document.getElementById("sankey-empty");
  const wrap = document.getElementById("sankey-wrap");
  const data = await api("/api/stats/sankey?days=7");
  svg.selectAll("*").remove();
  if (!data.links || !data.links.length) {
    empty.hidden = false;
    svg.attr("height", 0);
    return;
  }
  empty.hidden = true;

  const width = Math.max(wrap.clientWidth || 960, 720);
  const height = 440;
  const margin = { top: 36, right: 120, bottom: 16, left: 110 };
  const stageTitles = {
    category: "Категория",
    complexity: "Сложность",
    pool: "Пул",
    model: "Модель",
    status: "Статус",
  };

  svg.attr("viewBox", `0 0 ${width} ${height}`).attr("width", "100%").attr("height", height);

  const nodes = (data.nodes || []).map((n) => ({ ...n }));
  const nodeById = Object.fromEntries(nodes.map((n) => [n.id, n]));
  const links = (data.links || [])
    .filter((l) => nodeById[l.source] && nodeById[l.target])
    .map((l) => ({
      source: l.source,
      target: l.target,
      value: Number(l.value) || 0,
    }));

  const sankeyFn = d3.sankey || (typeof d3Sankey !== "undefined" && d3Sankey.sankey);
  const linkHoriz = d3.sankeyLinkHorizontal || (typeof d3Sankey !== "undefined" && d3Sankey.sankeyLinkHorizontal);
  if (!sankeyFn || !linkHoriz) {
    empty.hidden = false;
    empty.textContent = "Не удалось загрузить d3-sankey";
    return;
  }

  const sankey = sankeyFn()
    .nodeId((d) => d.id)
    .nodeWidth(12)
    .nodePadding(18)
    .extent([
      [margin.left, margin.top],
      [width - margin.right, height - margin.bottom],
    ]);

  const graph = sankey({
    nodes: nodes.map((d) => ({ ...d })),
    links: links.map((d) => ({ ...d })),
  });

  const g = svg.append("g");

  const stageX = {};
  for (const n of graph.nodes) {
    if (!n.stage || stageX[n.stage] != null) continue;
    stageX[n.stage] = (n.x0 + n.x1) / 2;
  }
  const stageOrder = ["category", "complexity", "pool", "model", "status"];
  g.append("g")
    .attr("class", "sankey-stages")
    .selectAll("text")
    .data(stageOrder.filter((s) => stageX[s] != null))
    .join("text")
    .attr("class", "sankey-stage-title")
    .attr("x", (s) => stageX[s])
    .attr("y", 18)
    .attr("text-anchor", "middle")
    .text((s) => stageTitles[s] || s);

  g.append("g")
    .attr("fill", "none")
    .selectAll("path")
    .data(graph.links)
    .join("path")
    .attr("d", linkHoriz())
    .attr("stroke", (d) => colorFor(d.source.id))
    .attr("stroke-opacity", 0.45)
    .attr("stroke-width", (d) => Math.max(1, d.width))
    .append("title")
    .text((d) => `${d.source.name} → ${d.target.name}: ${d.value}`);

  const node = g
    .append("g")
    .selectAll("g")
    .data(graph.nodes)
    .join("g");

  node
    .append("rect")
    .attr("x", (d) => d.x0)
    .attr("y", (d) => d.y0)
    .attr("height", (d) => Math.max(1, d.y1 - d.y0))
    .attr("width", (d) => d.x1 - d.x0)
    .attr("fill", (d) => colorFor(d.id))
    .attr("rx", 1)
    .append("title")
    .text((d) => `${d.name}: ${d.value ?? ""}`);

  node.each(function (d) {
    const el = d3.select(this);
    const label = `${d.name} ${fmt(d.value ?? 0)}`;
    const midY = (d.y0 + d.y1) / 2;
    const isLeft = d.stage === "category";
    const isRight = d.stage === "status";
    const isMid = !isLeft && !isRight;

    if (isMid) {
      const tx = (d.x0 + d.x1) / 2;
      const tw = Math.min(140, Math.max(72, label.length * 7));
      el.append("rect")
        .attr("class", "sankey-chip-bg")
        .attr("x", tx - tw / 2)
        .attr("y", midY - 12)
        .attr("width", tw)
        .attr("height", 24)
        .attr("rx", 8);
      el.append("text")
        .attr("x", tx)
        .attr("y", midY)
        .attr("dy", "0.35em")
        .attr("text-anchor", "middle")
        .attr("class", "sankey-chip-text")
        .text(label);
    } else {
      el.append("text")
        .attr("x", isLeft ? d.x0 - 8 : d.x1 + 8)
        .attr("y", midY)
        .attr("dy", "0.35em")
        .attr("text-anchor", isLeft ? "end" : "start")
        .attr("class", "sankey-label")
        .text(label);
    }
  });
}

function fmt(n) {
  return (n ?? 0).toLocaleString("ru-RU");
}

function metricCells(w) {
  if (!w) {
    return "<td>—</td><td>—</td><td>—</td><td>—</td><td>—</td>";
  }
  return `<td>${fmt(w.prompt_tokens)}</td><td>${fmt(w.cached_tokens)}</td><td>${fmt(w.completion_tokens)}</td><td>${fmt(w.requests)}</td><td>${Number(w.share_pct || 0).toFixed(1)}%</td>`;
}

function applyHardware(hw, note) {
  if (!hw) return;
  if (hw.chip) document.getElementById("rec-chip").value = hw.chip;
  if (hw.ram) document.getElementById("rec-ram").value = hw.ram;
  if (hw.vram) document.getElementById("rec-vram").value = hw.vram;
  else if (hw.ram) document.getElementById("rec-vram").value = hw.ram;
  document.getElementById("rec-mlx").checked = !!hw.prefer_mlx;
  const el = document.getElementById("rec-hw-note");
  if (el) {
    el.textContent = note || `Профиль: ${hw.chip || "—"}, RAM ${hw.ram || "—"} GB, source=${hw.source || "—"}`;
  }
}

async function fillMyHardware() {
  let hw = null;
  let note = "";
  try {
    hw = await api("/api/benchmarks/host");
  } catch (_) {
    hw = null;
  }
  // Prefer host-detect file (written on Mac before docker-up). Preset only as last resort.
  const fromHostFile = hw && hw.ram > 0 && (hw.source === "host-detect" || hw.source === "host-file" || hw.source === "sysctl");
  if (fromHostFile) {
    note = `Железо с хоста (${hw.source}): ${hw.chip || "—"}, ${hw.ram} GB` +
      (hw.unified || hw.prefer_mlx ? " unified, MLX." : ".");
    if (hw.arch === "apple_silicon" || hw.prefer_mlx) {
      hw.prefer_mlx = true;
      hw.unified = true;
    }
  } else if (!hw || !hw.ram || hw.source === "container") {
    const profiles = await api("/api/benchmarks/profiles");
    hw = profiles.default || profiles.profiles?.[0];
    note = "Файл host_hardware.json не найден. Запусти `make detect-host` (или `make docker-up`) на Mac. Пока пресет Mac M5 · 36 GB.";
  } else {
    note = `Профиль: ${hw.chip || "—"}, RAM ${hw.ram || "—"} GB (${hw.source || "request"}).`;
  }
  applyHardware(hw, note);
  await loadRecs();
}

async function loadRecs() {
  const body = {
    chip: document.getElementById("rec-chip").value,
    ram: Number(document.getElementById("rec-ram").value || 0),
    vram: Number(document.getElementById("rec-vram").value || 0),
    prefer_mlx: document.getElementById("rec-mlx").checked,
    unified: document.getElementById("rec-mlx").checked,
    arch: document.getElementById("rec-mlx").checked ? "apple_silicon" : "",
    source: "request",
  };
  const res = await api("/api/benchmarks/recommend", {
    method: "POST",
    body: JSON.stringify(body),
  });
  lastRecPayload = res;

  const loadNote = document.getElementById("rec-load-note");
  if (loadNote) {
    const notes = (res.notes || []).join(" ");
    loadNote.textContent = res.load_total
      ? `За 7 дней: ${fmt(res.load_total)} запросов. Бюджет ~${res.budget_gb} GB. ${notes}`
      : `Пока нет usage — показана полная матрица. Бюджет ~${res.budget_gb} GB. ${notes}`;
  }

  renderLoadCharts(res.load_profile || []);
  renderPrioritySections(res.priority_sections || []);
}

document.getElementById("rec-btn").addEventListener("click", loadRecs);
document.getElementById("rec-fill-btn").addEventListener("click", fillMyHardware);

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
  const periods = [
    { key: "day", label: "День" },
    { key: "week", label: "Неделя" },
    { key: "month", label: "Месяц" },
  ];
  let html = `<table><thead><tr>
    <th>Модель</th><th>Период</th><th>in</th><th>cache</th><th>out</th><th>reqs</th><th>share</th>
  </tr></thead><tbody>`;
  for (const id of ids) {
    const r = byId[id];
    periods.forEach((p, i) => {
      html += `<tr class="clickable" data-id="${esc(id)}">`;
      if (i === 0) {
        html += `<td class="model-span" rowspan="3">${esc(id)}</td>`;
      }
      html += `<td class="period-cell">${p.label}</td>${metricCells(r[p.key])}</tr>`;
    });
  }
  html += `</tbody></table>`;
  document.getElementById("overview").innerHTML = ids.length ? html : "<p>Пока нет данных</p>";
  document.querySelectorAll("#overview tr.clickable").forEach((tr) => {
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

function fmtReset(iso) {
  if (!iso || iso.startsWith("0001")) return "";
  try {
    return "сброс " + new Date(iso).toLocaleString("ru-RU", { day: "2-digit", month: "short", hour: "2-digit", minute: "2-digit" });
  } catch (_) {
    return "";
  }
}

function quotaBar(label, win) {
  if (!win) return "";
  const used = Number(win.used_percent || 0);
  const reset = fmtReset(win.resets_at);
  return `<div class="mb-2">
    <div class="d-flex justify-content-between small"><span>${esc(label)}</span><span>${used.toFixed(0)}% used${reset ? " · " + reset : ""}</span></div>
    <div class="progress" style="height:8px"><div class="progress-bar bg-success" style="width:${Math.min(100, used)}%"></div></div>
  </div>`;
}

async function loadModelsPage() {
  const [modelsRes, quotasRes] = await Promise.all([
    api("/api/models"),
    api("/api/quotas"),
  ]);
  const cards = document.getElementById("quota-cards");
  const providers = quotasRes.providers || [];
  if (!providers.length) {
    cards.innerHTML = `<div class="col-12"><p class="text-muted mb-0">Нет адаптеров квот. Добавь в конфиг <code>quota: opencode_go</code> или <code>ollama_balance</code>.</p></div>`;
  } else {
    cards.innerHTML = providers.map((p) => {
      const err = p.error ? `<p class="text-danger small mb-2">${esc(p.error)}</p>` : "";
      return `<div class="col-md-6 col-lg-4"><div class="panel p-3">
        <h3 class="section-subtitle mb-2">${esc(p.provider_id)} <span class="priority-meta">${esc(p.source || "")}</span></h3>
        ${err}
        ${quotaBar("Rolling", p.rolling)}
        ${quotaBar("Weekly", p.weekly)}
        ${quotaBar("Monthly", p.monthly)}
      </div></div>`;
    }).join("");
  }
  const models = modelsRes.models || [];
  let html = `<table class="table table-sm align-middle mb-0"><thead><tr>
    <th>Провайдер</th><th>Модель</th><th>Tier</th><th>Params</th><th>Routable</th><th>Tools</th><th>Задержка</th><th></th>
  </tr></thead><tbody>`;
  for (const m of models) {
    const lat = m.metrics?.latency_p50 != null ? Math.round(m.metrics.latency_p50) + " ms" : "—";
    const links = [];
    if (m.ollama_url) links.push(`<a href="${esc(m.ollama_url)}" target="_blank" rel="noopener" title="Ollama"><i class="bi bi-box-arrow-up-right"></i></a>`);
    if (m.benchmark_url) links.push(`<a href="${esc(m.benchmark_url)}" target="_blank" rel="noopener" title="Бенчмарк"><i class="bi bi-bar-chart-line"></i></a>`);
    html += `<tr>
      <td>${esc(m.provider_id)}</td>
      <td>${esc(m.display_name || m.upstream_id || m.id)}</td>
      <td>${esc(m.cost_tier || "—")}</td>
      <td>${m.params || "—"}</td>
      <td>${m.routable ? "да" : "нет"}</td>
      <td>${m.tools ? "да" : "нет"}</td>
      <td>${lat}</td>
      <td><div class="icon-actions">${links.join("")}</div></td>
    </tr>`;
  }
  html += `</tbody></table>`;
  document.getElementById("models-table").innerHTML = models.length ? html : "<p class=\"text-muted p-3 mb-0\">Нет моделей в inventory</p>";
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
  form.escalate_min_complexity.value = st.escalate_min_complexity || "hard";
  form.quota_threshold_rolling_pct.value = st.quota_threshold_rolling_pct ?? 85;
  form.quota_threshold_weekly_pct.value = st.quota_threshold_weekly_pct ?? 90;
  form.quota_threshold_monthly_pct.value = st.quota_threshold_monthly_pct ?? 95;
  form.quota_cooldown_seconds.value = st.quota_cooldown_seconds ?? 600;
  form.local_queue_max.value = st.local_queue_max ?? 8;
  form.local_queue_redirect.checked = !!st.local_queue_redirect;
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
    escalate_min_complexity: f.escalate_min_complexity.value,
    quota_threshold_rolling_pct: Number(f.quota_threshold_rolling_pct.value || 85),
    quota_threshold_weekly_pct: Number(f.quota_threshold_weekly_pct.value || 90),
    quota_threshold_monthly_pct: Number(f.quota_threshold_monthly_pct.value || 95),
    quota_cooldown_seconds: Number(f.quota_cooldown_seconds.value || 600),
    local_queue_max: Number(f.local_queue_max.value ?? 8),
    local_queue_redirect: !!f.local_queue_redirect.checked,
  };
  await api("/api/settings", { method: "PUT", body: JSON.stringify(body) });
  await loadSettings();
});

document.getElementById("sync-btn").addEventListener("click", async () => {
  await api("/api/inventory/sync", { method: "POST" });
  await loadSettings();
});

document.getElementById("models-refresh-btn").addEventListener("click", async () => {
  await api("/api/inventory/sync", { method: "POST" });
  await loadModelsPage();
});

document.getElementById("judge-btn").addEventListener("click", async () => {
  const r = await api("/api/eval/judge-run", { method: "POST" });
  alert("Judged: " + (r.judged ?? 0));
  await loadQuality();
});

activateTab(tabFromPath(location.pathname), { push: false });
