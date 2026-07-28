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
- get_domain_events: 获取案件相关的领域事件时间线（帖子创建、修改、参与、结算等）
- get_chat_messages: 获取活动群聊记录
- get_user_profile: 获取目标用户的基本信息和信用分
- get_user_history: 获取用户的历史活动和案件记录
- get_content_snapshots: 获取帖子内容的历史快照（内容修改前后对比）
- get_notifications: 获取活动相关的通知记录（谁收到了通知、是否已读）
- get_settlements: 获取活动的结算/确认记录
- get_credit_ledger: 获取信用分变动记录
- get_reports: 获取关联的举报记录
- get_policy: 获取相关政策内容（需指定policy_id: content_commercial / content_off_platform / settlement_no_show / settlement_material_change / credit_reversal）
- done: 证据已充分，可以进入评估阶段

根据以下信息决定下一步:
1. 还有哪些claim没有足够证据
2. 已经收集了哪些证据
3. 哪个工具最可能提供有用信息

返回JSON格式:
{"action": "tool_name", "params": {"key": "value"}, "reasoning": "选择这个工具的原因"}

params是可选的，仅在需要指定参数时使用（如get_policy需要policy_id，get_user_profile需要user_id）。
只返回JSON，不要其他文字。"""

EVALUATE_SYSTEM = """你是一个案件裁决分析员。根据收集到的所有证据，对案件做出公正评估。

## 裁决规则

### outcome 的含义（取决于案件类型）:
- **content_report（内容举报）**:
  - upheld = 举报成立，帖子确实违规，应处罚发帖人
  - rejected = 举报不成立，帖子没有问题
- **settlement_dispute（结算纠纷）**:
  - upheld = 纠纷成立，投诉方的诉求合理（被投诉方有过错）
  - rejected = 纠纷不成立，投诉方的诉求不合理
- **moderation_appeal（审核申诉）**:
  - upheld = 申诉成立，原审核决定有误，应恢复帖子
  - rejected = 申诉不成立，原审核决定正确，维持驳回
- **credit_appeal（信用分申诉）**:
  - upheld = 申诉成立，扣分不合理，应撤销处罚
  - rejected = 申诉不成立，扣分合理，维持处罚
- **insufficient_evidence** = 证据不足，需要人工进一步审查（仅在确实无法判断时使用）

### 关键判断原则:
1. **实质性变更**：活动地点、时间、费用、性质的重大改变属于实质性变更。仅推迟30分钟以内且提前通知了不算实质性变更。
2. **站外引流**：明确给出外部群号/链接/联系方式并引导用户离开平台 = 违规。仅在平台内协调不违规。
3. **商业行为**：收费、发外部支付链接、留商业联系方式 = 商业推广。免费互助学习不是商业行为。
4. **提前通知**：如果参与者在活动前明确告知无法参加（有聊天记录为证），即使未在系统内取消，也不应被判定为"放鸽子"。
5. **通知送达**：修改活动后发了通知但对方未读/未收到，不能完全怪对方没看。
6. **系统时间戳优先**：判断取消/变更是否满足时间要求时，以系统记录的操作时间为准。聊天里说了不等于系统操作已完成。

## 输出格式

对每个claim进行判断后给出整体裁决:

```json
{
  "claim_evaluations": [{"id": "claim_1", "status": "supported|unsupported|inconclusive", "reasoning": "..."}],
  "outcome": "upheld|rejected|insufficient_evidence",
  "responsible_party": "author|participant|reporter|none",
  "policy_violations": ["content_commercial", "content_off_platform", "settlement_no_show", "settlement_material_change", "credit_reversal"],
  "confidence": 0.0-1.0,
  "key_findings": ["关键发现1", "关键发现2"]
}
```

policy_violations只填确实违反的政策ID，没有违反就填空数组。
responsible_party填实际有过错的一方角色，无过错填"none"。
只返回JSON，不要其他文字。"""

REPORT_SYSTEM = """你是一个案件报告撰写员。根据调查结果生成结构化的调查报告。

