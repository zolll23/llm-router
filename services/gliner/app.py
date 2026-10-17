"""Local GLiNER2.5 classification sidecar for the OpenAI router proxy."""

from __future__ import annotations

import os
from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

MODEL_ID = os.getenv("GLINER_MODEL", "fastino/GLiNER2.5-Decide")
PORT = int(os.getenv("PORT", "8090"))

app = FastAPI(title="GLiNER2.5 classifier", version="1.0.0")
_model: Any = None


class TaskSpec(BaseModel):
    labels: list[str] = Field(min_length=1)


class ClassifyRequest(BaseModel):
    text: str = Field(min_length=1)
    tasks: dict[str, TaskSpec]


class LabelResult(BaseModel):
    label: str
    confidence: float = 0.0
    probabilities: dict[str, float] = Field(default_factory=dict)


class ClassifyResponse(BaseModel):
    results: dict[str, LabelResult]


def _load_model() -> Any:
    from gliner2 import AutoExtractor

    return AutoExtractor.from_pretrained(MODEL_ID)


@app.on_event("startup")
def startup() -> None:
    global _model
    _model = _load_model()


@app.get("/health")
def health() -> dict[str, Any]:
    return {"ok": True, "model": MODEL_ID, "ready": _model is not None}


def _normalize_task(raw: Any, labels: list[str]) -> LabelResult:
    """Normalize gliner2 classify_text output into {label, confidence, probabilities}."""
    if raw is None:
        return LabelResult(label=labels[0], confidence=0.0)

    if isinstance(raw, str):
        return LabelResult(label=raw, confidence=0.7)

    if not isinstance(raw, dict):
        return LabelResult(label=str(raw), confidence=0.7)

    label = (
        raw.get("label")
        or raw.get("category")
        or raw.get("true_label")
        or ""
    )
    if isinstance(label, list) and label:
        label = label[0]
    label = str(label).lower() if label else ""

    probs: dict[str, float] = {}
    for key in ("probabilities", "scores", "probs"):
        if isinstance(raw.get(key), dict):
            probs = {str(k).lower(): float(v) for k, v in raw[key].items()}
            break

    conf = raw.get("confidence")
    if conf is None and label and label in probs:
        conf = probs[label]
    if conf is None and probs:
        conf = max(probs.values())
    if conf is None:
        conf = 0.7

    if not label and probs:
        label = max(probs.items(), key=lambda kv: kv[1])[0]
    if not label:
        label = labels[0]

    return LabelResult(label=label, confidence=float(conf), probabilities=probs)


@app.post("/v1/classify", response_model=ClassifyResponse)
def classify(req: ClassifyRequest) -> ClassifyResponse:
    if _model is None:
        raise HTTPException(status_code=503, detail="model not loaded")

    schema: dict[str, Any] = {}
    for name, spec in req.tasks.items():
        schema[name] = {"labels": list(spec.labels)}

    try:
        raw = _model.classify_text(req.text, schema)
    except Exception as exc:  # noqa: BLE001 — surface model errors to caller
        raise HTTPException(status_code=502, detail=str(exc)) from exc

    # gliner2 may return {task: ...} or wrap under classifications
    if isinstance(raw, dict) and "classifications" in raw and len(raw) == 1:
        raw = raw["classifications"]

    results: dict[str, LabelResult] = {}
    for name, spec in req.tasks.items():
        results[name] = _normalize_task(
            raw.get(name) if isinstance(raw, dict) else None,
            spec.labels,
        )
    return ClassifyResponse(results=results)
