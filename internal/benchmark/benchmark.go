package benchmark

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"router-app/internal/provider"
	"router-app/internal/store"
)

type ModelBenchmark struct {
	ID             string  `json:"id"`
	CodingScore    float64 `json:"coding_score"`
	ReasoningScore float64 `json:"reasoning_score"`
	GeneralScore   float64 `json:"general_score"`
	CreativeScore  float64 `json:"creative_score"`
	Params         int     `json:"params"`   // billions
	VRAMReq        int     `json:"vram_req"` // GB approx working set
	MLX            bool    `json:"mlx"`
}

type BenchmarkStore struct {
	mu   sync.RWMutex
	data map[string]ModelBenchmark
	url  string
}

func NewStore(updateURL string) *BenchmarkStore {
	return &BenchmarkStore{
		data: make(map[string]ModelBenchmark),
		url:  updateURL,
	}
}

func (s *BenchmarkStore) Update() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.url == "" {
		s.loadDefaults()
		return nil
	}

	resp, err := http.Get(s.url)
	if err != nil {
		s.loadDefaults()
		return fmt.Errorf("failed to fetch benchmarks: %w", err)
	}
	defer resp.Body.Close()

	var benchmarks []ModelBenchmark
	if err := json.NewDecoder(resp.Body).Decode(&benchmarks); err != nil {
		s.loadDefaults()
		return fmt.Errorf("failed to decode benchmarks: %w", err)
	}

	for _, b := range benchmarks {
		s.data[b.ID] = b
	}
	return nil
}

func (s *BenchmarkStore) loadDefaults() {
	// IDs must be real `ollama pull` names (verified via registry.ollama.ai).
	// MLX tags exist on Ollama only for gemma4; other families use Metal/GGUF tags.
	defaults := []ModelBenchmark{
		{ID: "gemma4:e2b-mlx", CodingScore: 68, ReasoningScore: 70, GeneralScore: 76, CreativeScore: 74, Params: 2, VRAMReq: 8, MLX: true},
		{ID: "gemma4:e4b-mlx", CodingScore: 74, ReasoningScore: 76, GeneralScore: 84, CreativeScore: 82, Params: 4, VRAMReq: 10, MLX: true},
		{ID: "gemma4:12b-mlx", CodingScore: 82, ReasoningScore: 84, GeneralScore: 88, CreativeScore: 86, Params: 12, VRAMReq: 8, MLX: true},
		{ID: "gemma4:26b-mlx", CodingScore: 88, ReasoningScore: 88, GeneralScore: 90, CreativeScore: 86, Params: 26, VRAMReq: 18, MLX: true},
		{ID: "gemma4:31b-mlx", CodingScore: 90, ReasoningScore: 90, GeneralScore: 92, CreativeScore: 88, Params: 31, VRAMReq: 19, MLX: true},
		{ID: "qwen2.5-coder:7b", CodingScore: 86, ReasoningScore: 68, GeneralScore: 74, CreativeScore: 64, Params: 7, VRAMReq: 5},
		{ID: "qwen2.5-coder:14b", CodingScore: 91, ReasoningScore: 78, GeneralScore: 80, CreativeScore: 70, Params: 14, VRAMReq: 9},
		{ID: "qwen2.5-coder:32b", CodingScore: 94, ReasoningScore: 84, GeneralScore: 82, CreativeScore: 72, Params: 32, VRAMReq: 20},
		{ID: "qwen2.5:14b", CodingScore: 80, ReasoningScore: 82, GeneralScore: 86, CreativeScore: 85, Params: 14, VRAMReq: 9},
		{ID: "gemma2:9b", CodingScore: 74, ReasoningScore: 76, GeneralScore: 84, CreativeScore: 86, Params: 9, VRAMReq: 6},
		{ID: "gemma2:2b", CodingScore: 55, ReasoningScore: 58, GeneralScore: 66, CreativeScore: 68, Params: 2, VRAMReq: 2},
		{ID: "llama3.1:8b", CodingScore: 72, ReasoningScore: 76, GeneralScore: 84, CreativeScore: 80, Params: 8, VRAMReq: 5},
		{ID: "llama3.2:3b", CodingScore: 62, ReasoningScore: 64, GeneralScore: 72, CreativeScore: 70, Params: 3, VRAMReq: 2},
		{ID: "llama3.1:70b", CodingScore: 90, ReasoningScore: 93, GeneralScore: 94, CreativeScore: 88, Params: 70, VRAMReq: 40},
	}
	for _, b := range defaults {
		s.data[b.ID] = b
	}
}

