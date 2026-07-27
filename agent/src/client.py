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
