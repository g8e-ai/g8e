FROM python:3.11-slim

WORKDIR /app

RUN pip install --no-cache-dir fastapi uvicorn docker

COPY main.py index.html ./

EXPOSE 8080

CMD ["python", "main.py"]
