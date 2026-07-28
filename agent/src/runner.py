"""Runner — orchestrates a full investigation run."""

from __future__ import annotations

import logging
import os
import time
from typing import Any

from .client import BackendClient
from .config import Config, load_config
from .graph import InvestigationState, build_graph, build_llm_graph

logger = logging.getLogger(__name__)


def run_investigation(case_id: str, config: Config | None = None, use_llm: bool = True) -> dict[str, Any]:
    """Execute a full investigation for the given case.

    Returns the final state dict including the report.
    """
    if config is None:
        config = load_config()

    client = BackendClient(config)

    # If a run ID was pre-created by the Go backend, reuse it
    pre_run_id = os.environ.get("AGENT_RUN_ID", "").strip()
    if pre_run_id:
        run_id = pre_run_id
        # Update existing run record
        client.update_run(run_id, status="running", model=config.llm_model)
        logger.info(f"Reusing pre-created agent run {run_id} for case {case_id}")
    else:
        # Create a new run record in the backend
        run_data = client.create_run(case_id, model=config.llm_model)
        run_id = run_data["id"]
        logger.info(f"Created agent run {run_id} for case {case_id}")
        # Mark run as running
        client.update_run(run_id, status="running")

    try:
        # Build and compile the graph
        graph = build_llm_graph() if use_llm else build_graph()
        app = graph.compile()

        # Execute
        initial_state: InvestigationState = {
            "case_id": case_id,
            "run_id": run_id,
        }
        start = time.time()
        final_state = app.invoke(initial_state)
        elapsed_ms = int((time.time() - start) * 1000)

        # Record steps in backend
        for step in final_state.get("steps", []):
            client.create_step(
                run_id,
                step_index=step["stepIndex"],
                action=step["action"],
                latencyMs=step.get("latencyMs", 0),
                input=step.get("input", "{}"),
                output=step.get("output", "{}"),
                reasoning=step.get("reasoning", ""),
            )

        # Write decision to backend (proposed only — admin must approve)
        verdict = final_state.get("verdict", "insufficient_evidence")
        reasoning = ""
        if final_state.get("report"):
            # Use first 2000 chars of report as reasoning
            reasoning = final_state["report"][:2000]

        # Build proposed actions based on verdict + case type
        proposed_actions = _build_proposed_actions(final_state)
        final_state["proposed_actions"] = proposed_actions

        client.create_decision(
            case_id,
            outcome=verdict,
            reasoning=reasoning,
            run_id=run_id,
            actions=proposed_actions,
            responsible_party=final_state.get("responsible_party", ""),
            policy_refs=final_state.get("policy_violations", []),
            confidence=final_state.get("confidence", 0.0),
        )

        # Mark completed
        client.update_run(
            run_id,
            status="completed",
            report=final_state.get("report", ""),
            stepCount=final_state.get("step_count", 0),
        )

        logger.info(f"Run {run_id} completed in {elapsed_ms}ms, {final_state.get('step_count', 0)} steps")
        return final_state

    except Exception as e:
        logger.exception(f"Run {run_id} failed: {e}")
        client.update_run(run_id, status="failed", errorMsg=str(e))
        return {"error": str(e), "run_id": run_id}

    finally:
        client.close()


# --- Proposed actions logic ---

# Maps (case_type, outcome) → list of actions to propose
_ACTION_MAP: dict[tuple[str, str], list[dict[str, Any]]] = {
    # Content report upheld → take down post + penalize author
    ("content_report", "upheld"): [
        {"action": "post_takedown", "reason": "内容违规，建议下架"},
        {"action": "credit_deduct", "amount": -5, "reason": "发布违规内容，建议扣除信用分"},
    ],
    # Content report rejected → no action (reporter was wrong)
    ("content_report", "rejected"): [],

    # Settlement dispute upheld → penalize the responsible party
    ("settlement_dispute", "upheld"): [
        {"action": "credit_deduct", "amount": -5, "reason": "结算纠纷裁定有过错，建议扣除信用分"},
    ],
    # Settlement dispute rejected → no action
    ("settlement_dispute", "rejected"): [],

    # Moderation appeal upheld → restore the post
    ("moderation_appeal", "upheld"): [
        {"action": "post_restore", "reason": "申诉成立，建议恢复帖子"},
    ],
    # Moderation appeal rejected → no action (original decision was correct)
    ("moderation_appeal", "rejected"): [],

    # Credit appeal upheld → restore credit
    ("credit_appeal", "upheld"): [
        {"action": "credit_restore", "reason": "信用分申诉成立，建议撤销扣分"},
    ],
    # Credit appeal rejected → no action
    ("credit_appeal", "rejected"): [],
}


def _build_proposed_actions(final_state: dict[str, Any]) -> list[dict[str, Any]]:
    """Build a list of proposed actions based on verdict + case type.

    These actions are NOT executed — they are written into the CaseDecision
    for admin review. The admin approval endpoint will execute them.
    """
    verdict = final_state.get("verdict", "insufficient_evidence")
    case_data = final_state.get("case_data", {})
    case_type = case_data.get("caseType", "")
    responsible_party = final_state.get("responsible_party", "")

    # Determine target for credit actions
    target_user_id = ""
    if responsible_party == "author":
        target_user_id = case_data.get("targetUserId", "")
    elif responsible_party == "participant":
        target_user_id = case_data.get("reporterUserId", "")
    elif responsible_party == "reporter":
        target_user_id = case_data.get("reporterUserId", "")

    actions_key = (case_type, verdict)
    planned_actions = _ACTION_MAP.get(actions_key, [])

    if not planned_actions:
        return []

    result = []
    for action_spec in planned_actions:
        action_name = action_spec["action"]
        entry: dict[str, Any] = {
            "action": action_name,
            "reason": action_spec.get("reason", ""),
        }
        if action_name in ("credit_deduct", "credit_restore"):
            entry["targetId"] = target_user_id
            if "amount" in action_spec:
                entry["amount"] = action_spec["amount"]
        # post_restore / post_takedown use the case's postID (backend default)
        result.append(entry)

    logger.info(f"Built {len(result)} proposed actions for ({case_type}, {verdict})")
    return result
