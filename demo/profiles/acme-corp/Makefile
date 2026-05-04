# ACME Corp Global Fleet Demo
# A fake mixed-enterprise (retail/factory/office/DC) with up to 1000 named,
# globally-distributed edge devices. ~5% are broken with realistic issues
# that an AI operator can diagnose and fix via config edits + service restarts.

.PHONY: help generate-compose build up down status clean logs shell operators vanish devices broken

NODE_COUNT ?= 100
FAILURE_RATIO_PERCENT ?= 5
COMPOSE := docker compose
COMPOSE_FILES = $(shell if [ -f .batch-files ]; then for f in $$(cat .batch-files); do printf -- "-f %s " $$f; done; fi)
CONTAINER_PREFIX := acme-

help:
	@echo "ACME Corp Global Fleet Demo"
	@echo ""
	@echo "Usage: make [target] [NODE_COUNT=N]"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  FLEET LIFECYCLE"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "  up [NODE_COUNT=100]  - Build and start N devices (default 100, max ~1000)"
	@echo "  down                 - Stop all devices"
	@echo "  status               - Show container status"
	@echo "  clean                - Remove everything (containers, images, volumes)"
	@echo "  logs                 - Follow all container logs"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  INSPECTION"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "  devices              - List all device hostnames"
	@echo "  broken               - List devices with non-healthy profiles"
	@echo "  operators            - Show operator process status on all devices"
	@echo "  shell N=<name>       - Drop into a device's shell (e.g. N=pos-store-nyc-001)"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  OPERATOR MANAGEMENT"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "  Pass DEVICE_TOKEN=dlk_xxx to 'make up' to auto-attach operators."
	@echo "  vanish               - Kill operator processes on all devices"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  DEVICE TAXONOMY (proportional to NODE_COUNT)"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "  Retail:    pos-store-* kiosk-airport-* camera-store-*"
	@echo "  Warehouse: scanner-warehouse-* sensor-warehouse-*"
	@echo "  Factory:   sensor-factory-* controller-factory-*"
	@echo "  Office:    camera-hq-* printer-office-* badge-office-*"
	@echo "  Network:   gateway-branch-* router-hub-*"
	@echo "  Data ctr:  logger-dc-* probe-dc-*"
	@echo ""

generate-compose:
	@FAILURE_RATIO_PERCENT=$(FAILURE_RATIO_PERCENT) ./scripts/generate-compose.sh $(NODE_COUNT) 100

build: generate-compose
	@echo "Building edge-device image (direct docker build, bypassing compose)..."
	docker build -t acme-edge-device:latest ./containers/edge-device

up: generate-compose build
	@echo "Creating network..."
	@docker network create acme-net 2>/dev/null || echo "Network already exists"
	@echo "Starting $(NODE_COUNT) ACME edge devices in regions..."
	@for batch in $$(cat .batch-files); do \
		echo "Starting batch: $$batch"; \
		DEVICE_TOKEN="$(DEVICE_TOKEN)" $(COMPOSE) -f "$$batch" up -d --no-build || exit 1; \
	done
	@echo ""
	@echo "==========================================================="
	@echo "  ACME Corp Global Fleet — $(NODE_COUNT) devices up"
	@echo "==========================================================="
	@echo ""
	@echo "  Quick inspection:"
	@echo "    make devices           # list all device hostnames"
	@echo "    make broken            # list devices with non-healthy profiles"
	@echo "    make logs              # follow all logs"
	@echo "    make shell N=<name>    # shell into a specific device"
	@echo ""
	@echo "  Sample prompts for g8e:"
	@echo "    'Which devices haven't logged in the last 10 minutes?'"
	@echo "    'Find all NYC point-of-sale terminals and check their status'"
	@echo "    'Fix any device in a crash loop'"
	@echo "    'Which devices have TLS cert issues? Rotate them.'"
	@echo ""
	@echo "==========================================================="

down:
	@echo "Stopping fleet..."
	@if [ -n "$(COMPOSE_FILES)" ]; then \
		$(COMPOSE) $(COMPOSE_FILES) down; \
	else \
		$(COMPOSE) down; \
	fi

status:
	@echo "Fleet Status"
	@echo "============"
	@running=$$(docker ps -q -f "label=demo.service=acme-edge" | wc -l); \
	total=$$(docker ps -a -q -f "label=demo.service=acme-edge" | wc -l); \
	echo "  devices running: $$running / $$total"

clean:
	@echo "Cleaning up ACME Corp fleet..."
	@# Forcefully remove all containers with the demo label
	@docker ps -aq --filter "label=demo.service=acme-edge" | xargs -r docker rm -f
	@rm -f docker-compose.*.yml .batch-files
	@docker network rm acme-net 2>/dev/null || echo "Network cleaned."
	@echo "Done."

logs:
	@if [ -n "$(COMPOSE_FILES)" ]; then \
		$(COMPOSE) $(COMPOSE_FILES) logs -f; \
	else \
		$(COMPOSE) logs -f; \
	fi

shell:
	@test -n "$(N)" || (echo "Usage: make shell N=<device-name>  (e.g. N=pos-store-nyc-001)" && exit 1)
	@docker exec -it $(CONTAINER_PREFIX)$(N) bash

devices:
	@docker ps --filter "label=demo.service=acme-edge" --format '{{.Names}}' \
		| sed 's/^$(CONTAINER_PREFIX)//' | sort

broken:
	@echo "Devices with non-healthy profiles"
	@echo "================================="
	@for c in $$(docker ps --filter "label=demo.service=acme-edge" --format '{{.Names}}'); do \
		prof=$$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' $$c 2>/dev/null | awk -F= '/^DEVICE_PROFILE=/ {print $$2}'); \
		case "$$prof" in healthy|"") ;; *) printf "  %-40s %s\n" "$${c#$(CONTAINER_PREFIX)}" "$$prof";; esac; \
	done

operators:
	@echo ""
	@echo "Operator Status"
	@echo "==============="
	@running=0; total=0; \
	for c in $$(docker ps --filter "label=demo.service=acme-edge" --format '{{.Names}}'); do \
		total=$$((total + 1)); \
		if docker exec $$c pgrep -f g8e.operator > /dev/null 2>&1; then \
			running=$$((running + 1)); \
		fi; \
	done; \
	echo "  operators running: $$running / $$total"

vanish:
	@echo "Removing operators from all devices..."
	@for c in $$(docker ps --filter "label=demo.service=acme-edge" --format '{{.Names}}'); do \
		PID=$$(docker exec $$c pgrep -f 'g8e\.operator' 2>/dev/null); \
		if [ -n "$$PID" ]; then \
			docker exec $$c kill $$PID 2>/dev/null || true; \
		fi; \
		docker exec $$c sh -c 'rm -f /tmp/g8e.operator /home/appuser/g8e.operator /root/g8e.operator ./g8e.operator' 2>/dev/null || true; \
	done
	@echo "All operators removed."
