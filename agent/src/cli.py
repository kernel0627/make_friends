"""CLI entry point for the agent service.

Usage:
    python -m agent.src.cli investigate --case-id <case_id>

Environment variables:
    AGENT_BACKEND_URL  — Go backend base URL (default http://localhost:8080)
    AGENT_API_SECRET   — Secret token for /internal/agent/* endpoints
    LLM_API_KEY        — API key for LLM provider
    LLM_BASE_URL       — OpenAI-compatible endpoint
    LLM_MODEL          — Model name
"""

from __future__ import annotations

import argparse
import json
import logging
import sys

from .config import load_config
from .runner import run_investigation


def main() -> int:
    parser = argparse.ArgumentParser(prog="agent", description="Case investigation agent")
    sub = parser.add_subparsers(dest="command")

    # investigate subcommand
    inv = sub.add_parser("investigate", help="Run investigation on a case")
    inv.add_argument("--case-id", required=True, help="Case ID to investigate")
    inv.add_argument("--verbose", "-v", action="store_true", help="Enable debug logging")

    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        return 1

    # Setup logging
    level = logging.DEBUG if args.verbose else logging.INFO
    logging.basicConfig(
        level=level,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
        stream=sys.stderr,
    )

    if args.command == "investigate":
        return _do_investigate(args.case_id)

    return 1


def _do_investigate(case_id: str) -> int:
    """Run investigation and output result as JSON to stdout."""
    config = load_config()

    if not config.agent_api_secret:
        print("ERROR: AGENT_API_SECRET not set", file=sys.stderr)
        return 1

    if not config.llm_api_key:
        print("ERROR: LLM_API_KEY not set", file=sys.stderr)
        return 1

    result = run_investigation(case_id, config=config)

    if "error" in result:
        print(f"Investigation failed: {result['error']}", file=sys.stderr)
        # Still output JSON for structured consumption
        json.dump({"status": "failed", "error": result["error"], "run_id": result.get("run_id")}, sys.stdout)
        print()
        return 1

    # Output structured result
    output = {
        "status": "completed",
        "run_id": result.get("run_id", ""),
        "case_id": case_id,
        "verdict": result.get("verdict", ""),
        "step_count": result.get("step_count", 0),
    }
    json.dump(output, sys.stdout)
    print()
    return 0


if __name__ == "__main__":
    sys.exit(main())
