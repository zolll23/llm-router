# Sankey Usage Flow Design

Date: 2026-10-17  
Status: approved for implementation

## Goal

Show a Sankey diagram on the Usage tab: Category → Complexity → Pool → Model → Status, width = request count (7 days default).

## Implementation notes

- Classify `category` via GLiNER/nimble/heuristic
- Persist `category` + `pool` on usage/routing events
- `GET /api/stats/sankey?days=7`
- D3 + d3-sankey CDN on Usage tab
