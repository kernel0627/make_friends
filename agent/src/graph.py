"""LangGraph investigation graph for case review.

Graph structure:
  load_case → extract_claims → investigate_loop → evaluate_evidence → generate_report

The investigate_loop dynamically picks tools until it has enough evidence or
hits the step limit.
"""

from __future__ import annotations

import time
from typing import Any, TypedDict

from langgraph.graph import StateGraph, END


class InvestigationState(TypedDict, total=False):
    """State passed through the investigation graph."""

    # Input
    case_id: str
    run_id: str

    # Loaded context
    case_data: dict[str, Any]
    full_context: dict[str, Any]

    # Investigation
    claims: list[dict[str, Any]]
    evidence: list[dict[str, Any]]
    steps: list[dict[str, Any]]
    step_count: int

    # Output
    verdict: str  # "supported" | "unsupported" | "inconclusive"
    confidence: float
    report: str
    error: str | None

    # Internal (LLM mode)
    _done: bool
    _key_findings: list[str]


# PLACEHOLDER_GRAPH_NODES


# --- Node implementations ---

def load_case_node(state: InvestigationState) -> dict[str, Any]:
    """Load case data and full context from the backend."""
    from .client import BackendClient
    from .config import load_config

    config = load_config()
    client = BackendClient(config)
    try:
        case_data = client.get_case(state["case_id"])
        full_context = client.get_case_context(state["case_id"])
    finally:
        client.close()

    return {
        "case_data": case_data,
        "full_context": full_context,
        "steps": [],
        "evidence": [],
        "step_count": 0,
    }


def extract_claims_node(state: InvestigationState) -> dict[str, Any]:
    """Extract investigation claims from the case description.

    In the full implementation (Phase 3) this will use an LLM to parse
    the case into structured claims. For now, create a single claim
    from the case summary.
    """
    case_data = state.get("case_data", {})
    summary = case_data.get("summary", "") or case_data.get("description", "")
    claims = [{"id": "claim_1", "text": summary, "status": "pending"}]
    return {"claims": claims}


def investigate_node(state: InvestigationState) -> dict[str, Any]:
    """One step of investigation — pick a tool and gather evidence.

    Phase 3 will use LLM-based tool selection. For now, perform a
    deterministic sequence: domain events → chat messages → user history.
    """
    from .client import BackendClient
    from .config import load_config

    step_count = state.get("step_count", 0)
    evidence = list(state.get("evidence", []))
    steps = list(state.get("steps", []))
    case_id = state["case_id"]

    config = load_config()
    client = BackendClient(config)
    start = time.time()

    try:
        if step_count == 0:
            # Step 0: gather domain events
            events = client.get_domain_events(case_id)
            evidence.append({"type": "domain_events", "data": events})
            action = "get_domain_events"
        elif step_count == 1:
            # Step 1: gather chat messages
            messages = client.get_chat_messages(case_id)
            evidence.append({"type": "chat_messages", "data": messages})
            action = "get_chat_messages"
        else:
            # Step 2: gather target user history
            target_id = state.get("case_data", {}).get("targetUserId", "")
            if target_id:
                history = client.get_user_history(target_id)
                evidence.append({"type": "user_history", "data": history})
            action = "get_user_history"
    finally:
        client.close()

    latency_ms = int((time.time() - start) * 1000)
    steps.append({"stepIndex": step_count, "action": action, "latencyMs": latency_ms})

    return {
        "evidence": evidence,
        "steps": steps,
        "step_count": step_count + 1,
    }


def should_continue_investigating(state: InvestigationState) -> str:
    """Decide whether to continue investigating or move to evaluation."""
    from .config import load_config

    config = load_config()
    step_count = state.get("step_count", 0)
    # For the skeleton: 3 deterministic steps then done
    if step_count >= min(3, config.max_steps):
        return "done"
    return "continue"


def evaluate_node(state: InvestigationState) -> dict[str, Any]:
    """Evaluate gathered evidence against claims.

    Phase 3 will use LLM reasoning. For now, produce a placeholder verdict.
    """
    evidence_count = len(state.get("evidence", []))
    return {
        "verdict": "inconclusive",
        "confidence": 0.5,
    }


def report_node(state: InvestigationState) -> dict[str, Any]:
    """Generate the final investigation report.

    Phase 3 will use an LLM to write a structured report.
    For now, produce a template-based summary.
    """
    case_data = state.get("case_data", {})
    verdict = state.get("verdict", "inconclusive")
    confidence = state.get("confidence", 0.0)
    evidence = state.get("evidence", [])
    steps = state.get("steps", [])

    report = (
        f"# Investigation Report\n\n"
        f"**Case:** {case_data.get('id', 'unknown')}\n"
        f"**Type:** {case_data.get('caseType', 'unknown')}\n"
        f"**Verdict:** {verdict} (confidence: {confidence:.0%})\n\n"
        f"## Evidence Gathered\n\n"
        f"- {len(evidence)} evidence sources collected\n"
        f"- {len(steps)} investigation steps taken\n\n"
        f"## Summary\n\n"
        f"{case_data.get('summary', 'No summary available.')}\n\n"
        f"---\n*Generated by Activity Case Review Agent (skeleton mode)*\n"
    )
    return {"report": report}


def build_graph() -> StateGraph:
    """Construct the investigation graph (compile-ready)."""
    graph = StateGraph(InvestigationState)

    graph.add_node("load_case", load_case_node)
    graph.add_node("extract_claims", extract_claims_node)
    graph.add_node("investigate", investigate_node)
    graph.add_node("evaluate", evaluate_node)
    graph.add_node("report", report_node)

    graph.set_entry_point("load_case")
    graph.add_edge("load_case", "extract_claims")
    graph.add_edge("extract_claims", "investigate")
    graph.add_conditional_edges(
        "investigate",
        should_continue_investigating,
        {"continue": "investigate", "done": "evaluate"},
    )
    graph.add_edge("evaluate", "report")
    graph.add_edge("report", END)

    return graph


def build_llm_graph() -> StateGraph:
    """Construct the LLM-powered investigation graph.

    Uses Claude for claims extraction, tool selection, evidence evaluation,
    and report generation. Falls back to skeleton nodes for load_case (always
    deterministic).
    """
    from .llm_nodes import (
        extract_claims_llm,
        investigate_llm,
        should_continue_llm,
        evaluate_llm,
        report_llm,
    )

    graph = StateGraph(InvestigationState)

    graph.add_node("load_case", load_case_node)
    graph.add_node("extract_claims", extract_claims_llm)
    graph.add_node("investigate", investigate_llm)
    graph.add_node("evaluate", evaluate_llm)
    graph.add_node("report", report_llm)

    graph.set_entry_point("load_case")
    graph.add_edge("load_case", "extract_claims")
    graph.add_edge("extract_claims", "investigate")
    graph.add_conditional_edges(
        "investigate",
        should_continue_llm,
        {"continue": "investigate", "done": "evaluate"},
    )
    graph.add_edge("evaluate", "report")
    graph.add_edge("report", END)

    return graph
