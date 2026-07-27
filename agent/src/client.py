"""HTTP client for the Go backend Agent Tool API."""

from __future__ import annotations

import httpx
from typing import Any

from .config import Config


class BackendClient:
    """Thin wrapper around the /internal/agent/* endpoints."""

    def __init__(self, config: Config):
        self._base = config.backend_url.rstrip("/")
        self._headers = {"Authorization": f"Bearer {config.agent_api_secret}"}
        self._client = httpx.Client(base_url=self._base, headers=self._headers, timeout=30)

    # --- Read tools ---

    def list_cases(self, source_ref: str = "", status: str = "", limit: int = 50) -> list[dict[str, Any]]:
        params = {"limit": limit}
        if source_ref:
            params["source_ref"] = source_ref
        if status:
            params["status"] = status
        r = self._client.get("/internal/agent/cases", params=params)
        r.raise_for_status()
        return r.json().get("cases", [])

    def get_case(self, case_id: str) -> dict[str, Any]:
        r = self._client.get(f"/internal/agent/case/{case_id}")
        r.raise_for_status()
        return r.json()

    def get_case_context(self, case_id: str) -> dict[str, Any]:
        r = self._client.get(f"/internal/agent/case/{case_id}/context")
        r.raise_for_status()
        return r.json()

    def get_domain_events(self, case_id: str) -> list[dict[str, Any]]:
        r = self._client.get(f"/internal/agent/case/{case_id}/events")
        r.raise_for_status()
        return r.json().get("events", [])

    def get_chat_messages(self, case_id: str) -> list[dict[str, Any]]:
        r = self._client.get(f"/internal/agent/case/{case_id}/messages")
        r.raise_for_status()
        return r.json().get("messages", [])

    def get_user_profile(self, user_id: str) -> dict[str, Any]:
        r = self._client.get(f"/internal/agent/user/{user_id}/profile")
        r.raise_for_status()
        return r.json()

    def get_user_history(self, user_id: str, limit: int = 20) -> dict[str, Any]:
        r = self._client.get(f"/internal/agent/user/{user_id}/history", params={"limit": limit})
        r.raise_for_status()
        return r.json()

    # --- Evidence layer tools ---

    def get_reports(self, case_id: str) -> list[dict[str, Any]]:
        r = self._client.get(f"/internal/agent/case/{case_id}/reports")
        r.raise_for_status()
        return r.json().get("reports", [])

    def get_case_evidence(self, case_id: str) -> list[dict[str, Any]]:
        r = self._client.get(f"/internal/agent/case/{case_id}/evidence")
        r.raise_for_status()
        return r.json().get("evidence", [])

    def get_case_decisions(self, case_id: str) -> list[dict[str, Any]]:
        r = self._client.get(f"/internal/agent/case/{case_id}/decisions")
        r.raise_for_status()
        return r.json().get("decisions", [])

    def get_content_snapshots(self, case_id: str) -> list[dict[str, Any]]:
        r = self._client.get(f"/internal/agent/case/{case_id}/snapshots")
        r.raise_for_status()
        return r.json().get("snapshots", [])

    def get_notifications(self, case_id: str) -> list[dict[str, Any]]:
        r = self._client.get(f"/internal/agent/case/{case_id}/notifications")
        r.raise_for_status()
        return r.json().get("notifications", [])

    def get_settlements(self, case_id: str) -> list[dict[str, Any]]:
        r = self._client.get(f"/internal/agent/case/{case_id}/settlements")
        r.raise_for_status()
        return r.json().get("settlements", [])

    def get_credit_ledger(self, case_id: str) -> list[dict[str, Any]]:
        r = self._client.get(f"/internal/agent/case/{case_id}/credit-ledger")
        r.raise_for_status()
        return r.json().get("ledgers", [])

    def get_policy(self, policy_id: str) -> str:
        """Returns the raw YAML content of a policy file."""
        r = self._client.get(f"/internal/agent/policy/{policy_id}")
        r.raise_for_status()
        return r.text

    def add_evidence(self, case_id: str, evidence_type: str, evidence_id: str,
                     relevance: str = "supporting", note: str = "", run_id: str = "") -> dict[str, Any]:
        payload = {
            "evidenceType": evidence_type,
            "evidenceId": evidence_id,
            "relevance": relevance,
            "note": note,
            "runId": run_id,
        }
        r = self._client.post(f"/internal/agent/case/{case_id}/evidence", json=payload)
        r.raise_for_status()
        return r.json()

    def create_decision(self, case_id: str, outcome: str, reasoning: str = "",
                        evidence_refs: list[str] | None = None,
                        actions: list[str] | None = None,
                        run_id: str = "") -> dict[str, Any]:
        """Record the agent's verdict as a CaseDecision."""
        payload = {
            "outcome": outcome,
            "reasoning": reasoning,
            "evidenceRefs": evidence_refs or [],
            "actions": actions or [],
            "runId": run_id,
        }
        r = self._client.post(f"/internal/agent/case/{case_id}/decision", json=payload)
        r.raise_for_status()
        return r.json()

    # --- Write tools (run tracking) ---

    def create_run(self, case_id: str, model: str) -> dict[str, Any]:
        r = self._client.post("/internal/agent/run", json={"caseId": case_id, "model": model})
        r.raise_for_status()
        return r.json()

    def update_run(self, run_id: str, **kwargs) -> dict[str, Any]:
        r = self._client.patch(f"/internal/agent/run/{run_id}", json=kwargs)
        r.raise_for_status()
        return r.json()

    def create_step(self, run_id: str, step_index: int, action: str, **kwargs) -> dict[str, Any]:
        payload = {"stepIndex": step_index, "action": action, **kwargs}
        r = self._client.post(f"/internal/agent/run/{run_id}/step", json=payload)
        r.raise_for_status()
        return r.json()

    def close(self):
        self._client.close()
