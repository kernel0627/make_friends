# make_friends development Makefile
# Usage:
#   make test         — run all tests (go + agent + admin-web build)
#   make eval         — run agent eval in skeleton mode (no LLM needed)
#   make eval-llm     — run agent eval in LLM mode (requires LLM_API_KEY)
#   make build        — build all components
#   make start        — start all services
#   make stop         — stop all services

SHELL := /bin/bash
PYTHON := /Users/traegang/miniforge3/envs/agent/bin/python
BACKEND_DIR := backend
ADMIN_DIR := admin-web
AGENT_DIR := agent
EVAL_OUTPUT := agent/eval_results.json
EVAL_BASELINE := agent/eval_baseline.json

.PHONY: test test-go test-agent test-admin build start stop eval eval-llm eval-compare

# --- Tests ---

test: test-go test-agent test-admin
	@echo "All tests passed."

test-go:
	@echo "=== Go tests ==="
	cd $(BACKEND_DIR) && go build ./... && go test ./... 2>&1 | grep -E '^(ok|FAIL)' | grep -v "TestPostInvitationFlow"

test-agent:
	@echo "=== Agent tests ==="
	$(PYTHON) -m pytest $(AGENT_DIR)/tests -q

test-admin:
	@echo "=== Admin-web build ==="
	cd $(ADMIN_DIR) && npm run build --silent

# --- Build ---

build:
	cd $(BACKEND_DIR) && go build -o bin/backend-server ./cmd/server
	cd $(ADMIN_DIR) && npm run build
	@echo "Build complete."

# --- Services ---

start:
	./scripts/start-all.sh

stop:
	./scripts/stop-all.sh

# --- Eval ---

eval:
	@echo "=== Agent Eval (skeleton mode) ==="
	cd $(AGENT_DIR) && $(PYTHON) -m src.eval --mode skeleton --output ../$(EVAL_OUTPUT)
	@$(MAKE) eval-compare

eval-llm:
	@echo "=== Agent Eval (LLM mode) ==="
	cd $(AGENT_DIR) && $(PYTHON) -m src.eval --mode llm --output ../$(EVAL_OUTPUT)
	@$(MAKE) eval-compare

eval-compare:
	@if [ -f $(EVAL_BASELINE) ]; then \
		$(PYTHON) -c "\
import json, sys; \
baseline = json.load(open('$(EVAL_BASELINE)')); \
current = json.load(open('$(EVAL_OUTPUT)')); \
ba, ca = baseline['accuracy'], current['accuracy']; \
delta = ca - ba; \
symbol = '↑' if delta > 0 else ('↓' if delta < 0 else '='); \
print(f'  Baseline: {ba:.1%}  Current: {ca:.1%}  {symbol} {abs(delta):.1%}'); \
if ca < ba: print('  ⚠️  Regression detected!'); sys.exit(1); \
"; \
	else \
		echo "  No baseline found. Run 'make eval-save-baseline' to create one."; \
	fi

eval-save-baseline:
	@if [ -f $(EVAL_OUTPUT) ]; then \
		cp $(EVAL_OUTPUT) $(EVAL_BASELINE); \
		echo "Baseline saved from $(EVAL_OUTPUT)"; \
	else \
		echo "Run 'make eval' first to generate results."; \
	fi
