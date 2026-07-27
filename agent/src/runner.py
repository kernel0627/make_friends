"""Runner — orchestrates a full investigation run."""

from __future__ import annotations

import logging
import time
from typing import Any

from .client import BackendClient
from .config import Config, load_config
from .graph import InvestigationState, build_graph

logger = logging.getLogger(__name__)


def run_investigation(case_id: str, config: Config | None = None) -> dict[str, Any]:
    """Execute a full investigation for the given case.

    Returns the final state dict including the report.
    """
    if config is None:
        config = load_config()

    client = BackendClient(config)

    # Create a run record in the backend
    run_data = client.create_run(case_id, model=config.llm_model)
    run_id = run_data["id"]
    logger.info(f"Created agent run {run_id} for case {case_id}")

    # Mark run as running
    client.update_run(run_id, status="running")

    try:
        # Build and compile the graph
        graph = build_graph()
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