报告应包含:
1. 案件概述
2. 调查过程摘要
3. 关键证据
4. 各主张的评估结果
5. 整体结论和建议

使用Markdown格式，保持专业、客观、简洁。报告面向管理员审阅。"""

SUMMARIZE_SYSTEM = """你是一个证据摘要分析员。你需要将大量的原始证据压缩为结构化摘要，保留所有对裁决有影响的关键信息。

对每类证据，提取：
1. 关键事实（时间、人物、行为）
2. 矛盾点或异常
3. 与案件主张的相关性

重要：必须保留所有精确数据，包括：
- 具体时间戳（精确到分钟）
- 金额数字
- 用户ID
- 帖子/消息ID
- 状态变更的先后顺序

保持客观，不做判断。用简洁的中文输出。"""


# --- Helpers ---

_MAX_OUTPUT_LEN = 4000  # Max chars stored in step output (avoid bloating DB)


def _truncate_output(data: Any) -> str:
    """Serialize data to JSON, truncating if too long."""
    raw = json.dumps(data, ensure_ascii=False)
    if len(raw) > _MAX_OUTPUT_LEN:
        return raw[:_MAX_OUTPUT_LEN] + "...(truncated)"
    return raw


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
            f"案件类型: {case_data.get('caseType', '')}\n"
            f"请决定下一步行动。"
        ),
    )

    action = result.get("action", "done")
    reasoning = result.get("reasoning", "")
    params = result.get("params", {})

    if action == "done":
        steps.append({"stepIndex": step_count, "action": "done", "latencyMs": 0, "reasoning": reasoning, "input": "{}", "output": "{}"})
        return {"evidence": evidence, "steps": steps, "step_count": step_count + 1, "_done": True}

    # Execute the chosen tool
    client = BackendClient(config)
    start = time.time()
    tool_error = ""
    tool_input = json.dumps(params, ensure_ascii=False) if params else "{}"
    tool_output = ""
    try:
        if action == "get_domain_events":
            data = client.get_domain_events(case_id)
            evidence.append({"type": "domain_events", "data": data})
            tool_output = _truncate_output(data)
        elif action == "get_chat_messages":
            data = client.get_chat_messages(case_id)
            evidence.append({"type": "chat_messages", "data": data})
            tool_output = _truncate_output(data)
        elif action == "get_user_profile":
            target_id = params.get("user_id") or case_data.get("targetUserId", "")
            if target_id:
                data = client.get_user_profile(target_id)
                evidence.append({"type": "user_profile", "data": data})
                tool_output = _truncate_output(data)
        elif action == "get_user_history":
            target_id = params.get("user_id") or case_data.get("targetUserId", "")
            if target_id:
                data = client.get_user_history(target_id)
                evidence.append({"type": "user_history", "data": data})
                tool_output = _truncate_output(data)
        elif action == "get_content_snapshots":
            data = client.get_content_snapshots(case_id)
            evidence.append({"type": "content_snapshots", "data": data})
            tool_output = _truncate_output(data)
        elif action == "get_notifications":
            data = client.get_notifications(case_id)
            evidence.append({"type": "notifications", "data": data})
            tool_output = _truncate_output(data)
        elif action == "get_settlements":
            data = client.get_settlements(case_id)
            evidence.append({"type": "settlements", "data": data})
            tool_output = _truncate_output(data)
        elif action == "get_credit_ledger":
            data = client.get_credit_ledger(case_id)
            evidence.append({"type": "credit_ledger", "data": data})
            tool_output = _truncate_output(data)
        elif action == "get_reports":
            data = client.get_reports(case_id)
            evidence.append({"type": "reports", "data": data})
            tool_output = _truncate_output(data)
        elif action == "get_policy":
            policy_id = params.get("policy_id", "")
            if policy_id:
                data = client.get_policy(policy_id)
                evidence.append({"type": f"policy:{policy_id}", "data": data})
                tool_output = _truncate_output(data)
        else:
            logger.warning(f"Unknown action: {action}, treating as done")
            action = "done"
    except Exception as e:
        tool_error = str(e)
        logger.warning(f"Tool {action} failed: {e}")
        evidence.append({"type": f"error:{action}", "data": tool_error})
        tool_output = json.dumps({"error": tool_error})
    finally:
        client.close()

    latency_ms = int((time.time() - start) * 1000)
    step_record = {
        "stepIndex": step_count,
        "action": action,
        "latencyMs": latency_ms,
        "reasoning": reasoning,
        "input": tool_input,
        "output": tool_output or "{}",
    }
    if tool_error:
        step_record["error"] = tool_error
    steps.append(step_record)

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
            f"## 案件类型: {case_data.get('caseType', 'unknown')}\n\n"
            f"## 待评估主张\n{claims_str}\n\n"
            f"## 收集到的证据\n{evidence_str}\n\n"
            f"请根据案件类型的裁决规则，对以上证据进行综合评估。"
        ),
    )

    verdict = result.get("outcome", result.get("verdict", "insufficient_evidence"))
    if verdict not in ("upheld", "rejected", "insufficient_evidence"):
        # Map old format to new
        if verdict == "supported":
            verdict = "upheld"
        elif verdict == "unsupported":
            verdict = "rejected"
        elif verdict == "inconclusive":
            verdict = "insufficient_evidence"
        else:
            verdict = "insufficient_evidence"
    confidence = float(result.get("confidence", 0.5))
    key_findings = result.get("key_findings", [])
    responsible_party = result.get("responsible_party", "")
    policy_violations = result.get("policy_violations", [])

    return {
        "verdict": verdict,
        "confidence": min(max(confidence, 0.0), 1.0),
        "responsible_party": responsible_party,
        "policy_violations": policy_violations,
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


# --- Evidence Summarization ---

_EVIDENCE_CHAR_THRESHOLD = 16000  # Summarize if total evidence exceeds this


def _evidence_total_chars(evidence: list[dict]) -> int:
    """Calculate total character length of serialized evidence."""
    total = 0
    for e in evidence:
        data = e.get("data", [])
        total += len(json.dumps(data, ensure_ascii=False))
    return total


def summarize_evidence_llm(state: dict[str, Any], config: Config | None = None) -> dict[str, Any]:
    """Summarize evidence if it exceeds the token threshold.

    This node is inserted between investigate and evaluate. If evidence is
    within limits, it passes through unchanged. If it exceeds the threshold,
    each evidence source is compressed into a key-point summary.
    """
    if config is None:
        config = load_config()

    evidence = state.get("evidence", [])
    total_chars = _evidence_total_chars(evidence)

    if total_chars <= _EVIDENCE_CHAR_THRESHOLD:
        # No summarization needed
        return {}

    logger.info(f"Evidence exceeds threshold ({total_chars} > {_EVIDENCE_CHAR_THRESHOLD}), summarizing...")

    case_data = state.get("case_data", {})
    claims = state.get("claims", [])
    claims_str = json.dumps(claims, ensure_ascii=False)

    llm = LLMClient(config)

    # Summarize each evidence source individually to preserve structure
    summarized = []
    for e in evidence:
        etype = e.get("type", "unknown")
        data = e.get("data", [])
        raw = json.dumps(data, ensure_ascii=False)

        if len(raw) <= 1000:
            # Small enough, keep as-is
            summarized.append(e)
            continue

        summary_text = llm.chat(
            SUMMARIZE_SYSTEM,
            (
                f"案件类型: {case_data.get('caseType', '')}\n"
                f"待验证主张: {claims_str}\n\n"
                f"请摘要以下 {etype} 类型的证据（{len(raw)}字符）:\n\n"
                f"{raw[:6000]}"  # Cap input to avoid sending too much
            ),
        )
        summarized.append({
            "type": etype,
            "data": summary_text,
            "_summarized": True,
            "_original_chars": len(raw),
        })

    return {"evidence": summarized}
