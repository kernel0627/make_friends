import { useState } from "react";
import PaginationBar from "../components/PaginationBar";
import Modal from "../components/Modal";
import { fetchAdminCaseDetail, fetchAdminCases, resolveAdminCase, investigateCase, reviewCaseDecision } from "../lib/api";
import { getSession } from "../lib/session";
import usePagedList from "../lib/usePagedList";
import {
  getCaseResolutionLabel,
  getCaseStatusLabel,
  getCaseStatusTone,
  getSettlementDecisionLabel,
} from "../lib/display";
import { formatDateTime, formatDelta } from "../lib/format";

const initialFilters = { page: 1, pageSize: 20, status: "", keyword: "" };

function ComparisonCard({ title, item }) {
  if (!item) return null;
  return (
    <div className="comparison-card">
      <div className="comparison-card__title">{title}</div>
      <div className="comparison-card__name">{item.nickname || "-"}</div>
      <div className="comparison-card__grid">
        <div>
          <span className="comparison-card__label">处理前</span>
          <strong>{item.before ?? "-"}</strong>
        </div>
        <div>
          <span className="comparison-card__label">处理后</span>
          <strong>{item.after ?? "-"}</strong>
        </div>
        <div>
          <span className="comparison-card__label">变化值</span>
          <strong className={Number(item.delta) >= 0 ? "delta-positive" : "delta-negative"}>{formatDelta(item.delta)}</strong>
        </div>
      </div>
    </div>
  );
}

const ACTION_LABELS = {
  credit_deduct: "扣除信誉分",
  credit_restore: "恢复信誉分",
  post_takedown: "下架帖子",
  post_restore: "恢复帖子",
};

function getActionLabel(action) {
  return ACTION_LABELS[action] || action || "未知操作";
}

function ConfidenceBar({ value }) {
  const pct = Math.round((value || 0) * 100);
  const tone = pct >= 80 ? "confidence-high" : pct >= 50 ? "confidence-mid" : "confidence-low";
  return (
    <div className="confidence-bar">
      <div className={`confidence-bar__fill ${tone}`} style={{ width: `${pct}%` }} />
      <span className="confidence-bar__label">{pct}%</span>
    </div>
  );
}

