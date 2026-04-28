# g8el (g8el) - llama.cpp inference server

FROM ghcr.io/ggml-org/llama.cpp:server

LABEL io.g8e.managed=true

RUN mkdir -p /models

COPY --chmod=755 components/g8el/entrypoint.sh /app/entrypoint.sh

EXPOSE 11444

ENTRYPOINT ["/app/entrypoint.sh"]
