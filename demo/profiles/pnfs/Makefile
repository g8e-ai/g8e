# g8e pNFS Governance Demo Makefile

.PHONY: help build up down status clean deploy vanish shell logs devices broken

COMPOSE := docker compose
CONTAINER_PREFIX := pnfs
G8E_ENDPOINT ?= 172.17.0.1

help:
	@echo "g8e pNFS Governance Demo"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "  up               - Build and start the pNFS cluster"
	@echo "  down             - Stop the cluster"
	@echo "  status           - Show container status"
	@echo "  clean            - Remove containers and volumes"
	@echo "  deploy -d <tk>   - Deploy g8e operators to the client node"
	@echo "  shell N=client   - Shell into the client node"
	@echo "  logs             - Follow container logs"

build:
	@echo "Building pNFS containers..."
	$(COMPOSE) build

up: build
	@echo "Starting pNFS cluster..."
	$(COMPOSE) up -d
	@echo ""
	@echo "pNFS cluster is running."
	@echo "  Client Node:    pnfs-client-node"
	@echo "  Metadata Srv:   pnfs-metadata-server"
	@echo "  Data Srv:       pnfs-data-server"
	@echo ""
	@echo "Deploy operator to start governance demo:"
	@echo "  ./g8e demo deploy -d dlk_your_token"

down:
	@echo "Stopping pNFS cluster..."
	$(COMPOSE) down

status:
	@$(COMPOSE) ps --format "table {{.Name}}\t{{.Status}}"

clean:
	@echo "Cleaning up..."
	$(COMPOSE) down -v --rmi local

deploy:
	@set -e; \
	TOKEN="$(DEVICE_TOKEN)"; \
	if [ -z "$$TOKEN" ] && echo "$(filter-out $@,$(MAKECMDGOALS))" | grep -q '\-d'; then \
		TOKEN=$$(echo "$(filter-out $@,$(MAKECMDGOALS))" | sed 's/.*-d \([^ ]*\).*/\1/'); \
	fi; \
	if [ -z "$$TOKEN" ]; then \
		echo "Usage: make deploy -d dlk_xxx"; \
		exit 1; \
	fi; \
	echo "Deploying operator to client node..."; \
	docker exec pnfs-client-node sh -c "\
		curl -fsSL -H \"Authorization: Bearer $$TOKEN\" \
			-o g8e.operator \"https://$(G8E_ENDPOINT)/blob/operator-binary/linux-amd64\" && \
		chmod +x g8e.operator && \
		nohup ./g8e.operator --endpoint $(G8E_ENDPOINT) --working-dir /root --log info -D \"$$TOKEN\" < /dev/null > g8e.operator.log 2>&1 & \
		PID=\$$!; \
		echo \"    operator started (pid: \$$PID)\""

shell:
	@test -n "$(N)" || (echo "Usage: make shell N=client-node" && exit 1)
	@docker exec -it pnfs-$(N) bash

logs:
	$(COMPOSE) logs -f

devices:
	@docker ps --filter "label=demo.service=pnfs-node" --format '{{.Names}}' | sort

broken:
	@echo "No broken nodes in this demo; focus on governance."
