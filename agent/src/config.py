"""Configuration for the agent service. Loads from .env file."""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path

from dotenv import load_dotenv

# Load .env from the agent/ directory
_env_path = Path(__file__).parent.parent / ".env"
load_dotenv(_env_path)


@dataclass(frozen=True)
class Config:
    """Agent service configuration, loaded from environment variables.

    LLM layer is OpenAI-compatible — works with DeepSeek, OpenAI, Anthropic
    (via proxy), local vLLM, etc.
    """

    # LLM (OpenAI-compatible)
    llm_api_key: str = os.getenv("LLM_API_KEY", "")
    llm_base_url: str = os.getenv("LLM_BASE_URL", "https://api.deepseek.com/v1")
    llm_model: str = os.getenv("LLM_MODEL", "deepseek-chat")
    llm_max_tokens: int = int(os.getenv("LLM_MAX_TOKENS", "4096"))
    llm_temperature: float = float(os.getenv("LLM_TEMPERATURE", "0"))

    # Go backend
    backend_url: str = os.getenv("AGENT_BACKEND_URL", "http://localhost:8080")
    agent_api_secret: str = os.getenv("AGENT_API_SECRET", "")

    # Agent behavior
    max_steps: int = int(os.getenv("AGENT_MAX_STEPS", "15"))
    port: int = int(os.getenv("AGENT_PORT", "8090"))
    log_level: str = os.getenv("AGENT_LOG_LEVEL", "INFO")


def load_config() -> Config:
    """Load config (re-reads env each time for testability)."""
    return Config(
        llm_api_key=os.getenv("LLM_API_KEY", ""),
        llm_base_url=os.getenv("LLM_BASE_URL", "https://api.deepseek.com/v1"),
        llm_model=os.getenv("LLM_MODEL", "deepseek-chat"),
        llm_max_tokens=int(os.getenv("LLM_MAX_TOKENS", "4096")),
        llm_temperature=float(os.getenv("LLM_TEMPERATURE", "0")),
        backend_url=os.getenv("AGENT_BACKEND_URL", "http://localhost:8080"),
        agent_api_secret=os.getenv("AGENT_API_SECRET", ""),
        max_steps=int(os.getenv("AGENT_MAX_STEPS", "15")),
        port=int(os.getenv("AGENT_PORT", "8090")),
        log_level=os.getenv("AGENT_LOG_LEVEL", "INFO"),
    )
