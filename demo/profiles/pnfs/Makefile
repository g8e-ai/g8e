# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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
	@echo "pNFS Cluster Status"
	@echo "==================="
	@running=$$(docker ps -q -f "label=demo.service=pnfs-node" | wc -l); \
	total=$$(docker ps -a -q -f "label=demo.service=pnfs-node" | wc -l); \
	echo "  devices running: $$running / $$total"
	@$(COMPOSE) ps --format "table {{.Name}}\t{{.Status}}"

clean:
	@echo "Cleaning up..."
	$(COMPOSE) down -v --rmi local

operators:
	@echo "Operator Status"
	@echo "==============="
	@if docker exec pnfs-client-node pgrep -f g8e.operator > /dev/null 2>&1; then \
		echo "  pnfs-client-node: running"; \
	else \
		echo "  pnfs-client-node: stopped"; \
	fi

deploy:
	@set -e; \
	TOKEN="$(DEVICE_TOKEN)"; \
	if [ -z "$$TOKEN" ] && echo "$(filter-out $@,$(MAKECMDGOALS))" | grep -q '\-d'; then \
		TOKEN=$$(echo "$(filter-out $@,$(MAKECMDGOALS))" | sed 's/.*-d \([^ ]*\).*/\1/'); \
	fi; \
	if [ -z "$$TOKEN" ]; then \
		echo "Usage: ./g8e demo deploy -d dlk_xxx"; \
		exit 1; \
	fi; \
	echo "Deploying operator to client node..."; \
	docker exec -d -e DEVICE_TOKEN="$$TOKEN" pnfs-client-node sh -c "\
		CURL_OPTS=\"-fsSL --cacert /operator/ca.crt\"; \
		[ ! -f /operator/ca.crt ] && echo \"CRITICAL: /operator/ca.crt missing\" && exit 1; \
		curl \$$CURL_OPTS -H \"Authorization: Bearer \$$DEVICE_TOKEN\" \
			-o /root/g8e.operator \"https://$(G8E_ENDPOINT)/blob/operator-binary/linux-amd64\" && \
		chmod +x /root/g8e.operator && \
		nohup /root/g8e.operator --endpoint $(G8E_ENDPOINT) --working-dir /root --log info --cloud=false -D \"\$$DEVICE_TOKEN\" < /dev/null > /root/g8e.operator.log 2>&1 & \
		PID=\$$!; \
		echo \"    operator started (pid: \$$PID)\""

shell:
	@test -n "$(N)" || (echo "Usage: ./g8e demo shell N=client-node" && exit 1)
	@docker exec -it pnfs-$(N) bash

logs:
	$(COMPOSE) logs -f

devices:
	@docker ps --filter "label=demo.service=pnfs-node" --format '{{.Names}}' | sed 's/^$(CONTAINER_PREFIX)-//' | sort

discover-hosts: devices

health: status

vanish:
	@echo "Removing operator from client node..."
	@PID=$$(docker exec pnfs-client-node pgrep -f 'g8e\.operator' 2>/dev/null); \
	if [ -n "$$PID" ]; then \
		docker exec pnfs-client-node kill $$PID 2>/dev/null || true; \
	fi; \
	docker exec pnfs-client-node rm -f g8e.operator g8e.operator.log
	@echo "Operator removed."

broken:
	@echo "No broken nodes in this demo; focus on governance."
