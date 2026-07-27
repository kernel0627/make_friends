"""Test loader and validator for golden cases.

This module:
1. Loads all golden case JSON fixtures
2. Validates their structure
3. Provides parametrized test cases for evaluation
"""

import json
import pathlib
from typing import Any

import pytest

GOLDEN_DIR = pathlib.Path(__file__).parent / "golden_cases"
CASE_TYPES = ["content_report", "settlement_dispute", "moderation_appeal", "credit_appeal"]
VALID_VERDICTS = {"supported", "unsupported", "inconclusive"}
VALID_DIFFICULTIES = {"easy", "medium", "hard"}


def load_all_golden_cases() -> list[dict[str, Any]]:
    """Load all golden cases from JSON fixtures."""
    all_cases = []
    for case_type in CASE_TYPES:
        path = GOLDEN_DIR / f"{case_type}.json"
        if path.exists():
            data = json.loads(path.read_text())
            all_cases.extend(data["cases"])
    return all_cases


def load_golden_cases_by_type(case_type: str) -> list[dict[str, Any]]:
    """Load golden cases for a specific case type."""
    path = GOLDEN_DIR / f"{case_type}.json"
    if not path.exists():
        return []
    data = json.loads(path.read_text())
    return data["cases"]


# --- Structure validation tests ---


ALL_CASES = load_all_golden_cases()


def test_golden_cases_count():
    """We should have 20 golden cases (5 per type)."""
    assert len(ALL_CASES) == 20, f"Expected 20 cases, got {len(ALL_CASES)}"


@pytest.mark.parametrize("case_type", CASE_TYPES)
def test_five_cases_per_type(case_type: str):
    """Each case type should have exactly 5 golden cases."""
    cases = load_golden_cases_by_type(case_type)
    assert len(cases) == 5, f"{case_type}: expected 5 cases, got {len(cases)}"


@pytest.mark.parametrize("case", ALL_CASES, ids=[c["id"] for c in ALL_CASES])
def test_golden_case_structure(case: dict):
    """Each golden case has the required fields."""
    required_fields = ["id", "case_type", "difficulty", "description",
                       "expected_verdict", "expected_evidence_types", "case_input"]
    for field in required_fields:
        assert field in case, f"Case {case.get('id', '?')} missing field: {field}"

    assert case["expected_verdict"] in VALID_VERDICTS, \
        f"Case {case['id']}: invalid verdict '{case['expected_verdict']}'"
    assert case["difficulty"] in VALID_DIFFICULTIES, \
        f"Case {case['id']}: invalid difficulty '{case['difficulty']}'"
    assert case["case_type"] in CASE_TYPES, \
        f"Case {case['id']}: invalid case_type '{case['case_type']}'"


@pytest.mark.parametrize("case", ALL_CASES, ids=[c["id"] for c in ALL_CASES])
def test_golden_case_input_has_case(case: dict):
    """Each case_input must contain at minimum a 'case' object."""
    case_input = case["case_input"]
    assert "case" in case_input, f"Case {case['id']}: case_input missing 'case'"
    inner = case_input["case"]
    assert "id" in inner
    assert "caseType" in inner
    assert "status" in inner


def test_difficulty_distribution():
    """Golden cases should cover all difficulty levels."""
    difficulties = {c["difficulty"] for c in ALL_CASES}
    assert difficulties == VALID_DIFFICULTIES


def test_verdict_distribution():
    """Golden cases should cover all verdict types."""
    verdicts = {c["expected_verdict"] for c in ALL_CASES}
    assert verdicts == VALID_VERDICTS


def test_unique_ids():
    """All golden case IDs should be unique."""
    ids = [c["id"] for c in ALL_CASES]
    assert len(ids) == len(set(ids)), f"Duplicate IDs found: {[x for x in ids if ids.count(x) > 1]}"
