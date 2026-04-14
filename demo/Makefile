# Broken Web App Fleet Demo - 10 Nginx Nodes
# Each node runs nginx proxying to a Flask backend.
# 5 healthy, 5 broken in different ways. Deploy operators and fix with g8e.

.PHONY: help build up down status clean deploy setup-ssh discover-hosts stream vanish shell logs health nginx-check operators dashboard

COMPOSE := docker compose
CONTAINER_PREFIX := web-node
SSH_USER := appuser

help:
	@echo "Broken Web App Fleet Demo"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  FLEET LIFECYCLE"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "  up               - Build and start all 10 web nodes"
	@echo "  down             - Stop all nodes"
	@echo "  status           - Show container status"
	@echo "  clean            - Remove everything (containers, images, volumes)"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  OPERATOR DEPLOYMENT"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "  deploy DEVICE_TOKEN=dlk_xxx  - Deploy operators via API download (device link)"
	@echo "  stream DEVICE_TOKEN=dlk_xxx  - Deploy operators via SSH streaming (auto-configures SSH)"
	@echo "  discover-hosts              - List discovered demo fleet hosts"
	@echo "  vanish                      - Remove all operators (zero trace)"
	@echo "  operators                   - Show operator status on all nodes"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  FLEET INSPECTION"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "  health           - Check Flask backend health on all nodes"
	@echo "  nginx-check      - Check nginx status on all nodes"
	@echo "  dashboard        - Print dashboard URL (http://localhost:3000)"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  DEBUG"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "  shell N=01       - Shell into specific node"
	@echo "  logs             - Follow all container logs"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  NODE PROFILES"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "  node-01 to 05: Healthy (nginx -> Flask, working)"
	@echo "  node-06:       Bad Upstream (proxy_pass wrong port -> 502)"
	@echo "  node-07:       SSL Expired (HTTPS with bad cert)"
	@echo "  node-08:       Wrong Root (document root missing -> 404)"
	@echo "  node-09:       High Load (tiny buffers -> 504 timeouts)"
	@echo "  node-10:       Crashed (nginx config syntax error, won't start)"

build:
	@echo "Building web node image..."
	$(COMPOSE) build

up: build
	@echo "Starting 10 web nodes..."
	$(COMPOSE) up -d
	@echo ""
	@echo "========================================"
	@echo "  Broken Web App Fleet - 10 Nodes"
	@echo "========================================"
	@echo ""
	@echo "Node Profiles:"
	@echo "  node-01 to 05: Healthy"
	@echo "  node-06: Bad Upstream (502)"
	@echo "  node-07: SSL Expired"
	@echo "  node-08: Wrong Root (404)"
	@echo "  node-09: High Load (504)"
	@echo "  node-10: Crashed (nginx won't start)"
	@echo ""
	@echo "Deploy operators:"
	@echo "  ./g8e demo stream DEVICE_TOKEN=dlk_your_token"
	@echo ""
	@echo "Dashboard: http://localhost:3000"
	@echo ""
	@echo "========================================"

setup-ssh:
	@docker cp $(CONTAINER_PREFIX)-01:/opt/demo-key ./.demo-key 2>/dev/null && chmod 600 ./.demo-key
	@docker exec g8ep mkdir -p /home/g8e/.ssh
	@cat ./.demo-key | docker exec -i g8ep bash -c "cat > /home/g8e/.ssh/id_ed25519"
	@printf 'Host web-node-* node-*\n    User $(SSH_USER)\n    IdentityFile /home/g8e/.ssh/id_ed25519\n    StrictHostKeyChecking accept-new\n' \
		| docker exec -i g8ep bash -c "cat > /home/g8e/.ssh/config"
	@docker exec g8ep chmod 700 /home/g8e/.ssh
	@docker exec g8ep chmod 600 /home/g8e/.ssh/id_ed25519 /home/g8e/.ssh/config

down:
	@echo "Stopping fleet..."
	$(COMPOSE) down

status:
	@echo "Fleet Status"
	@echo "============"
	@$(COMPOSE) ps --format "table {{.Name}}\t{{.Status}}"
	@echo ""
	@echo "Running: $$(docker ps -q -f name=$(CONTAINER_PREFIX) | wc -l) / 10"

clean:
	@echo "Cleaning up..."
	$(COMPOSE) down -v --rmi all
	@echo "Done."

logs:
	$(COMPOSE) logs -f

shell:
	@test -n "$(N)" || (echo "Usage: make shell N=01" && exit 1)
	@docker exec -it $(CONTAINER_PREFIX)-$(N) bash

