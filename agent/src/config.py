"""Configuration and environment for the agent service."""

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Config:
    """Agent service configuration, loaded from environment."""

    # Go backend connection
    backend_url: str = os.getenv("AGENT_BACKEND_URL", "http://localhost:8080")
    agent_api_secret: str = os.getenv("AGENT_API_SECRET", "")

    # LLM
    llm_model: str = os.getenv("AGENT_LLM_MODEL", "claude-sonnet-4-20250514")
    llm_api_key: str = os.getenv("ANTHROPIC_API_KEY", "")
    max_steps: int = int(os.getenv("AGENT_MAX_STEPS", "15"))
    max_tokens_per_step: int = int(os.getenv("AGENT_MAX_TOKENS_PER_STEP", "4096"))

    # Service
    port: int = int(os.getenv("AGENT_PORT", "8090"))
    log_level: str = os.getenv("AGENT_LOG_LEVEL", "INFO")


def load_config() -> Config:
    return Config()
