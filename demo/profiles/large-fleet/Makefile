# Featherweight Large Fleet Simulator (20-1000 nodes)
# Uses minimal Alpine Linux containers to run just the Operator binary.

.PHONY: help generate-compose build up down status clean logs

NODE_COUNT ?= 100
COMPOSE := docker compose
COMPOSE_FILES = $(shell if [ -f .batch-files ]; then for f in $$(cat .batch-files); do printf -- "-f %s " $$f; done; fi)

help:
	@echo "Featherweight Large Fleet Simulator"
	@echo ""
	@echo "Usage: make [target] [NODE_COUNT=N]"
	@echo ""
	@echo "  up [NODE_COUNT=100]  - Start N nodes (default 100)"
	@echo "  down                 - Stop all nodes"
	@echo "  status               - Show status"
	@echo "  clean                - Remove everything"
	@echo "  logs                 - Follow logs"
	@echo ""

generate-compose:
	@./scripts/generate-compose.sh $(NODE_COUNT) 100

build: generate-compose
	@echo "Building image..."
	docker build -t operator-node:latest ./containers/node

up: generate-compose build
	@echo "Creating network..."
	@docker network create large-fleet-net 2>/dev/null || echo "Network already exists"
	@echo "Starting $(NODE_COUNT) nodes in regions..."
	@for batch in $$(cat .batch-files); do \
		echo "Starting batch: $$batch"; \
		DEVICE_TOKEN="$(DEVICE_TOKEN)" $(COMPOSE) -f "$$batch" up -d --no-build || exit 1; \
	done

down:
	@echo "Stopping fleet..."
	@if [ -n "$(COMPOSE_FILES)" ]; then \
		$(COMPOSE) $(COMPOSE_FILES) down; \
	else \
		$(COMPOSE) down; \
	fi
	@docker network rm large-fleet-net 2>/dev/null || true

status:
	@running=$$(docker ps -q -f "label=demo.service=operator-node" | wc -l); \
	total=$$(docker ps -a -q -f "label=demo.service=operator-node" | wc -l); \
	echo "  nodes running: $$running / $$total"

clean:
	@echo "Cleaning up..."
	@docker ps -aq --filter "label=demo.service=operator-node" | xargs -r docker rm -f
	@rm -f docker-compose.*.yml .batch-files
	@docker network rm large-fleet-net 2>/dev/null || true
	@echo "Done."

logs:
	@if [ -n "$(COMPOSE_FILES)" ]; then \
		$(COMPOSE) $(COMPOSE_FILES) logs -f; \
	else \
		$(COMPOSE) logs -f; \
	fi
