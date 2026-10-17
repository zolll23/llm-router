# Quota-aware escalation + страница «Модели»

Дата: 2026-10-17  
Статус: draft → на ревью

## Цель

Сейчас почти все запросы уходят на локальные модели (локальный бонус в скорере сильнее внешних). Нужно:

1. Сложные запросы приоритетно слать на внешние модели, **если есть запас по лимитам**.
2. Показывать все доступные модели и лимиты на отдельной странице **«Модели»**.
3. Текущую вкладку «Модели» (подбор под железо) переименовать в **«Рекомендации»**.
4. Дожать динамический скор: живые замеры задержки/скорости из журнала реально влияют на выбор.

## Решения (зафиксировано с пользователем)

| Тема | Выбор |
|------|--------|
| Внешние провайдеры | Любой `openai_compatible` + Ollama (в т.ч. cloud); смешанные лимиты |
| Порог эскалации | Настройка в UI, default `hard` |
| Headroom | Отдельные пороги used% для rolling / weekly / monthly (default 85 / 90 / 95) |
| Объём v1 | UI + роутинг + cooldown после 429 → лучшая локальная (без цепочки из нескольких внешних) |
| Архитектура скора | Quota-aware scorer + общее хранилище лимитов для UI и Decide |
| Замеры | Сразу: заполнять Metrics из usage, не оставлять заготовку пустой |

## Секция 1 — Роутинг

1. Классификатор как сейчас → `complexity` ∈ {trivial, simple, medium, hard}.
2. Если `complexity < escalate_min` → предпочитать локальные (текущее local-first).
3. Если `complexity ≥ escalate_min` **и** у внешнего провайдера есть запас (headroom) → бонус внешним в скорере (не жёсткий filter: tools/липкая сессия учитываются).
4. Нет запаса / нет внешних → локальные.
5. Upstream **429** (или явная ошибка квоты): cooldown на провайдера (`quota_cooldown_seconds`), **один** повтор на лучшую локальную; в usage/sankey статус «лимит».
6. **Лучшая локальная** = `pickBest` по пулу `CostTier=local` (routable, tools если нужны) с учётом живых Metrics и Params.
7. Sticky: можно сменить pin, если политика эскалации/cooldown требует другого пула (иначе hard-сессии залипнут на local).

### Динамический скор (дописать сразу)

Уже есть крючок в `scoreModel`: `Metrics["tps"]`. Сейчас inventory его не заполняет.

В v1:

- После успешного ответа обновлять по `model_id` скользящие метрики из `usage_events` (как минимум p50 latency_ms; при наличии токенов/времени — оценка tps).
- Перед Decide подмешивать метрики в `ModelInfo.Metrics` (или держать отдельный PerformanceStore и читать из скорера).
- По возможности парсить `Params` из имени модели при sync inventory (уже есть эвристика в benchmark).
- Capability-метки — по возможности из имени/каталога; отсутствие не блокирует.

## Секция 2 — Лимиты

### Хранилище

Общий снимок для роутера и UI:

- по провайдеру: окна (rolling / weekly / monthly), percent **used**, `resets_at`, cooldown_until;
- опрос ~раз в 60с (не чаще; Ollama balance — 10 req/min).

### Интерфейс расширения (модульность)

Провайдеры уже модульные: `Provider` = ListModels + ChatCompletions, фабрика `NewFromConfig`.

Для квот — **опциональный** разъём (не ломает провайдеры без лимитов):

```go
type QuotaProvider interface {
    Provider
    FetchQuota(ctx context.Context) (QuotaSnapshot, error)
}
```

Новый OpenAI/Claude через совместимый HTTP — обычно `openai_compatible` без кода. Отдельный тип — только если API иное. Квоты подключаются отдельной реализацией `FetchQuota` или адаптером `quota: opencode_go | ollama_balance | none` в конфиге.

### Источники

| Провайдер | Endpoint | Поля |
|-----------|----------|------|
| OpenCode Go | `GET https://opencode.ai/zen/go/v1/usage` Bearer key | `usage.rolling/weekly/monthly` — percent used, resetsAt (как в консоли Go) |
| Ollama Cloud | `GET https://ollama.com/api/balance` | included.session/weekly.remaining_percent → перевести в used%; purchased balance |
| Прочий openai_compatible | нет API | headroom=true пока нет cooldown после 429; позже ручные RPM |