health:
	@echo "Backend Health - All Nodes"
	@echo "=========================="
	@for i in $$(seq -f "%02g" 1 10); do \
		result=$$(docker exec $(CONTAINER_PREFIX)-$$i curl -s http://127.0.0.1:5000/health 2>/dev/null || echo '{"status":"unreachable"}'); \
		status=$$(echo "$$result" | grep -o '"status":"[^"]*"' | cut -d'"' -f4); \
		printf "node-%s: %s\n" "$$i" "$$status"; \
	done

dashboard:
	@echo "Fleet dashboard: http://localhost:3000"

nginx-check:
	@echo "Nginx Status - All Nodes"
	@echo "========================"
	@for i in $$(seq -f "%02g" 1 10); do \
		if docker exec $(CONTAINER_PREFIX)-$$i pgrep nginx > /dev/null 2>&1; then \
			http=$$(docker exec $(CONTAINER_PREFIX)-$$i curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8181/ 2>/dev/null || echo "000"); \
			printf "node-%s: nginx running, HTTP %s\n" "$$i" "$$http"; \
		else \
			printf "node-%s: nginx NOT running\n" "$$i"; \
		fi; \
	done

deploy:
	@test -n "$(DEVICE_TOKEN)" || (echo "Usage: make deploy DEVICE_TOKEN=dlk_xxx" && exit 1)
	@echo "Deploying operators to all 10 nodes..."
	@for i in $$(seq -f "%02g" 1 10); do \
		echo "  Deploying to node-$$i..."; \
		docker exec $(CONTAINER_PREFIX)-$$i sh -c '\
			curl -fsSLk -H "Authorization: Bearer $(DEVICE_TOKEN)" \
				-o g8e.operator "https://g8e.local/operator/download/linux/amd64" && \
			curl -fsSLk -H "Authorization: Bearer $(DEVICE_TOKEN)" \
				-o g8e.operator.sha256 "https://g8e.local/operator/download/linux/amd64/sha256" && \
			sha256sum -c g8e.operator.sha256 && \
			chmod +x g8e.operator && \
			nohup ./g8e.operator -D "$(DEVICE_TOKEN)" -e g8es --http-port 9000 --wss-port 9001 > g8e.operator.log 2>&1 & \
			echo "    operator started (pid: $$!)"' || true; \
	done
	@echo "Done."

discover-hosts:
	@docker ps --filter "label=demo.service=web-node" --format '$(SSH_USER)@{{.Names}}' | sort

stream:
	@test -n "$(DEVICE_TOKEN)" || (echo "Usage: make stream DEVICE_TOKEN=dlk_xxx" && exit 1)
	@echo "Configuring SSH in g8ep..."
	@$(MAKE) -s setup-ssh
	@echo "Discovering demo fleet hosts..."
	@HOSTS=$$(docker ps --filter "label=demo.service=web-node" --format '$(SSH_USER)@{{.Names}}' | sort | tr '\n' ' '); \
	if [ -z "$$HOSTS" ]; then echo "No demo nodes found. Run './g8e demo up' first." && exit 1; fi; \
	echo "  Streaming to: $$HOSTS"; \
	docker exec g8ep /home/g8e/g8e.operator stream \
		--endpoint g8e.local \
		--device-token "$(DEVICE_TOKEN)" \
		$$HOSTS

operators:
	@echo ""
	@echo "Operator Status - All Nodes"
	@echo "============================"
	@running=0; \
	for i in $$(seq -f "%02g" 1 10); do \
		if docker exec $(CONTAINER_PREFIX)-$$i pgrep -f g8e.operator > /dev/null 2>&1; then \
			echo "  node-$$i: running"; \
			running=$$((running + 1)); \
		else \
			echo "  node-$$i: not deployed"; \
		fi; \
	done; \
	echo ""; \
	echo "Operators: $$running / 10"

vanish:
	@echo ""
	@echo "Removing operators from all nodes..."
	@echo ""
	@for i in $$(seq -f "%02g" 1 10); do \
		docker exec $(CONTAINER_PREFIX)-$$i pkill -f g8e.operator 2>/dev/null || true; \
		docker exec $(CONTAINER_PREFIX)-$$i rm -f /tmp/g8e.operator /home/appuser/g8e.operator /root/g8e.operator ./g8e.operator 2>/dev/null || true; \
		echo "  node-$$i: cleaned"; \
	done
	@echo ""
	@echo "All operators removed. Zero trace."
	@echo ""
