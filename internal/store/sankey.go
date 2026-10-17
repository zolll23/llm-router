package store

import (
	"sort"
	"time"
)

type SankeyNode struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Stage string `json:"stage"`
}

type SankeyLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Value  int64  `json:"value"`
}

type SankeyDiagram struct {
	Nodes []SankeyNode `json:"nodes"`
	Links []SankeyLink `json:"links"`
	Days  int          `json:"days"`
	Total int64        `json:"total"`
}

func (d *DB) SankeySince(days int) (SankeyDiagram, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
	rows, err := d.SQL.Query(`
SELECT
  COALESCE(NULLIF(TRIM(category), ''), 'chat') AS category,
  COALESCE(NULLIF(TRIM(complexity), ''), 'simple') AS complexity,
  COALESCE(NULLIF(TRIM(pool), ''),
    CASE
      WHEN provider_id IS NULL OR provider_id = '' THEN 'unknown · unknown'
      ELSE provider_id || ' · unknown'
    END
  ) AS pool,
  COALESCE(NULLIF(TRIM(model_id), ''), 'unknown') AS model_id,
  COALESCE(http_status, 0) AS http_status,
  COUNT(*) AS n
FROM usage_events
WHERE ts >= ?
GROUP BY 1, 2, 3, 4, 5
`, since)
	if err != nil {
		return SankeyDiagram{}, err
	}
	defer rows.Close()

	linkMap := map[string]int64{}
	nodeMeta := map[string]SankeyNode{}
	var total int64

	addNode := func(id, name, stage string) {
		if _, ok := nodeMeta[id]; !ok {
			nodeMeta[id] = SankeyNode{ID: id, Name: name, Stage: stage}
		}
	}
	addLink := func(srcID, srcName, srcStage, dstID, dstName, dstStage string, n int64) {
		addNode(srcID, srcName, srcStage)
		addNode(dstID, dstName, dstStage)
		key := srcID + "\x00" + dstID
		linkMap[key] += n
	}

	for rows.Next() {
		var cat, cx, pool, model string
		var statusCode int
		var n int64
		if err := rows.Scan(&cat, &cx, &pool, &model, &statusCode, &n); err != nil {
			return SankeyDiagram{}, err
		}
		status := StatusLabel(statusCode)
		total += n
		// Scope complexity/pool by category so chat vs coding never merge mid-diagram.
		catID := "cat:" + cat
		cxID := "cx:" + cat + ":" + cx
		cxName := cat + " · " + cx
		poolID := "pool:" + cat + ":" + pool
		poolName := cat + " · " + pool
		modelID := "model:" + model
		stID := "status:" + status
		addLink(catID, cat, "category", cxID, cxName, "complexity", n)
		addLink(cxID, cxName, "complexity", poolID, poolName, "pool", n)
		addLink(poolID, poolName, "pool", modelID, model, "model", n)
		addLink(modelID, model, "model", stID, status, "status", n)
	}
	if err := rows.Err(); err != nil {
		return SankeyDiagram{}, err
	}

	stageOrder := map[string]int{
		"category": 0, "complexity": 1, "pool": 2, "model": 3, "status": 4,
	}
	nodes := make([]SankeyNode, 0, len(nodeMeta))
	for _, n := range nodeMeta {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		si, sj := stageOrder[nodes[i].Stage], stageOrder[nodes[j].Stage]
		if si != sj {
			return si < sj
		}
		return nodes[i].Name < nodes[j].Name
	})

	links := make([]SankeyLink, 0, len(linkMap))
	for key, v := range linkMap {
		parts := splitNull(key)
		if len(parts) != 2 {
			continue
		}
		links = append(links, SankeyLink{Source: parts[0], Target: parts[1], Value: v})
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].Source == links[j].Source {
			if links[i].Target == links[j].Target {
				return links[i].Value > links[j].Value
			}
			return links[i].Target < links[j].Target
		}
		return links[i].Source < links[j].Source
	})

	return SankeyDiagram{Nodes: nodes, Links: links, Days: days, Total: total}, nil
}

func splitNull(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return []string{s[:i], s[i+1:]}
		}
	}
	return nil
}
