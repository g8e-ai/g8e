# Fleet Demo (20-1000 nodes)

.PHONY: help build up down status clean logs deploy vanish operators discover-hosts dashboard

COMPOSE := docker compose

help:
	@echo "Fleet Demo (20-1000 nodes)"
	@echo "Usage: make [target]"
	@echo ""
	@echo "  up N=20 [-d dlk_xxx]    - Start N identical nodes with operators and dashboard"
	@echo "  down                    - Stop all nodes and dashboard"
	@echo "  status                  - Show container status"
	@echo "  dashboard               - Show dashboard URL"
	@echo "  devices                 - List all device hostnames"
	@echo "  broken                  - List devices with non-healthy status"
	@echo "  clean                   - Remove everything"
	@echo "  logs                    - Follow all container logs"
	@echo ""
	@echo "  deploy [-d dlk_xxx]     - Deploy operators to all nodes"
	@echo "  vanish                  - Remove all operators"
	@echo "  operators               - Show operator status"
	@echo "  discover-hosts          - List all node names"

build:
	@echo "Building featherweight node and dashboard images..."
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
		echo "Starting $$NUM nodes and dashboard (operators auto-attach via DEVICE_TOKEN)..."; \
		DEVICE_TOKEN="$$TOKEN" $(COMPOSE) up -d --scale node=$$NUM dashboard; \
	else \
		echo "Starting $$NUM nodes and dashboard (no DEVICE_TOKEN)..."; \
		$(COMPOSE) up -d --scale node=$$NUM dashboard; \
	fi; \
	echo ""; \
	echo "Dashboard: http://localhost:8080"

dashboard:
	@echo "Dashboard is available at: http://localhost:8080"

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

deploy:
	@set -e; \
	TOKEN="$(DEVICE_TOKEN)"; \
	if [ -z "$$TOKEN" ] && echo "$(filter-out $@,$(MAKECMDGOALS))" | grep -q '\-d'; then \
		TOKEN=$$(echo "$(filter-out $@,$(MAKECMDGOALS))" | sed 's/.*-d \([^ ]*\).*/\1/'); \
	fi; \
	if [ -z "$$TOKEN" ]; then \
		echo "Usage: make deploy -d dlk_xxx"; \
		echo "       make deploy DEVICE_TOKEN=dlk_xxx"; \
		exit 1; \
	fi; \
	echo "Deploying operators to all nodes..."; \
	for id in $$(docker ps -q -f label=demo.service=operator-node); do \
		name=$$(docker inspect --format '{{.Name}}' $$id | sed 's/^\///'); \
		echo "  Deploying to $$name..."; \
		docker exec -e DEVICE_TOKEN="$$TOKEN" $$id sh -c '\
			curl -fsSL --cacert /g8es/ca.crt -H "Authorization: Bearer $$DEVICE_TOKEN" \
				-o g8e.operator "https://g8e.local/operator/download/linux/amd64" && \
			curl -fsSL --cacert /g8es/ca.crt -H "Authorization: Bearer $$DEVICE_TOKEN" \
				-o g8e.operator.sha256 "https://g8e.local/operator/download/linux/amd64/sha256" && \
			sha256sum -c g8e.operator.sha256 && \
			chmod +x g8e.operator && \
			nohup ./g8e.operator -D "$$DEVICE_TOKEN" -e g8e.local > g8e.operator.log 2>&1 & \
			echo "    operator started (pid: $$!)"' || true; \
	done; \
	echo "Done."

vanish:
	@echo "Removing operators from all nodes..."
	@for id in $$(docker ps -q -f label=demo.service=operator-node); do \
		name=$$(docker inspect --format '{{.Name}}' $$id | sed 's/^\///'); \
		docker exec $$id pkill -f g8e.operator 2>/dev/null || true; \
		docker exec $$id rm -f g8e.operator g8e.operator.sha256 g8e.operator.log 2>/dev/null || true; \
		echo "  $$name: cleaned"; \
	done
	@echo "Done."

operators:
	@echo "Operator Status"
	@echo "==============="
	@running=0; \
	total=0; \
	for id in $$(docker ps -q -f label=demo.service=operator-node); do \
		total=$$((total + 1)); \
		name=$$(docker inspect --format '{{.Name}}' $$id | sed 's/^\///'); \
		if docker exec $$id pgrep -f g8e.operator > /dev/null 2>&1; then \
			echo "  $$name: running"; \
			running=$$((running + 1)); \
		else \
			echo "  $$name: not deployed"; \
		fi; \
	done; \
	echo ""; \
	echo "Operators: $$running / $$total"

discover-hosts:
	@docker ps --filter "label=demo.service=operator-node" --format '{{.Names}}' | sort

devices: discover-hosts

broken:
	@echo "Devices with non-healthy status"
	@echo "==============================="
	@echo "  (All fleet nodes are currently configured as healthy)"
