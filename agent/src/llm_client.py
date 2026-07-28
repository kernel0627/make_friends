"""LLM client — OpenAI-compatible, works with DeepSeek/OpenAI/vLLM/etc."""

from __future__ import annotations

import json
import logging
import time
from typing import Any

from openai import OpenAI

from .config import Config

logger = logging.getLogger(__name__)


class LLMClient:
    """OpenAI-compatible chat completion client.

    Supports any provider that implements the /v1/chat/completions API:
    DeepSeek, OpenAI, Anthropic (via proxy), local vLLM, Ollama, etc.
    """

    def __init__(self, config: Config):
        self._client = OpenAI(
            api_key=config.llm_api_key,
            base_url=config.llm_base_url,
        )
        self._model = config.llm_model
        self._max_tokens = config.llm_max_tokens
        self._temperature = config.llm_temperature

    def chat(self, system: str, user: str) -> str:
        """Send a chat completion request. Returns the assistant content."""
        start = time.time()
        response = self._client.chat.completions.create(
            model=self._model,
            messages=[
                {"role": "system", "content": system},
                {"role": "user", "content": user},
            ],
            max_tokens=self._max_tokens,
            temperature=self._temperature,
        )
        latency_ms = int((time.time() - start) * 1000)
        content = response.choices[0].message.content or ""
        usage = response.usage
        _log_llm_call(self._model, latency_ms, usage, len(content))
        return content

    def chat_json(self, system: str, user: str) -> dict[str, Any]:
        """Send a chat completion and parse the response as JSON.

        Tries response_format=json_object first (supported by DeepSeek/OpenAI).
        Falls back to plain parsing if the provider doesn't support it.
        """
        start = time.time()
        try:
            response = self._client.chat.completions.create(
                model=self._model,
                messages=[
                    {"role": "system", "content": system},
                    {"role": "user", "content": user},
                ],
                max_tokens=self._max_tokens,
                temperature=self._temperature,
                response_format={"type": "json_object"},
            )
            content = response.choices[0].message.content or "{}"
            usage = response.usage
            latency_ms = int((time.time() - start) * 1000)
            _log_llm_call(self._model, latency_ms, usage, len(content))
        except Exception:
            # Fallback: some providers don't support response_format
            logger.debug("response_format not supported, falling back to plain mode")
            content = self.chat(system, user)

        return _parse_json_response(content)


def _log_llm_call(model: str, latency_ms: int, usage: Any, content_len: int) -> None:
    """Emit a structured log line for every LLM call."""
    tokens_in = getattr(usage, "prompt_tokens", 0) if usage else 0
    tokens_out = getattr(usage, "completion_tokens", 0) if usage else 0
    logger.info(
        "llm_call",
        extra={
            "model": model,
            "latency_ms": latency_ms,
            "tokens_in": tokens_in,
            "tokens_out": tokens_out,
            "content_len": content_len,
        },
    )


def _parse_json_response(content: str) -> dict[str, Any]:
    """Extract JSON from LLM response, handling markdown fences."""
    content = content.strip()
    # Strip markdown code fences if present
    if content.startswith("```"):
        lines = content.split("\n")
        # Remove first and last fence lines
        if lines[0].startswith("```"):
            lines = lines[1:]
        if lines and lines[-1].strip() == "```":
            lines = lines[:-1]
        content = "\n".join(lines).strip()

    try:
        return json.loads(content)
    except json.JSONDecodeError:
        logger.warning(f"Failed to parse JSON from LLM response: {content[:200]}")
        return {}
