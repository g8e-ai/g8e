# Fleet Demo (20-1000 nodes)

.PHONY: help build up down status clean logs

COMPOSE := docker compose

help:
	@echo "Fleet Demo (20-1000 nodes)"
	@echo "Usage: make [target]"
	@echo ""
	@echo "  up N=1000 [-d dlk_xxx]  - Start N identical nodes with operators"
	@echo "  down                    - Stop all nodes"
	@echo "  status                  - Show container status"
	@echo "  clean                   - Remove everything"
	@echo "  logs                    - Follow all container logs"

build:
	@echo "Building featherweight node image..."
	$(COMPOSE) build

up: build
	@set -e; \
	TOKEN="$(DEVICE_TOKEN)"; \
	if [ -z "$$TOKEN" ] && echo "$(filter-out $@,$(MAKECMDGOALS))" | grep -q '\-d'; then \
		TOKEN=$$(echo "$(filter-out $@,$(MAKECMDGOALS))" | sed 's/.*-d \([^ ]*\).*/\1/'); \
	fi; \
	NUM="${NODES}"; \
	if [ -z "$$NUM" ]; then \
		NUM="${N}"; \
	fi; \
	if [ -z "$$NUM" ]; then \
		NUM=20; \
	fi; \
	if [ -n "$$TOKEN" ]; then \
		echo "Starting $$NUM nodes (operators auto-attach via DEVICE_TOKEN)..."; \
		DEVICE_TOKEN="$$TOKEN" $(COMPOSE) up -d --scale node=$$NUM; \
	else \
		echo "Starting $$NUM nodes (no DEVICE_TOKEN)..."; \
		$(COMPOSE) up -d --scale node=$$NUM; \
	fi

down:
	@echo "Stopping fleet..."
	$(COMPOSE) down

status:
	@echo "Running: $$(docker ps -q -f label=demo.service=operator-node | wc -l) nodes"

clean:
	@echo "Cleaning up..."
	$(COMPOSE) down -v --rmi all

logs:
	$(COMPOSE) logs -f