type Hardware struct {
	VRAMGB    int    `json:"vram"`
	RAMGB     int    `json:"ram"`
	PreferMLX bool   `json:"prefer_mlx"`
	Unified   bool   `json:"unified"`
	Arch      string `json:"arch"`
	Chip      string `json:"chip"`
	Source    string `json:"source,omitempty"` // sysctl|preset|request|container
}

type CategoryRecommendation struct {
	Category    string  `json:"category"`
	Complexity  string  `json:"complexity,omitempty"`
	ModelID     string  `json:"model_id"`
	DisplayName string  `json:"display_name"`
	Params      int     `json:"params"`
	MemReqGB    int     `json:"mem_req_gb"`
	TierBudget  int     `json:"tier_budget_gb,omitempty"`
	Score       float64 `json:"score"`
	MLX         bool    `json:"mlx"`
	InInventory bool    `json:"in_inventory"`
	Requests    int64   `json:"requests,omitempty"`
	SharePct    float64 `json:"share_pct,omitempty"`
	Gap         bool    `json:"gap,omitempty"`
	GapReason   string  `json:"gap_reason,omitempty"`
	Reason      string  `json:"reason"`
}

type ModelPick struct {
	ModelID          string            `json:"model_id"`
	DisplayName      string            `json:"display_name"`
	Params           int               `json:"params"`
	MemReqGB         int               `json:"mem_req_gb"`
	TierBudget       int               `json:"tier_budget_gb"`
	Score            float64           `json:"score"`
	MLX              bool              `json:"mlx"`
	InInventory      bool              `json:"in_inventory"`
	OllamaURL        string            `json:"ollama_url"`
	OllamaPull       string            `json:"ollama_pull"`
	BenchmarkURL     string            `json:"benchmark_url"`
	Criteria         []string          `json:"criteria"`
	Characteristics  map[string]string `json:"characteristics"`
	Gap              bool              `json:"gap,omitempty"`
	GapReason        string            `json:"gap_reason,omitempty"`
}

type PrioritySection struct {
	Priority   int         `json:"priority"`
	Category   string      `json:"category"`
	Complexity string      `json:"complexity"`
	Title      string      `json:"title"`
	Requests   int64       `json:"requests"`
	SharePct   float64     `json:"share_pct"`
	UpdatedAt  string      `json:"updated_at"`
	TierBudget int         `json:"tier_budget_gb"`
	Models     []ModelPick `json:"models"`
	Gap        bool        `json:"gap"`
}

type RecommendResponse struct {
	Hardware     Hardware                 `json:"hardware"`
	BudgetGB     int                      `json:"budget_gb"`
	LoadTotal    int64                    `json:"load_total"`
	LoadProfile  []store.LoadCell         `json:"load_profile,omitempty"`
	UpdatedAt    string                   `json:"updated_at"`
	ByCategory   []CategoryRecommendation `json:"by_category"`
	ByComplexity []CategoryRecommendation `json:"by_complexity"`
	Priority     []PrioritySection        `json:"priority_sections"`
	Gaps         []CategoryRecommendation `json:"gaps,omitempty"`
	Candidates   []CategoryRecommendation `json:"candidates,omitempty"`
	Notes        []string                 `json:"notes,omitempty"`
}

