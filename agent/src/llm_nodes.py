"""LLM-powered node implementations for the investigation graph.

Uses OpenAI-compatible API (DeepSeek, OpenAI, vLLM, etc.).
Requires LLM_API_KEY to be set in .env.
"""

from __future__ import annotations

import json
import logging
import time
from typing import Any

from .config import Config, load_config
from .llm_client import LLMClient

logger = logging.getLogger(__name__)

# --- Prompts ---

EXTRACT_CLAIMS_SYSTEM = """你是一个案件调查分析员。你的任务是从案件描述中提取需要调查的核心主张(claims)。

每个claim应该包含:
- id: 唯一标识符 (claim_1, claim_2, ...)
- text: 主张的具体内容
- evidence_needed: 需要什么证据来验证这个主张
- status: 初始为 "pending"

返回JSON格式:
{"claims": [{"id": "claim_1", "text": "...", "evidence_needed": "...", "status": "pending"}, ...]}

只返回JSON，不要其他文字。"""

INVESTIGATE_SYSTEM = """你是一个案件调查员。根据当前掌握的证据和待验证的主张，决定下一步应该使用哪个调查工具。

可用工具:
- get_domain_events: 获取案件相关的领域事件时间线
- get_chat_messages: 获取活动群聊记录
- get_user_profile: 获取目标用户的基本信息和信用分
- get_user_history: 获取用户的历史活动和案件记录
- done: 证据已充分，可以进入评估阶段

根据以下信息决定下一步:
1. 还有哪些claim没有足够证据
2. 已经收集了哪些证据
3. 哪个工具最可能提供有用信息

返回JSON格式:
{"action": "tool_name", "reasoning": "选择这个工具的原因"}

只返回JSON，不要其他文字。"""

EVALUATE_SYSTEM = """你是一个案件裁决分析员。根据收集到的所有证据，对案件做出评估。

对每个claim进行判断:
- supported: 证据支持该主张成立
- unsupported: 证据不支持该主张
- inconclusive: 证据不足以做出判断

然后给出案件整体裁决:
- supported: 举报/申诉有充分依据
- unsupported: 举报/申诉缺乏依据
- inconclusive: 证据不足，需要人工进一步审查

返回JSON格式:
{
  "claim_evaluations": [{"id": "claim_1", "status": "supported|unsupported|inconclusive", "reasoning": "..."}],
  "verdict": "supported|unsupported|inconclusive",
  "confidence": 0.0-1.0,
  "key_findings": ["关键发现1", "关键发现2"]
}

只返回JSON，不要其他文字。"""

REPORT_SYSTEM = """你是一个案件报告撰写员。根据调查结果生成结构化的调查报告。

报告应包含:
1. 案件概述
2. 调查过程摘要
3. 关键证据
4. 各主张的评估结果
5. 整体结论和建议

使用Markdown格式，保持专业、客观、简洁。报告面向管理员审阅。"""


# --- Helpers ---

def _format_context_for_llm(case_data: dict, full_context: dict) -> str:
    """Format case context into a readable string for the LLM."""
    parts = []
    parts.append(f"## 案件信息\n- ID: {case_data.get('id')}\n- 类型: {case_data.get('caseType')}")
    parts.append(f"- 摘要: {case_data.get('summary', '')}")
    parts.append(f"- 详情: {case_data.get('description', '')}")

    if full_context.get("post"):
        post = full_context["post"]
        parts.append(f"\n## 活动信息\n- 标题: {post.get('title')}")
        parts.append(f"- 描述: {post.get('description')}")
        parts.append(f"- 状态: {post.get('status')}")
        parts.append(f"- 人数: {post.get('currentCount')}/{post.get('maxCount')}")

    return "\n".join(parts)


def _format_evidence_for_llm(evidence: list[dict]) -> str:
    """Format collected evidence into a readable string."""
    if not evidence:
        return "尚未收集任何证据。"

    parts = []
    for i, e in enumerate(evidence, 1):
        etype = e.get("type", "unknown")
        data = e.get("data", [])
        if isinstance(data, list):
            parts.append(f"\n### 证据{i}: {etype} ({len(data)}条记录)")
            for item in data[:10]:
                parts.append(f"  - {json.dumps(item, ensure_ascii=False)[:200]}")
            if len(data) > 10:
                parts.append(f"  ... 还有{len(data) - 10}条记录")
        else:
            parts.append(f"\n### 证据{i}: {etype}")
            parts.append(f"  {json.dumps(data, ensure_ascii=False)[:500]}")

    return "\n".join(parts)


# --- LLM-powered nodes ---

def extract_claims_llm(state: dict[str, Any], config: Config | None = None) -> dict[str, Any]:
    """Use LLM to extract investigation claims from case data."""
    if config is None:
        config = load_config()

    case_data = state.get("case_data", {})
    full_context = state.get("full_context", {})
    context_str = _format_context_for_llm(case_data, full_context)

    llm = LLMClient(config)
    result = llm.chat_json(
        EXTRACT_CLAIMS_SYSTEM,
        f"请分析以下案件并提取需要调查的主张:\n\n{context_str}",
    )

    claims = result.get("claims", [])
    if not claims:
        # Fallback
        summary = case_data.get("summary", "") or case_data.get("description", "")
        claims = [{"id": "claim_1", "text": summary, "evidence_needed": "all", "status": "pending"}]

    return {"claims": claims}


