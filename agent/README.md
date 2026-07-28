# Activity Case Review Agent

LangGraph-based investigation agent for make_friends Trust & Safety cases.
Investigates reported content, settlement disputes, moderation appeals, and credit appeals.

## Architecture

```
┌─────────────┐     LPUSH      ┌──────────────┐      HTTP       ┌────────────┐
│  Go Backend │ ──────────────► │ Redis Queue  │ ◄─── BRPOP ──── │ Python     │
│  (trigger)  │                 │ agent:tasks  │                  │ Worker     │
└─────────────┘                 └──────────────┘                  └─────┬──────┘
                                                                        │
                                                                        ▼
                                                              ┌─────────────────┐
                                                              │  LangGraph Run  │
                                                              │                 │
                                                              │  load_case      │
                                                              │  extract_claims │
                                                              │  investigate ×N │
                                                              │  summarize      │
                                                              │  evaluate       │
                                                              │  report         │
                                                              └────────┬────────┘
                                                                       │
                                                                       ▼
                                                              ┌─────────────────┐
                                                              │ CaseDecision    │
                                                              │ status=proposed │
                                                              │ + actions[]     │
                                                              └─────────────────┘
                                                                       │
                                                              admin approves ───► executed
```

### Safety Contract

The agent NEVER auto-executes penalties. It outputs:
- `proposed_decision` (outcome + reasoning)
- `proposed_actions` (credit_deduct, post_takedown, etc.)

An admin reviews via `POST /admin/cases/:id/review-decision` and either approves (triggers execution) or rejects.

### Graph Nodes

| Node | Description |
|------|-------------|
| `load_case` | Fetch case + context from backend |
| `extract_claims` | LLM extracts testable claims from the case |
| `investigate_loop` | LLM selects tools iteratively (max N steps) |
| `summarize` | Compress evidence when > 8000 chars |
| `evaluate` | LLM produces verdict + confidence + responsible party |
| `report` | LLM generates structured investigation report |

## Setup

```bash
conda activate agent
pip install -r requirements.txt
cp .env.example .env  # fill in AGENT_API_SECRET and LLM_API_KEY
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `AGENT_BACKEND_URL` | Go backend base URL | `http://localhost:8080` |
| `AGENT_API_SECRET` | Shared secret for /internal/agent/ endpoints | (required) |
| `LLM_API_KEY` | DeepSeek API key for LLM calls | (required) |
| `AGENT_LLM_MODEL` | Model name | `deepseek-chat` |
| `AGENT_MAX_STEPS` | Max investigation loop iterations | `15` |
| `AGENT_RUN_ID` | Pre-created run ID (set by worker) | — |
| `AGENT_CONCURRENCY` | Worker thread pool size | `2` |
| `REDIS_URL` | Redis connection for task queue | `redis://localhost:6379/0` |

## Running

### Worker mode (production)

```bash
conda run -n agent python -m agent.src.worker
```

The worker pulls tasks from Redis `agent:tasks` queue (pushed by Go backend's
`POST /admin/cases/:id/investigate`).

### CLI mode (development/debugging)

```bash
conda run -n agent python -m agent.src.cli investigate --case-id <CASE_ID>
```

### Evaluation

```bash
# Offline eval (golden cases, no backend needed)
conda run -n agent python -m agent.src.eval --mode llm

# End-to-end eval (seeds DB, starts backend, runs all scenarios)
conda run -n agent python -m agent.src.eval_scenarios --output results.json
```

## Evaluation Metrics

| Metric | Description |
|--------|-------------|
| `accuracy` | Exact outcome match (primary) |
| `evidence_recall` | Fraction of required evidence found in agent output |
| `fabrication_free` | No forbidden claims in agent output |
| `policy_recall` | Fraction of expected policy refs cited |
| `party_accuracy` | Responsible party identification |
| `calibration` | Confidence > 0.6 iff correct |

## Project Structure

```
agent/
├── src/
│   ├── cli.py              # CLI entry point
│   ├── client.py           # HTTP client for Go backend
│   ├── config.py           # Config + env loading
│   ├── eval.py             # Offline eval harness
│   ├── eval_scenarios.py   # End-to-end scenario eval
│   ├── graph.py            # LangGraph state + nodes
│   ├── llm_nodes.py        # LLM-powered graph nodes
│   ├── runner.py           # Investigation orchestrator
│   └── worker.py           # Redis queue consumer
├── policies/               # YAML policy documents
├── tests/
│   ├── golden_cases/       # Offline test fixtures (JSON)
│   ├── golden_manifest.json# End-to-end scenario ground truth
│   └── test_*.py           # Unit/integration tests
└── requirements.txt
```
