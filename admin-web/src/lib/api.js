import { clearSession, getSession } from "./session";

const API_BASE = (import.meta.env.VITE_API_BASE || "http://127.0.0.1:8080").replace(/\/$/, "");

function buildQuery(params = {}) {
  const query = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === "") {
      return;
    }
    query.set(key, String(value));
  });
  const raw = query.toString();
  return raw ? `?${raw}` : "";
}

export async function apiRequest(path, options = {}) {
  const session = getSession();
  const headers = {
    "Content-Type": "application/json",
    ...(options.headers || {}),
  };
  if (session?.token) {
    headers.Authorization = `Bearer ${session.token}`;
  }

  const url = path.startsWith("http://") || path.startsWith("https://") ? path : `${API_BASE}${path}`;
  const response = await fetch(url, {
    ...options,
    headers,
  });

  if (response.status === 401) {
    clearSession();
  }

  const contentType = response.headers.get("content-type") || "";
  const payload = contentType.includes("application/json")
    ? await response.json()
    : await response.text();

  if (!response.ok) {
    const message =
      payload?.error ||
      payload?.message ||
      (typeof payload === "string" ? payload : "request failed");
    throw new Error(message);
  }
  return payload;
}

export function loginByPassword(body) {
  return apiRequest("/api/v1/auth/password-login", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function fetchAdminDashboardSummary() {
  return apiRequest("/api/v1/admin/dashboard/summary");
}

export function fetchAdminDashboardAnalytics(params) {
  return apiRequest(`/api/v1/admin/dashboard/analytics${buildQuery(params)}`);
}

export function fetchAdminCases(params) {
  return apiRequest(`/api/v1/admin/cases${buildQuery(params)}`);
}

export function fetchAdminCaseDetail(id) {
  return apiRequest(`/api/v1/admin/cases/${id}`);
}

export function resolveAdminCase(id, body) {
  return apiRequest(`/api/v1/admin/cases/${id}/resolve`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function fetchAdminUsers(params) {
  return apiRequest(`/api/v1/admin/users${buildQuery(params)}`);
}

export function createAdminUser(body) {
  return apiRequest("/api/v1/admin/users", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function updateAdminUser(id, body) {
  return apiRequest(`/api/v1/admin/users/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function deleteAdminUser(id) {
  return apiRequest(`/api/v1/admin/users/${id}`, { method: "DELETE" });
}

export function restoreAdminUser(id) {
  return apiRequest(`/api/v1/admin/users/${id}/restore`, { method: "POST" });
}

export function resetAdminUserPassword(id, body) {
  return apiRequest(`/api/v1/admin/users/${id}/reset-password`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function fetchAdminUserDetail(id) {
  return apiRequest(`/api/v1/admin/users/${id}`);
}

export function fetchAdminUserCreditLedger(id, params) {
  return apiRequest(`/api/v1/admin/users/${id}/credit-ledger${buildQuery(params)}`);
}

export function adjustAdminUserCredit(id, body) {
  return apiRequest(`/api/v1/admin/users/${id}/credit-adjust`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function fetchAdminPosts(params) {
  return apiRequest(`/api/v1/admin/posts${buildQuery(params)}`);
}

export function createAdminPost(body) {
  return apiRequest("/api/v1/admin/posts", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function updateAdminPost(id, body) {
  return apiRequest(`/api/v1/admin/posts/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function deleteAdminPost(id) {
  return apiRequest(`/api/v1/admin/posts/${id}`, { method: "DELETE" });
}

export function restoreAdminPost(id) {
  return apiRequest(`/api/v1/admin/posts/${id}/restore`, { method: "POST" });
}

export function fetchAdminPostDetail(id) {
  return apiRequest(`/api/v1/admin/posts/${id}`);
}

export function fetchAdminReviews(params) {
  return apiRequest(`/api/v1/admin/reviews${buildQuery(params)}`);
}

export function fetchAdminAccounts(params) {
  return apiRequest(`/api/v1/admin/admin-users${buildQuery(params)}`);
}

export function createAdminAccount(body) {
  return apiRequest("/api/v1/admin/admin-users", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function updateAdminAccount(id, body) {
  return apiRequest(`/api/v1/admin/admin-users/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function deleteAdminAccount(id) {
  return apiRequest(`/api/v1/admin/admin-users/${id}`, { method: "DELETE" });
}

export function restoreAdminAccount(id) {
  return apiRequest(`/api/v1/admin/admin-users/${id}/restore`, { method: "POST" });
}

export function resetAdminAccountPassword(id, body) {
  return apiRequest(`/api/v1/admin/admin-users/${id}/reset-password`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}