### Headroom

Для каждого окна с данными: `used_percent < threshold` (настройки). Провайдер «ок», если **все** имеющиеся окна проходят порог и нет активного cooldown.

Правила по наличию адаптера квот:

1. **`quota: opencode_go | ollama_balance`** и последний FetchQuota успешен → headroom по порогам окон.
2. **Тот же адаптер**, но fetch упал / нет ключа → эскалация на этот провайдер **выключена** (безопасный default).
3. **`quota: none`** или адаптер не задан → считаем headroom=true (лимитов API нет), стоп только по cooldown после 429.

Локальный пул всегда доступен как запасной путь.

### Ключи

- `api_key_env` в yaml провайдера;
- опционально чтение Go key из `~/.local/share/opencode/auth.json` (`opencode-go`) как fallback.

## Секция 3 — UI

### Маршруты

| Вкладка | Path | Содержание |
|---------|------|------------|
| Рекомендации | `/recommendations` | Бывшая «Модели»: железо, диаграммы нагрузки, приоритетные таблицы |
| Модели | `/models` | Inventory + бенчмарк-ссылки + карточки лимитов |
| Настройки | `/settings` | + escalate_min, пороги квот, cooldown |
| Usage / Качество | без смены смысла | |

Редирект: старый смысл `/models` → `/recommendations` (или 301 с `/recommendations`); новый `/models` = inventory.  
Алиас: `/recommendations` уже есть; обновить nav и `PATH_TAB` в `app.js`.

### Страница «Модели»

- Таблица: провайдер, имя, tier (local/metered), routable, tools, params, latency/tps если есть, ссылка бенчмарк, ссылка ollama.com при local.
- Карточки квот по провайдерам (rolling / week / month + reset).
- Кнопка обновить: sync inventory + refresh quotas.
- Сырой JSON inventory — опционально свёрнут в Настройках.

### Настройки (новые поля)

- `escalate_min_complexity` — default `hard`
- `quota_threshold_rolling_pct` — 85
- `quota_threshold_weekly_pct` — 90
- `quota_threshold_monthly_pct` — 95
- `quota_cooldown_seconds` — например 600

## Секция 4 — API и конфиг

### Runtime settings

Расширить `RuntimeSettings` / `GET|PUT /api/settings` полями выше.

### Новые endpoints

- `GET /api/models` — обогащённый список для UI Модели
- `GET /api/quotas` — снимок лимитов (+ `fetched_at`)
- `POST /api/inventory/sync` — как сейчас + триггер опроса квот

### Chat path

`POST /v1/chat/completions`: Decide с эскалацией/headroom/метриками; на 429 — cooldown + один retry на лучшую локальную; usage как сейчас.

### Конфиг провайдера (пример)

```yaml
providers:
  - id: ollama
    type: ollama
    base_url: "http://host.docker.internal:11434"
  - id: opencode-go
    type: openai_compatible
    base_url: "https://opencode.ai/zen/go/v1"
    api_key_env: OPENCODE_API_KEY
    quota: opencode_go
```

## Вне scope v1

- Цепочка failover Go → другой cloud → local
- Ручные RPM-лимиты в UI (кроме cooldown)
- Публичный API остатка free Zen без Go
- Полная матрица capabilities из внешнего каталога

## Критерии готовности

1. При `escalate_min=hard` и headroom Go OK — hard уходит на внешнюю (если есть routable metered); simple остаётся local.
2. При used% выше порога — hard снова на local.
3. 429 → cooldown, следующий запрос не бьёт тот же провайдер до истечения; один retry local в том же запросе.
4. `/models` показывает inventory + квоты Go/Ollama; `/recommendations` — бывший UI рекомендаций.
5. Скор учитывает непустые Metrics из usage после нескольких запросов.
6. Тесты: canonicalize headroom, escalate gate, cooldown retry; UI routes.

## Риски

- Go `/usage` или balance недоступны из Docker без ключа/сети → эскалация тихо выключена (ожидаемо).
- Percent в Go — account-wide (все устройства), как в консоли.
- Sticky vs эскалация: нужна явная смена pin (см. §1).
