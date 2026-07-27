# Activity Case Review Agent

LangGraph-based investigation agent for make_friends Trust & Safety cases.

## Setup

```bash
conda activate agent
pip install -r requirements.txt
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `AGENT_BACKEND_URL` | Go backend URL | `http://localhost:8080` |
| `AGENT_API_SECRET` | Shared secret for /internal/agent/ endpoints | (required) |
| `ANTHROPIC_API_KEY` | Anthropic API key | (required for Phase 3+) |
| `AGENT_LLM_MODEL` | Model to use | `claude-sonnet-4-20250514` |
| `AGENT_MAX_STEPS` | Max investigation steps | `15` |
| `AGENT_PORT` | Agent service port | `8090` |

## Architecture

```
load_case → extract_claims → investigate_loop → evaluate_evidence → generate_report
```

The `investigate_loop` dynamically selects tools (get_domain_events, get_chat_messages,
get_user_history, get_user_profile) until it has enough evidence or hits the step limit.

## Current Status

**Phase 1 (skeleton):** Deterministic 3-step investigation with template-based report.
LLM integration comes in Phase 3.
