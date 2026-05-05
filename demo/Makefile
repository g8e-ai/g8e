# g8e Demo Dispatcher
# Delegates to the active demo profile in profiles/

PROFILES_DIR := profiles
ACTIVE_PROFILE_FILE := $(PROFILES_DIR)/.active
ACTIVE_PROFILE := $(shell cat $(ACTIVE_PROFILE_FILE) 2>/dev/null || echo "acme-corp")
PROFILE_PATH := $(PROFILES_DIR)/$(ACTIVE_PROFILE)

.PHONY: all help switch list

# Standard demo targets to delegate
TARGETS := build up down status clean logs shell deploy discover-hosts stream vanish health nginx-check operators dashboard devices broken

help:
	@echo "g8e Demo System"
	@echo "Active Profile: $(ACTIVE_PROFILE)"
	@echo ""
	@echo "Commands:"
	@echo "  list            - List available demo profiles"
	@echo "  switch P=<name> - Switch to a different profile"
	@echo ""
	@echo "Forwarded Commands (to $(ACTIVE_PROFILE)):"
	@$(MAKE) -C $(PROFILE_PATH) help --no-print-directory | grep -E '^  [a-z]' || echo "  (no help available for this profile)"

list:
	@echo "Available Profiles:"
	@for d in $(PROFILES_DIR)/*/; do \
		p=$$(basename $$d); \
		if [ "$$p" = "$(ACTIVE_PROFILE)" ]; then \
			echo "  * $$p (active)"; \
		else \
			echo "    $$p"; \
		fi; \
	done

switch:
	@if [ -z "$(P)" ]; then \
		echo "Usage: make switch P=<profile-name>"; \
		exit 1; \
	fi
	@if [ ! -d "$(PROFILES_DIR)/$(P)" ]; then \
		echo "Profile '$(P)' not found in $(PROFILES_DIR)"; \
		exit 1; \
	fi
	@echo "$(P)" > $(ACTIVE_PROFILE_FILE)
	@echo "Switched to profile: $(P)"

# Delegate all other targets to the active profile
# Use --no-print-directory to keep output clean
# Filter out MAKEFLAGS that might cause sub-make to fail
$(TARGETS):
	@if [ ! -d "$(PROFILE_PATH)" ]; then \
		echo "Error: Active profile directory $(PROFILE_PATH) not found."; \
		exit 1; \
	fi
	@$(MAKE) -C $(PROFILE_PATH) --no-print-directory $@ $(filter-out $@,$(MAKECMDGOALS))
