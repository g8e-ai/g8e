#!/bin/sh
set -e

MODEL_NAME=${G8EL_MODEL_NAME:-google_gemma-4-E2B-it-Q4_K_M.gguf}
MODEL_URL=${G8EL_MODEL_URL:-https://huggingface.co/bartowski/google_gemma-4-E2B-it-GGUF/resolve/main/google_gemma-4-E2B-it-Q4_K_M.gguf}
CONTEXT_SIZE=${G8EL_CONTEXT_SIZE:-8192}
THREADS=${G8EL_THREADS:-8}
HOST=${G8EL_HOST:-0.0.0.0}
PORT=${G8EL_PORT:-11444}

if [ ! -f "/models/${MODEL_NAME}" ]; then
    echo "Downloading model from Hugging Face..."
    curl -L -o "/models/${MODEL_NAME}" "${MODEL_URL}"
fi

exec /app/llama-server -m "/models/${MODEL_NAME}" -c "${CONTEXT_SIZE}" --host "${HOST}" --port "${PORT}" -t "${THREADS}" --mlock 