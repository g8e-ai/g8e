# Reproducibility wrapper for the g8e ensemble evals harness (g8e-powered).
#
# Build context is the repo root so the in-tree g8e protocol package is in scope:
#   docker build -f ensemble/evals/Dockerfile -t g8e-evals:local .
#
# The Gateway (g8eg) stays host-native. This image only carries the harness.
# At runtime the container needs trust material and an authenticated session:
#   docker run --rm \
#     --network=host \
#     -v "$PWD/.g8e/pki:/g8e/pki:ro" \
#     -e G8E_PKI_DIR=/g8e/pki \
#     -e G8E_OPERATOR_SESSION_ID="$(cat ~/.g8e/credentials/operator_session_id)" \
#     -e OPENAI_API_KEY \
#     g8e-evals:local \
#     run --suite ifeval --provider openai --model gpt-4o
FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1

WORKDIR /opt/g8e

COPY protocol/python /opt/g8e/protocol/python
COPY ensemble/evals /opt/g8e/ensemble/evals

RUN pip install /opt/g8e/protocol/python \
 && pip install /opt/g8e/ensemble/evals

WORKDIR /opt/g8e/ensemble/evals

ENTRYPOINT ["python", "-m", "g8e_evals"]
