export const ROLE_LABELS = {
  user: "普通用户",
  admin: "管理员",
};

export const USER_STATUS_LABELS = {
  active: "正常",
  deleted: "已删除",
};

export const POST_STATUS_LABELS = {
  open: "进行中",
  closed: "已关闭",
  deleted: "已删除",
};

export const CASE_STATUS_LABELS = {
  open: "待处理",
  in_review: "处理中",
  resolved: "已结案",
};

export const TIME_MODE_LABELS = {
  range: "范围天数",
  fixed: "固定时间",
};

export const SETTLEMENT_DECISION_LABELS = {
  completed: "已到场",
  cancelled: "已取消",
  no_show: "未到场",
  disputed: "活动异常",
  pending: "待处理",
};

export const LEDGER_SOURCE_LABELS = {
  participant_completed: "参与者完成活动",
  organizer_completed: "发起人完成活动",
  participant_cancelled: "参与者主动取消",
  participant_no_show: "参与者未到场",
  organizer_cancelled: "发起人取消项目",
  review_completed: "按时完成评价",
  review_missed: "超时未完成评价",
  manual_credit_adjust: "管理员手动调分",
};

export const CASE_RESOLUTION_LABELS = {
  completed: "判定已到场",
  cancelled: "判定已取消",
  no_show: "判定未到场",
  auto_closed: "系统自动结案",
};

export function getRoleLabel(value) {
  return ROLE_LABELS[value] || value || "-";
}

export function getUserStatusLabel(user) {
  return user && Number(user.deletedAt) > 0 ? USER_STATUS_LABELS.deleted : USER_STATUS_LABELS.active;
}

export function getPostStatusLabel(post) {
  if (post && Number(post.deletedAt) > 0) {
    return POST_STATUS_LABELS.deleted;
  }
  return POST_STATUS_LABELS[post?.status] || post?.status || "-";
}

export function getCaseStatusLabel(value) {
  return CASE_STATUS_LABELS[value] || value || "-";
}

export function getTimeModeLabel(value) {
  return TIME_MODE_LABELS[value] || "时间设置";
}

export function getSettlementDecisionLabel(value) {
  return SETTLEMENT_DECISION_LABELS[value] || value || "待处理";
}

export function getLedgerSourceLabel(value) {
  return LEDGER_SOURCE_LABELS[value] || value || "信誉变动";
}

export function getCaseResolutionLabel(value) {
  return CASE_RESOLUTION_LABELS[value] || value || "待处理";
}

export function getPostStatusTone(post) {
  if (post && Number(post.deletedAt) > 0) return "status-resolved";
  if (post?.status === "closed") return "status-closed";
  return "status-open";
}

export function getUserStatusTone(user) {
  return user && Number(user.deletedAt) > 0 ? "status-resolved" : "status-open";
}

export function getCaseStatusTone(value) {
  if (value === "resolved") return "status-resolved";
  if (value === "in_review") return "status-in_review";
  return "status-open";
}

export function mapAnalyticsSeries(series, mapper) {
  if (!Array.isArray(series)) return [];
  return series.map((item) => ({
    ...item,
    label: mapper ? mapper(item.label) : item.label,
  }));
}

export function mapPostStatusName(value) {
  return POST_STATUS_LABELS[value] || value || "-";
}

export function mapCaseStatusName(value) {
  return CASE_STATUS_LABELS[value] || value || "-";
}