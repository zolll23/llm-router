FROM python:3.12-slim-bookworm

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends curl \
  && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .

# CPU torch first — default Linux wheels pull multi‑GB CUDA stacks and fill Colima disk.
RUN pip install --no-cache-dir --upgrade pip \
  && pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cpu \
  && pip install --no-cache-dir -r requirements.txt

COPY app.py .

ENV PORT=8090
ENV GLINER_MODEL=fastino/GLiNER2.5-Decide
ENV HF_HOME=/cache/huggingface
ENV TOKENIZERS_PARALLELISM=false
EXPOSE 8090

HEALTHCHECK --interval=15s --timeout=5s --start-period=180s --retries=30 \
  CMD curl -fsS "http://127.0.0.1:${PORT}/health" || exit 1

CMD ["sh", "-c", "uvicorn app:app --host 0.0.0.0 --port ${PORT}"]