def investigate_llm(state: dict[str, Any], config: Config | None = None) -> dict[str, Any]:
    """Use LLM to decide next investigation action and execute it."""
    if config is None:
        config = load_config()

    from .client import BackendClient

    step_count = state.get("step_count", 0)
    evidence = list(state.get("evidence", []))
    steps = list(state.get("steps", []))
    claims = state.get("claims", [])
    case_data = state.get("case_data", {})
    case_id = state["case_id"]

    # Ask LLM what tool to use
    llm = LLMClient(config)
    evidence_str = _format_evidence_for_llm(evidence)
    claims_str = json.dumps(claims, ensure_ascii=False, indent=2)

    result = llm.chat_json(
        INVESTIGATE_SYSTEM,
        (
            f"待验证主张:\n{claims_str}\n\n"
            f"已收集证据:\n{evidence_str}\n\n"
            f"已执行步骤数: {step_count}/{config.max_steps}\n"
            f"请决定下一步行动。"
        ),
    )

    action = result.get("action", "done")
    reasoning = result.get("reasoning", "")

    if action == "done":
        steps.append({"stepIndex": step_count, "action": "done", "latencyMs": 0, "reasoning": reasoning})
        return {"evidence": evidence, "steps": steps, "step_count": step_count + 1, "_done": True}

    # Execute the chosen tool
    client = BackendClient(config)
    start = time.time()
    try:
        if action == "get_domain_events":
            data = client.get_domain_events(case_id)
            evidence.append({"type": "domain_events", "data": data})
        elif action == "get_chat_messages":
            data = client.get_chat_messages(case_id)
            evidence.append({"type": "chat_messages", "data": data})
        elif action == "get_user_profile":
            target_id = case_data.get("targetUserId", "")
            if target_id:
                data = client.get_user_profile(target_id)
                evidence.append({"type": "user_profile", "data": data})
        elif action == "get_user_history":
            target_id = case_data.get("targetUserId", "")
            if target_id:
                data = client.get_user_history(target_id)
                evidence.append({"type": "user_history", "data": data})
        else:
            logger.warning(f"Unknown action: {action}, treating as done")
            action = "done"
    finally:
        client.close()

    latency_ms = int((time.time() - start) * 1000)
    steps.append({"stepIndex": step_count, "action": action, "latencyMs": latency_ms, "reasoning": reasoning})

    return {"evidence": evidence, "steps": steps, "step_count": step_count + 1}


def should_continue_llm(state: dict[str, Any]) -> str:
    """Check if LLM signaled done or we hit max steps."""
    from .config import load_config
    config = load_config()

    if state.get("_done"):
        return "done"
    if state.get("step_count", 0) >= config.max_steps:
        return "done"
    return "continue"


def evaluate_llm(state: dict[str, Any], config: Config | None = None) -> dict[str, Any]:
    """Use LLM to evaluate evidence and produce a verdict."""
    if config is None:
        config = load_config()

    case_data = state.get("case_data", {})
    full_context = state.get("full_context", {})
    claims = state.get("claims", [])
    evidence = state.get("evidence", [])

    llm = LLMClient(config)
    context_str = _format_context_for_llm(case_data, full_context)
    evidence_str = _format_evidence_for_llm(evidence)
    claims_str = json.dumps(claims, ensure_ascii=False, indent=2)

    result = llm.chat_json(
        EVALUATE_SYSTEM,
        (
            f"## 案件背景\n{context_str}\n\n"
            f"## 待评估主张\n{claims_str}\n\n"
            f"## 收集到的证据\n{evidence_str}\n\n"
            f"请对以上证据进行综合评估。"
        ),
    )

    verdict = result.get("verdict", "inconclusive")
    if verdict not in ("supported", "unsupported", "inconclusive"):
        verdict = "inconclusive"
    confidence = float(result.get("confidence", 0.5))
    key_findings = result.get("key_findings", [])

    return {
        "verdict": verdict,
        "confidence": min(max(confidence, 0.0), 1.0),
        "_key_findings": key_findings,
    }


def report_llm(state: dict[str, Any], config: Config | None = None) -> dict[str, Any]:
    """Use LLM to generate a structured investigation report."""
    if config is None:
        config = load_config()

    case_data = state.get("case_data", {})
    full_context = state.get("full_context", {})
    evidence = state.get("evidence", [])
    verdict = state.get("verdict", "inconclusive")
    confidence = state.get("confidence", 0.0)
    steps = state.get("steps", [])
    key_findings = state.get("_key_findings", [])

    llm = LLMClient(config)
    context_str = _format_context_for_llm(case_data, full_context)
    evidence_str = _format_evidence_for_llm(evidence)

    report = llm.chat(
        REPORT_SYSTEM,
        (
            f"## 案件背景\n{context_str}\n\n"
            f"## 调查步骤\n共执行 {len(steps)} 步调查\n\n"
            f"## 收集到的证据\n{evidence_str}\n\n"
            f"## 评估结果\n- 裁决: {verdict}\n- 置信度: {confidence:.0%}\n"
            f"- 关键发现: {json.dumps(key_findings, ensure_ascii=False)}\n\n"
            f"请生成完整的调查报告。"
        ),
    )

    return {"report": report}