function DecisionCard({ decision, onApprove, onReject, loading: actionLoading }) {
  const [rejectComment, setRejectComment] = useState("");
  const [showReject, setShowReject] = useState(false);

  if (!decision) return null;

  let actions = [];
  try {
    actions = JSON.parse(decision.actions || "[]");
  } catch { /* ignore */ }

  const isPending = decision.status === "proposed";

  return (
    <div className="detail-block decision-card">
      <div className="detail-label">Agent 调查结论</div>
      <div className="decision-card__header">
        <span className={`status-tag status-${decision.status}`}>{decision.status}</span>
        <span className="decision-card__outcome">{decision.outcome || "-"}</span>
        {decision.responsibleParty ? <span className="muted-text">责任方：{decision.responsibleParty}</span> : null}
      </div>

      <div className="decision-card__confidence">
        <span className="detail-sublabel">置信度</span>
        <ConfidenceBar value={decision.confidence} />
      </div>

      {actions.length > 0 ? (
        <div className="decision-card__actions">
          <span className="detail-sublabel">建议操作</span>
          <ul className="action-list">
            {actions.map((act, i) => (
              <li key={i}>
                <strong>{getActionLabel(act.action)}</strong>
                {act.amount ? ` (${act.amount}分)` : ""}
                {act.reason ? ` — ${act.reason}` : ""}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {decision.reasoning ? (
        <details className="decision-card__reasoning">
          <summary>推理过程</summary>
          <p>{decision.reasoning}</p>
        </details>
      ) : null}

      {!isPending && decision.approvedBy ? (
        <div className="decision-card__reviewed">
          <span className="muted-text">
            {decision.status === "rejected" ? "驳回" : "批准"}：
            {decision.approvedBy === "system:auto-approve" ? "系统自动审批" : decision.approvedBy}
            {decision.approvedAt ? ` · ${new Date(decision.approvedAt).toLocaleString("zh-CN")}` : ""}
          </span>
        </div>
      ) : null}

      {isPending ? (
        <div className="decision-card__buttons">
          <button className="primary-button" disabled={actionLoading} onClick={onApprove}>
            批准执行
          </button>
          <button className="secondary-button" disabled={actionLoading} onClick={() => setShowReject(true)}>
            驳回
          </button>
          {showReject ? (
            <div className="reject-form">
              <textarea
                rows="3"
                placeholder="驳回原因（必填）"
                value={rejectComment}
                onChange={(e) => setRejectComment(e.target.value)}
              />
              <button
                className="danger-button"
                disabled={actionLoading || !rejectComment.trim()}
                onClick={() => onReject(rejectComment)}
              >
                确认驳回
              </button>
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function AgentRunStatus({ agentRun }) {
  if (!agentRun) return null;
  const statusLabel = {
    pending: "等待调查",
    running: "调查中",
    completed: "调查完成",
    failed: "调查失败",
  };
  const isRunning = agentRun.status === "pending" || agentRun.status === "running";
  return (
    <div className="detail-block agent-run-status">
      <div className="detail-label">Agent 调查状态</div>
      <div className="agent-run-status__row">
        {isRunning ? <span className="spinner" /> : null}
        <span className={`status-tag status-${agentRun.status}`}>{statusLabel[agentRun.status] || agentRun.status}</span>
        {agentRun.stepCount > 0 ? <span className="muted-text">已完成 {agentRun.stepCount} 步</span> : null}
      </div>
    </div>
  );
}

export default function CasesPage() {
  const {
    filters,
    setFilters,
    updateFilters,
    data: listData,
    loading,
    error,
    setError,
    refresh: refreshList,
  } = usePagedList(fetchAdminCases, initialFilters);
  const [detail, setDetail] = useState(null);
  const [resolveForm, setResolveForm] = useState({ resolution: "completed", note: "" });
  const [actionLoading, setActionLoading] = useState(false);
  const [confirmApprove, setConfirmApprove] = useState(false);

  async function openDetail(id) {
    try {
      const payload = await fetchAdminCaseDetail(id);
      setDetail(payload);
      setResolveForm({ resolution: "completed", note: "" });
    } catch (err) {
      setError(err.message || "加载案例详情失败");
    }
  }

  async function handleResolve(event) {
    event.preventDefault();
    if (!detail?.case?.id) return;
    try {
      await resolveAdminCase(detail.case.id, resolveForm);
      await openDetail(detail.case.id);
      await refreshList();
    } catch (err) {
      setError(err.message || "结案失败");
    }
  }

  async function handleInvestigate() {
    if (!detail?.case?.id) return;
    setActionLoading(true);
    try {
      await investigateCase(detail.case.id);
      await openDetail(detail.case.id);
      await refreshList();
    } catch (err) {
      setError(err.message || "触发调查失败");
    } finally {
      setActionLoading(false);
    }
  }

  async function handleApproveDecision() {
    if (!detail?.decision?.id) return;
    setActionLoading(true);
    try {
      await reviewCaseDecision(detail.case.id, {
        decisionId: detail.decision.id,
        approve: true,
        adminId: getSession()?.user?.id || "",
        comment: "",
      });
      setConfirmApprove(false);
      await openDetail(detail.case.id);
      await refreshList();
    } catch (err) {
      setError(err.message || "批准失败");
    } finally {
      setActionLoading(false);
    }
  }

  async function handleRejectDecision(comment) {
    if (!detail?.decision?.id) return;
    setActionLoading(true);
    try {
      await reviewCaseDecision(detail.case.id, {
        decisionId: detail.decision.id,
        approve: false,
        adminId: getSession()?.user?.id || "",
        comment,
      });
      await openDetail(detail.case.id);
      await refreshList();
    } catch (err) {
      setError(err.message || "驳回失败");
    } finally {
      setActionLoading(false);
    }
  }

  return (
    <div className="management-shell">
      <section className="panel panel-scroll">
        <div className="panel-header">
          <div>
            <h1>争议案例</h1>
            <p>待处理案例默认排在最前面，方便你优先处理真正卡住的履约问题。</p>
          </div>
        </div>

        <div className="filter-row filter-row-wrap">
          <input
            className="filter-input"
            placeholder="搜索活动标题、目标用户或案例摘要"
            value={filters.keyword}
            onChange={(event) => updateFilters({ keyword: event.target.value })}
          />
          <select value={filters.status} onChange={(event) => updateFilters({ status: event.target.value })}>
            <option value="">全部状态</option>
            <option value="open">待处理</option>
            <option value="investigating">调查中</option>
            <option value="under_review">待审批</option>
            <option value="in_review">处理中</option>
            <option value="resolved">已结案</option>
          </select>
        </div>

        {error ? <div className="error-banner">{error}</div> : null}

        <div className="table-scroll">
          <table className="data-table">
            <thead>
              <tr>
                <th>状态</th>
                <th>活动</th>
                <th>目标用户</th>
                <th>案例摘要</th>
                <th>更新时间</th>
              </tr>
            </thead>
            <tbody>
              {listData.items?.map((item) => (
                <tr
                  key={item.id}
                  className={`${detail?.case?.id === item.id ? "table-row-active" : ""} table-row-clickable`}
                  onClick={() => openDetail(item.id)}
                >
                  <td>
                    <span className={`status-tag ${getCaseStatusTone(item.status)}`}>{getCaseStatusLabel(item.status)}</span>
                  </td>
                  <td>{item.postTitle || "-"}</td>
                  <td>{item.targetNickname || item.targetUserId || "-"}</td>
                  <td>{item.summary || "-"}</td>
                  <td>{formatDateTime(item.updatedAt)}</td>
                </tr>
              ))}
              {!listData.items?.length ? (
                <tr>
                  <td colSpan="5" className="empty-cell">{loading ? "加载中..." : "当前没有争议案例"}</td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>

        <PaginationBar
          page={listData.page || filters.page}
          pageSize={listData.pageSize || filters.pageSize}
          total={listData.total || 0}
          onPageChange={(page) => setFilters((prev) => ({ ...prev, page }))}
          onPageSizeChange={(pageSize) => setFilters((prev) => ({ ...prev, page: 1, pageSize }))}
        />
      </section>

      <aside className="panel detail-panel panel-scroll">
        <div className="panel-header">
          <div>
            <h2>案例详情</h2>
            <p>这里会展示双方提交、处理时间线，以及结案前后的信誉分变化。</p>
          </div>
        </div>

        {!detail ? (
          <div className="empty-state">点击左侧任一案例，右侧就会展开完整详情。</div>
        ) : (
          <div className="detail-scroll">
            <div className="detail-stack">
              <div className="detail-block">
                <div className="detail-label">基础信息</div>
                <div className="detail-title">{detail.case.postTitle || "未关联活动"}</div>
                <div className="muted-text">{detail.case.summary || "暂无案例摘要"}</div>
                <div className="detail-kpis detail-kpis-wrap">
                  <span>状态：{getCaseStatusLabel(detail.case.status)}</span>
                  <span>目标用户：{detail.case.targetNickname || detail.case.targetUserId || "-"}</span>
                  <span>提交人：{detail.case.reporterNickname || detail.case.reporterUserId || "-"}</span>
                  <span>创建时间：{formatDateTime(detail.case.createdAt)}</span>
                </div>
              </div>

              <div className="detail-block">
                <div className="detail-label">履约记录</div>
                <div className="detail-kpis detail-kpis-wrap">
                  <span>参与者提交：{getSettlementDecisionLabel(detail.settlement?.participantDecision)}</span>
                  <span>发起人提交：{getSettlementDecisionLabel(detail.settlement?.authorDecision)}</span>
                  <span>最终状态：{getSettlementDecisionLabel(detail.settlement?.finalStatus || detail.case.status)}</span>
                </div>
                <div className="muted-stack">
                  <div>参与者说明：{detail.settlement?.participantNote || "暂无说明"}</div>
                  <div>发起人说明：{detail.settlement?.authorNote || "暂无说明"}</div>
                </div>
              </div>

              <div className="detail-block">
                <div className="detail-label">处理记录时间线</div>
                <div className="timeline-list">
                  {(detail.timeline || []).map((item, index) => (
                    <div className="timeline-item" key={`${item.time}_${index}`}>
                      <div className="timeline-item__dot" />
                      <div className="timeline-item__content">
                        <div className="timeline-item__title">{item.title}</div>
                        <div className="timeline-item__desc">{item.description || "暂无说明"}</div>
                        <div className="timeline-item__time">{formatDateTime(item.time)}</div>
                      </div>
                    </div>
                  ))}
                  {!detail.timeline?.length ? <div className="empty-state slim">暂无处理时间线</div> : null}
                </div>
              </div>

              <div className="detail-block">
                <div className="detail-label">结案前后信誉变化对比</div>
                <div className="comparison-grid">
                  <ComparisonCard title="目标用户" item={detail.creditComparison?.target} />
                  <ComparisonCard title="提交方 / 发起方" item={detail.creditComparison?.reporter} />
                </div>
              </div>

              <AgentRunStatus agentRun={detail.agentRun} />

              <DecisionCard
                decision={detail.decision}
                loading={actionLoading}
                onApprove={() => setConfirmApprove(true)}
                onReject={handleRejectDecision}
              />

              {detail.case.status === "open" && !detail.agentRun && !detail.decision ? (
                <div className="detail-block">
                  <button className="secondary-button" disabled={actionLoading} onClick={handleInvestigate}>
                    触发 Agent 调查
                  </button>
                </div>
              ) : null}

              {detail.case.status !== "resolved" ? (
                <form className="resolve-form" onSubmit={handleResolve}>
                  <div className="detail-label">管理员结案</div>
                  <label className="field">
                    <span>结案结论</span>
                    <select
                      value={resolveForm.resolution}
                      onChange={(event) => setResolveForm((prev) => ({ ...prev, resolution: event.target.value }))}
                    >
                      <option value="completed">判定已到场</option>
                      <option value="cancelled">判定已取消</option>
                      <option value="no_show">判定未到场</option>
                    </select>
                  </label>
                  <label className="field">
                    <span>结案说明</span>
                    <textarea
                      rows="4"
                      placeholder="把你为什么这样判定写清楚，后续回看会更方便。"
                      value={resolveForm.note}
                      onChange={(event) => setResolveForm((prev) => ({ ...prev, note: event.target.value }))}
                    />
                  </label>
                  <button className="primary-button">确认结案</button>
                </form>
              ) : (
                <div className="detail-block">
                  <div className="detail-label">结案结果</div>
                  <div className="detail-title">{getCaseResolutionLabel(detail.case.resolution)}</div>
                  <div className="muted-text">{detail.case.resolutionNote || "暂无结案说明"}</div>
                  <div className="muted-text">结案时间：{formatDateTime(detail.case.resolvedAt)}</div>
                </div>
              )}
            </div>
          </div>
        )}
      </aside>

      <Modal
        open={confirmApprove}
        title="确认批准"
        onClose={() => setConfirmApprove(false)}
        width={520}
        footer={
          <div className="modal-buttons">
            <button className="primary-button" disabled={actionLoading} onClick={handleApproveDecision}>确认批准</button>
            <button className="secondary-button" onClick={() => setConfirmApprove(false)}>取消</button>
          </div>
        }
      >
        <p>以下操作将被执行：</p>
        <ul className="action-list">
          {(() => {
            try {
              return JSON.parse(detail?.decision?.actions || "[]").map((act, i) => (
                <li key={i}>{getActionLabel(act.action)}{act.amount ? ` (${act.amount}分)` : ""}{act.reason ? ` — ${act.reason}` : ""}</li>
              ));
            } catch { return <li>无法解析操作列表</li>; }
          })()}
        </ul>
      </Modal>
    </div>
  );
}