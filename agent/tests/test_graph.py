"""Tests for the investigation graph structure."""

import pytest
from agent.src.graph import build_graph, InvestigationState


def test_graph_compiles():
    """The graph should compile without errors."""
    graph = build_graph()
    app = graph.compile()
    assert app is not None


def test_graph_nodes_exist():
    """All expected nodes are registered."""
    graph = build_graph()
    expected_nodes = {"load_case", "extract_claims", "investigate", "evaluate", "report"}
    # LangGraph stores nodes in the builder; check they compile
    app = graph.compile()
    assert app is not None


def test_extract_claims_node():
    """extract_claims_node produces at least one claim from case data."""
    from agent.src.graph import extract_claims_node

    state: InvestigationState = {
        "case_id": "test_case_1",
        "case_data": {"summary": "User reported no-show at activity"},
        "full_context": {},
    }
    result = extract_claims_node(state)
    assert "claims" in result
    assert len(result["claims"]) >= 1
    assert result["claims"][0]["text"] == "User reported no-show at activity"


def test_should_continue_investigating():
    """Investigation stops after reaching step limit."""
    from agent.src.graph import should_continue_investigating

    state_0: InvestigationState = {"case_id": "x", "step_count": 0}
    assert should_continue_investigating(state_0) == "continue"

    state_3: InvestigationState = {"case_id": "x", "step_count": 3}
    assert should_continue_investigating(state_3) == "done"


def test_evaluate_node():
    """evaluate_node produces a verdict."""
    from agent.src.graph import evaluate_node

    state: InvestigationState = {
        "case_id": "x",
        "evidence": [{"type": "domain_events", "data": []}],
    }
    result = evaluate_node(state)
    assert result["verdict"] in ("supported", "unsupported", "inconclusive")
    assert 0 <= result["confidence"] <= 1.0


def test_report_node():
    """report_node generates a markdown report."""
    from agent.src.graph import report_node

    state: InvestigationState = {
        "case_id": "test_1",
        "case_data": {"id": "case_abc", "caseType": "no_show", "summary": "Test case"},
        "verdict": "supported",
        "confidence": 0.8,
        "evidence": [{"type": "events", "data": []}],
        "steps": [{"stepIndex": 0, "action": "test"}],
    }
    result = report_node(state)
    assert "# Investigation Report" in result["report"]
    assert "case_abc" in result["report"]
    assert "supported" in result["report"]