// PresetMacM536 is the user's primary workstation profile.
var PresetMacM536 = Hardware{
	RAMGB:     36,
	VRAMGB:    36,
	PreferMLX: true,
	Unified:   true,
	Arch:      "apple_silicon",
	Chip:      "Apple M5",
	Source:    "preset",
}

func Profiles() []Hardware {
	return []Hardware{PresetMacM536}
}

var paramsRe = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*b\b`)

func estimateParams(name string) int {
	m := paramsRe.FindStringSubmatch(name)
	if len(m) < 2 {
		return 0
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	return int(f + 0.5)
}

func isMLXName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "mlx") || strings.Contains(n, "-metal")
}

func memReqGB(params int, mlx bool, catalogVRAM int) int {
	if catalogVRAM > 0 {
		return catalogVRAM
	}
	if params <= 0 {
		return 8
	}
	// Rough working-set: MLX quantized ≈ 0.7GB/B, dense ≈ 1.1GB/B
	per := 1.1
	if mlx {
		per = 0.7
	}
	req := int(float64(params)*per + 0.5)
	if req < 2 {
		req = 2
	}
	return req
}

func scoreFor(b ModelBenchmark, category string) float64 {
	switch category {
	case "coding":
		return b.CodingScore
	case "reasoning":
		return b.ReasoningScore
	case "creative":
		if b.CreativeScore > 0 {
			return b.CreativeScore
		}
		return b.GeneralScore
	default: // chat
		return b.GeneralScore
	}
}

func (s *BenchmarkStore) lookup(name string) (ModelBenchmark, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if b, ok := s.data[name]; ok {
		return b, true
	}
	// fuzzy: catalog id contained in upstream name or vice versa
	lower := strings.ToLower(name)
	var best ModelBenchmark
	found := false
	for id, b := range s.data {
		if strings.Contains(lower, strings.ToLower(id)) || strings.Contains(strings.ToLower(id), lower) {
			if !found || b.Params > best.Params {
				best = b
				found = true
			}
		}
	}
	return best, found
}

type candidate struct {
	CategoryRecommendation
	rank float64
}

type seed struct {
	id    string
	name  string
	inInv bool
}

func normalizeHW(hw Hardware) (Hardware, int) {
	if hw.RAMGB <= 0 && hw.VRAMGB > 0 {
		hw.RAMGB = hw.VRAMGB
	}
	if hw.VRAMGB <= 0 && hw.RAMGB > 0 {
		hw.VRAMGB = hw.RAMGB
	}
	if hw.Unified || hw.PreferMLX || hw.Arch == "apple_silicon" {
		hw.Unified = true
		if hw.RAMGB > 0 {
			hw.VRAMGB = hw.RAMGB
		}
	}
	budget := hw.VRAMGB
	if hw.Unified && hw.RAMGB > 0 {
		budget = int(float64(hw.RAMGB) * 0.72)
	}
	if budget <= 0 {
		budget = 12
	}
	return hw, budget
}

// complexityBudget caps model memory by task difficulty so simple traffic
// does not pull a 30B model into RAM.
func complexityBudget(totalBudget int, complexity string) int {
	switch strings.ToLower(complexity) {
	case "trivial":
		return clampInt(totalBudget/5, 3, totalBudget)
	case "simple":
		return clampInt(totalBudget*2/5, 6, totalBudget)
	case "medium":
		return clampInt(totalBudget*3/4, 12, totalBudget)
	default: // hard
		return totalBudget
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (s *BenchmarkStore) collectSeeds(inventory []provider.ModelInfo) map[string]seed {
	seeds := map[string]seed{}
	for _, m := range inventory {
		if !m.Routable || m.CostTier == "metered" {
			continue
		}
		seeds[m.UpstreamID] = seed{id: m.ID(), name: m.UpstreamID, inInv: true}
	}
	s.mu.RLock()
	for id := range s.data {
		if _, ok := seeds[id]; !ok {
			seeds[id] = seed{id: "catalog/" + id, name: id, inInv: false}
		}
	}
	s.mu.RUnlock()
	return seeds
}

func (s *BenchmarkStore) rankModel(se seed, cat, complexity string, hw Hardware, tierBudget int) (candidate, bool) {
	b, ok := s.lookup(se.name)
	params := b.Params
	if params <= 0 {
		params = estimateParams(se.name)
	}
	mlx := b.MLX || isMLXName(se.name)
	req := memReqGB(params, mlx, b.VRAMReq)
	if req > tierBudget {
		return candidate{}, false
	}
	sc := 50.0
	if ok {
		sc = scoreFor(b, cat)
	} else {
		n := strings.ToLower(se.name)
		if cat == "coding" && (strings.Contains(n, "coder") || strings.Contains(n, "code")) {
			sc = 80
		}
		if cat == "reasoning" && (strings.Contains(n, "reason") || strings.Contains(n, "r1")) {
			sc = 80
		}
	}
	rank := sc
	if hw.PreferMLX && mlx {
		rank += 12
	}
	if se.inInv {
		rank += 8
	}
	nlow := strings.ToLower(se.name)
	if cat == "coding" && (strings.Contains(nlow, "coder") || strings.Contains(nlow, "code")) {
		rank += 18
	}
	if cat == "reasoning" && (strings.Contains(nlow, "reason") || strings.Contains(nlow, "r1") || strings.Contains(nlow, "qwq")) {
		rank += 14
	}
	if (cat == "chat" || cat == "creative") && (strings.Contains(nlow, "gemma") || strings.Contains(nlow, "instruct")) {
		rank += 6
	}
	// Prefer filling the tier without overshooting — keep simple tasks on small models.
	rank += float64(req) * 0.15
	slack := tierBudget - req
	if slack > 8 {
		rank -= float64(slack-8) * 0.4
	}

	return candidate{
		CategoryRecommendation: CategoryRecommendation{
			Category:    cat,
			Complexity:  complexity,
			ModelID:     se.id,
			DisplayName: CanonicalOllamaName(se.name),
			Params:      params,
			MemReqGB:    req,
			TierBudget:  tierBudget,
			Score:       sc,
			MLX:         mlx || strings.Contains(strings.ToLower(CanonicalOllamaName(se.name)), "mlx"),
			InInventory: se.inInv,
			Reason:      fmt.Sprintf("score=%.0f, mem≈%dGB ≤ tier %dGB", sc, req, tierBudget),
		},
		rank: rank,
	}, true
}

func (s *BenchmarkStore) bestFor(cat, complexity string, hw Hardware, budget int, seeds map[string]seed) (CategoryRecommendation, bool) {
	tier := complexityBudget(budget, complexity)
	var best candidate
	found := false
	for _, se := range seeds {
		cand, ok := s.rankModel(se, cat, complexity, hw, tier)
		if !ok {
			continue
		}
		if !found || cand.rank > best.rank {
			best = cand
			found = true
		}
	}
	if !found {
		return CategoryRecommendation{
			Category:   cat,
			Complexity: complexity,
			TierBudget: tier,
			Gap:        true,
			GapReason:  fmt.Sprintf("нет модели в бюджете сложности %s (≤%dGB)", complexity, tier),
			Reason:     fmt.Sprintf("нет модели в бюджете сложности %s (≤%dGB)", complexity, tier),
		}, false
	}
	if best.MLX && hw.PreferMLX {
		best.Reason += "; MLX"
	}
	if best.InInventory {
		best.Reason += "; inventory"
	} else {
		best.Gap = true
		best.GapReason = "рекомендуемая модель не в inventory — стоит скачать"
	}
	return best.CategoryRecommendation, true
}

func (s *BenchmarkStore) RecommendForCategories(hw Hardware, inventory []provider.ModelInfo) RecommendResponse {
	return s.RecommendWithLoad(hw, inventory, nil, 0)
}

func categoryTitleRU(cat string) string {
	switch cat {
	case "coding":
		return "Кодинг"
	case "reasoning":
		return "Reasoning"
	case "creative":
		return "Creative"
	default:
		return "Чат"
	}
}

// hfSearchQuery turns Ollama-style ids (gemma2:9b-mlx) into HF-friendly tokens
// (gemma-2 9b mlx). Raw names with ":" return zero HF hits.
func hfSearchQuery(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	base, tag, _ := strings.Cut(name, ":")
	lowName := strings.ToLower(name)
	mlx := strings.Contains(lowName, "mlx")

	base = strings.ToLower(strings.TrimSpace(base))
	base = strings.TrimSuffix(base, "-mlx")
	base = expandHFFamily(base)

	tag = strings.ToLower(strings.TrimSpace(tag))
	tag = strings.ReplaceAll(tag, "-mlx", "")
	tag = strings.Trim(tag, "-_")
	tag = strings.ReplaceAll(tag, "-", " ")
	tag = strings.ReplaceAll(tag, "_", " ")

	parts := make([]string, 0, 4)
	if base != "" {
		parts = append(parts, base)
	}
	if tag != "" {
		parts = append(parts, tag)
	}
	if mlx {
		parts = append(parts, "mlx")
	}
	return strings.Join(parts, " ")
}

func expandHFFamily(base string) string {
	// HF repos use gemma-2 / llama-3.x; Ollama uses gemma2 / llama3.1.
	// Leave qwen2.5* as-is (HF keeps the dotted form).
	type pair struct{ from, to string }
	for _, p := range []pair{
		{"gemma4", "gemma-4"},
		{"gemma3", "gemma-3"},
		{"gemma2", "gemma-2"},
		{"llama3.3", "llama-3.3"},
		{"llama3.2", "llama-3.2"},
		{"llama3.1", "llama-3.1"},
		{"llama3", "llama-3"},
		{"phi4", "phi-4"},
		{"phi3", "phi-3"},
		{"mistral", "mistral"},
	} {
		if strings.HasPrefix(base, p.from) {
			return p.to + base[len(p.from):]
		}
	}
	return base
}

func benchmarkSearchURL(name string) string {
	q := hfSearchQuery(name)
	if q == "" {
		q = name
	}
	return "https://huggingface.co/models?search=" + url.QueryEscape(q)
}

func (s *BenchmarkStore) rankedPicks(cat, complexity string, hw Hardware, budget int, seeds map[string]seed, topN int) []ModelPick {
	if topN <= 0 {
		topN = 5
	}
	tier := complexityBudget(budget, complexity)
	var ranked []candidate
	for _, se := range seeds {
		cand, ok := s.rankModel(se, cat, complexity, hw, tier)
		if !ok {
			continue
		}
		ranked = append(ranked, cand)
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].rank > ranked[j].rank })
	out := make([]ModelPick, 0, topN)
	for i, c := range ranked {
		if i >= topN {
			break
		}
		criteria := []string{
			fmt.Sprintf("Категория «%s»: score качества %.0f", cat, c.Score),
			fmt.Sprintf("Сложность %s: лимит памяти tier ≤ %d GB, модель ≈ %d GB", complexity, tier, c.MemReqGB),
		}
		if hw.PreferMLX {
			if c.MLX {
				criteria = append(criteria, "MLX / Apple Silicon — совпадает с предпочтением")
			} else {
				criteria = append(criteria, "Не MLX — ниже приоритет при prefer_mlx")
			}
		}
		if c.InInventory {
			criteria = append(criteria, "Уже есть в локальном inventory")
		} else {
			criteria = append(criteria, "Нет в inventory — рекомендуется установить через Ollama")
		}
		nlow := strings.ToLower(c.DisplayName)
		if cat == "coding" && (strings.Contains(nlow, "coder") || strings.Contains(nlow, "code")) {
			criteria = append(criteria, "Имя модели указывает на coder/code specialization")
		}
		pullName := CanonicalOllamaName(c.DisplayName)
		exists := OllamaModelExists(pullName)
		pullCmd := ""
		if exists {
			pullCmd = ollamaPullCommand(pullName)
		} else {
			criteria = append(criteria, "Тег не найден в Ollama library — открывай страницу модели на сайте")
		}
		pick := ModelPick{
			ModelID:      c.ModelID,
			DisplayName:  pullName,
			Params:       c.Params,
			MemReqGB:     c.MemReqGB,
			TierBudget:   tier,
			Score:        c.Score,
			MLX:          c.MLX || strings.Contains(strings.ToLower(pullName), "mlx"),
			InInventory:  c.InInventory,
			OllamaURL:    ollamaLibraryURL(pullName),
			OllamaPull:   pullCmd,
			BenchmarkURL: benchmarkSearchURL(pullName),
			Criteria:     criteria,
			Characteristics: map[string]string{
				"params_b":       fmt.Sprintf("%d", c.Params),
				"mem_est_gb":     fmt.Sprintf("%d", c.MemReqGB),
				"tier_budget":    fmt.Sprintf("%d GB", tier),
				"mlx":            fmt.Sprintf("%v", c.MLX || strings.Contains(strings.ToLower(pullName), "mlx")),
				"in_inventory":   fmt.Sprintf("%v", c.InInventory),
				"ollama_exists":  fmt.Sprintf("%v", exists),
				"ollama_library": ollamaLibraryURL(pullName),
				"category":       cat,
				"complexity":     complexity,
				"quality":        fmt.Sprintf("%.0f", c.Score),
			},
		}
		if !c.InInventory {
			pick.Gap = true
			if !exists {
				pick.GapReason = "нет в Ollama library"
			} else {
				pick.GapReason = "нет в inventory"
			}
		}
		out = append(out, pick)
	}
	return out
}

func (s *BenchmarkStore) RecommendWithLoad(hw Hardware, inventory []provider.ModelInfo, load []store.LoadCell, loadTotal int64) RecommendResponse {
	hw, budget := normalizeHW(hw)
	seeds := s.collectSeeds(inventory)
	updatedAt := time.Now().UTC().Format(time.RFC3339)

	categories := []string{"coding", "reasoning", "chat", "creative"}
	complexities := []string{"trivial", "simple", "medium", "hard"}
	var notes []string
	if hw.PreferMLX {
		notes = append(notes, "Приоритет MLX под Apple Silicon / unified memory.")
	}
	notes = append(notes, fmt.Sprintf("Общий бюджет ~%d GB; для simple/trivial — урезанный tier.", budget))

	byCat := make([]CategoryRecommendation, 0, len(categories))
	for _, cat := range categories {
		rec, _ := s.bestFor(cat, "medium", hw, budget, seeds)
		rec.Complexity = ""
		byCat = append(byCat, rec)
	}

	loadIndex := map[string]store.LoadCell{}
	for _, c := range load {
		loadIndex[c.Category+"|"+c.Complexity] = c
	}

	type key struct{ cat, cx string }
	want := []key{}
	if len(load) > 0 {
		for _, c := range load {
			want = append(want, key{c.Category, c.Complexity})
		}
	} else {
		for _, cat := range categories {
			for _, cx := range complexities {
				want = append(want, key{cat, cx})
			}
		}
	}

	byCx := make([]CategoryRecommendation, 0, len(want))
	gaps := make([]CategoryRecommendation, 0)
	sections := make([]PrioritySection, 0, len(want))
	allFits := []candidate{}

	seenCell := map[string]bool{}
	for _, k := range want {
		sk := k.cat + "|" + k.cx
		if seenCell[sk] {
			continue
		}
		seenCell[sk] = true
		rec, ok := s.bestFor(k.cat, k.cx, hw, budget, seeds)
		reqs, share := int64(0), 0.0
		if lc, has := loadIndex[sk]; has {
			rec.Requests = lc.Requests
			rec.SharePct = lc.SharePct
			reqs, share = lc.Requests, lc.SharePct
		}
		if !ok || rec.Gap {
			rec.Gap = true
			if rec.GapReason == "" {
				rec.GapReason = rec.Reason
			}
			gaps = append(gaps, rec)
		}
		byCx = append(byCx, rec)

		picks := s.rankedPicks(k.cat, k.cx, hw, budget, seeds, 6)
		secGap := len(picks) == 0 || (len(picks) > 0 && !picks[0].InInventory)
		sections = append(sections, PrioritySection{
			Category:   k.cat,
			Complexity: k.cx,
			Title:      fmt.Sprintf("%s — %s", categoryTitleRU(k.cat), k.cx),
			Requests:   reqs,
			SharePct:   share,
			UpdatedAt:  updatedAt,
			TierBudget: complexityBudget(budget, k.cx),
			Models:     picks,
			Gap:        secGap,
		})

		tier := complexityBudget(budget, k.cx)
		for _, se := range seeds {
			if cand, ok := s.rankModel(se, k.cat, k.cx, hw, tier); ok {
				allFits = append(allFits, cand)
			}
		}
	}

	sort.Slice(byCx, func(i, j int) bool {
		if byCx[i].Requests != byCx[j].Requests {
			return byCx[i].Requests > byCx[j].Requests
		}
		return byCx[i].Category < byCx[j].Category
	})
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].Requests > gaps[j].Requests })
	// Priority: more requests first; gaps (missing inventory) boosted
	sort.Slice(sections, func(i, j int) bool {
		si, sj := sections[i], sections[j]
		scorei, scorej := si.Requests, sj.Requests
		if si.Gap {
			scorei = scorei*2 + 1
		}
		if sj.Gap {
			scorej = scorej*2 + 1
		}
		if scorei != scorej {
			return scorei > scorej
		}
		return si.Title < sj.Title
	})
	for i := range sections {
		sections[i].Priority = i + 1
	}

	sort.Slice(allFits, func(i, j int) bool { return allFits[i].rank > allFits[j].rank })
	uniq := []CategoryRecommendation{}
	seen := map[string]bool{}
	for _, c := range allFits {
		if seen[c.ModelID] {
			continue
		}
		seen[c.ModelID] = true
		uniq = append(uniq, c.CategoryRecommendation)
		if len(uniq) >= 10 {
			break
		}
	}

	if loadTotal > 0 {
		notes = append(notes, fmt.Sprintf("Профиль нагрузки: %d запросов; секции отсортированы по приоритету (трафик × пробел в inventory).", loadTotal))
	}

	return RecommendResponse{
		Hardware:     hw,
		BudgetGB:     budget,
		LoadTotal:    loadTotal,
		LoadProfile:  load,
		UpdatedAt:    updatedAt,
		ByCategory:   byCat,
		ByComplexity: byCx,
		Priority:     sections,
		Gaps:         gaps,
		Candidates:   uniq,
		Notes:        notes,
	}
}

// Recommend keeps a simple list for backward compatibility.
func (s *BenchmarkStore) Recommend(hw Hardware) []provider.ModelInfo {
	res := s.RecommendForCategories(hw, nil)
	out := make([]provider.ModelInfo, 0, len(res.Candidates))
	for _, c := range res.Candidates {
		out = append(out, provider.ModelInfo{
			UpstreamID:  c.DisplayName,
			DisplayName: c.DisplayName,
			Routable:    true,
			Params:      c.Params,
			CostTier:    "local",
		})
	}
	return out
}
