"""Evaluation harness for the investigation agent.

Runs golden test cases against the agent (LLM or skeleton mode) and
measures verdict accuracy, evidence coverage, and latency.

Usage (from project root):
    conda run -n agent python -m agent.src.eval --mode skeleton
    conda run -n agent python -m agent.src.eval --mode llm
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import time
from pathlib import Path
from typing import Any

# Add project root to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from agent.src.config import Config, load_config
from agent.src.graph import InvestigationState, build_graph, build_llm_graph
from agent.tests.test_golden_cases import load_all_golden_cases

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
logger = logging.getLogger(__name__)


def run_golden_case_offline(case: dict[str, Any], use_llm: bool = False, config: Config | None = None) -> dict[str, Any]:
    """Run a golden case through the graph without a backend.

    Injects case_input directly into the state instead of calling the backend.
    """
    if config is None:
        config = load_config()

    case_input = case["case_input"]

    # Build initial state from the golden case fixture
    initial_state: InvestigationState = {
        "case_id": case_input["case"]["id"],
        "run_id": f"eval_{case['id']}",
        "case_data": case_input["case"],
        "full_context": case_input,
        "steps": [],
        "evidence": [],
        "step_count": 0,
    }

    if use_llm:
        from agent.src.llm_nodes import (
            extract_claims_llm,
            evaluate_llm,
            report_llm,
        )
        # In offline mode with LLM, skip the investigate loop (no backend)
        # and go straight from claims → evaluate → report using fixture data
        claims_result = extract_claims_llm(initial_state, config)
        initial_state.update(claims_result)

        # Inject all available evidence from the fixture
        evidence = []
        if case_input.get("domain_events"):
            evidence.append({"type": "domain_events", "data": case_input["domain_events"]})
        if case_input.get("messages"):
            evidence.append({"type": "chat_messages", "data": case_input["messages"]})
        if case_input.get("user_history"):
            evidence.append({"type": "user_history", "data": case_input["user_history"]})
        if case_input.get("participants"):
            evidence.append({"type": "participants", "data": case_input["participants"]})
        if case_input.get("settlements"):
            evidence.append({"type": "settlements", "data": case_input["settlements"]})
        if case_input.get("moderations"):
            evidence.append({"type": "moderations", "data": case_input["moderations"]})
        if case_input.get("credit_ledgers"):
            evidence.append({"type": "credit_ledgers", "data": case_input["credit_ledgers"]})
        if case_input.get("reporter_history"):
            evidence.append({"type": "reporter_history", "data": case_input["reporter_history"]})

        initial_state["evidence"] = evidence

        eval_result = evaluate_llm(initial_state, config)
        initial_state.update(eval_result)

        report_result = report_llm(initial_state, config)
        initial_state.update(report_result)

        return initial_state
    else:
        # Skeleton mode: use deterministic nodes but skip load_case (inject data)
        from agent.src.graph import extract_claims_node, evaluate_node, report_node

        claims_result = extract_claims_node(initial_state)
        initial_state.update(claims_result)

        # Inject evidence from fixture
        evidence = []
        if case_input.get("domain_events"):
            evidence.append({"type": "domain_events", "data": case_input["domain_events"]})
        if case_input.get("messages"):
            evidence.append({"type": "chat_messages", "data": case_input["messages"]})
        initial_state["evidence"] = evidence
        initial_state["step_count"] = len(evidence)

        eval_result = evaluate_node(initial_state)
        initial_state.update(eval_result)

        report_result = report_node(initial_state)
        initial_state.update(report_result)

        return initial_state


def evaluate_all(use_llm: bool = False) -> dict[str, Any]:
    """Run all golden cases and compute metrics."""
    cases = load_all_golden_cases()
    config = load_config()

    results = []
    correct = 0
    total = len(cases)

    for case in cases:
        case_id = case["id"]
        expected = case["expected_verdict"]
        logger.info(f"Running {case_id} (expected: {expected})...")

        start = time.time()
        try:
            final_state = run_golden_case_offline(case, use_llm=use_llm, config=config)
            actual = final_state.get("verdict", "error")
            confidence = final_state.get("confidence", 0.0)
            elapsed = time.time() - start

            is_correct = actual == expected
            if is_correct:
                correct += 1

            results.append({
                "id": case_id,
                "case_type": case["case_type"],
                "difficulty": case["difficulty"],
                "expected": expected,
                "actual": actual,
                "confidence": confidence,
                "correct": is_correct,
                "elapsed_s": round(elapsed, 2),
            })
            status = "✓" if is_correct else "✗"
            logger.info(f"  {status} got={actual} conf={confidence:.0%} ({elapsed:.1f}s)")

        except Exception as e:
            elapsed = time.time() - start
            logger.error(f"  ✗ ERROR: {e} ({elapsed:.1f}s)")
            results.append({
                "id": case_id,
                "case_type": case["case_type"],
                "difficulty": case["difficulty"],
                "expected": expected,
                "actual": "error",
                "confidence": 0.0,
                "correct": False,
                "elapsed_s": round(elapsed, 2),
                "error": str(e),
            })

    accuracy = correct / total if total > 0 else 0.0

    # Breakdown by type
    by_type = {}
    for r in results:
        t = r["case_type"]
        if t not in by_type:
            by_type[t] = {"total": 0, "correct": 0}
        by_type[t]["total"] += 1
        if r["correct"]:
            by_type[t]["correct"] += 1

    # Breakdown by difficulty
    by_diff = {}
    for r in results:
        d = r["difficulty"]
        if d not in by_diff:
            by_diff[d] = {"total": 0, "correct": 0}
        by_diff[d]["total"] += 1
        if r["correct"]:
            by_diff[d]["correct"] += 1

    summary = {
        "mode": "llm" if use_llm else "skeleton",
        "total": total,
        "correct": correct,
        "accuracy": round(accuracy, 3),
        "by_type": {k: {"accuracy": v["correct"] / v["total"]} for k, v in by_type.items()},
        "by_difficulty": {k: {"accuracy": v["correct"] / v["total"]} for k, v in by_diff.items()},
        "results": results,
    }

    return summary


def main():
    parser = argparse.ArgumentParser(description="Evaluate the investigation agent")
    parser.add_argument("--mode", choices=["skeleton", "llm"], default="skeleton",
                        help="Which graph to use (default: skeleton)")
    parser.add_argument("--output", type=str, default=None,
                        help="Write results JSON to this file")
    args = parser.parse_args()

    use_llm = args.mode == "llm"
    if use_llm:
        config = load_config()
        if not config.llm_api_key:
            logger.error("LLM_API_KEY not set in .env, cannot run LLM mode")
            sys.exit(1)

    summary = evaluate_all(use_llm=use_llm)

    print(f"\n{'='*60}")
    print(f"  Agent Evaluation — mode={args.mode}")
    print(f"{'='*60}")
    print(f"  Accuracy: {summary['correct']}/{summary['total']} ({summary['accuracy']:.1%})")
    print(f"\n  By case type:")
    for t, v in summary["by_type"].items():
        print(f"    {t}: {v['accuracy']:.1%}")
    print(f"\n  By difficulty:")
    for d, v in summary["by_difficulty"].items():
        print(f"    {d}: {v['accuracy']:.1%}")
    print(f"{'='*60}\n")

    if args.output:
        Path(args.output).write_text(json.dumps(summary, indent=2, ensure_ascii=False))
        logger.info(f"Results written to {args.output}")


if __name__ == "__main__":
    main()
