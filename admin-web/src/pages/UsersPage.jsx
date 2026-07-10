import { useEffect, useState } from "react";
import Modal from "../components/Modal";
import PaginationBar from "../components/PaginationBar";
import {
  adjustAdminUserCredit,
  createAdminUser,
  deleteAdminUser,
  fetchAdminUserCreditLedger,
  fetchAdminUserDetail,
  fetchAdminUsers,
  resetAdminUserPassword,
  restoreAdminUser,
  updateAdminUser,
} from "../lib/api";
import { getLedgerSourceLabel, getRoleLabel, getUserStatusLabel, getUserStatusTone } from "../lib/display";
import { formatDateTime, formatDelta, formatScore } from "../lib/format";

const initialFilters = { page: 1, pageSize: 20, keyword: "", status: "active" };

export default function UsersPage() {
  const [filters, setFilters] = useState(initialFilters);
  const [data, setData] = useState({ items: [], total: 0, page: 1, pageSize: 20 });
  const [selectedUser, setSelectedUser] = useState(null);
  const [ledger, setLedger] = useState([]);
  const [adjustForm, setAdjustForm] = useState({ delta: -3, note: "" });
  const [createForm, setCreateForm] = useState({ nickname: "", password: "123456" });
  const [editForm, setEditForm] = useState({ nickname: "" });
  const [passwordForm, setPasswordForm] = useState({ password: "123456" });
  const [showCreate, setShowCreate] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    refreshList();
  }, [filters]);

  async function refreshList() {
    try {
      const payload = await fetchAdminUsers(filters);
      setData(payload);
    } catch (err) {
      setError(err.message || "加载用户列表失败");
    }
  }

  async function openUser(userId) {
    try {
      const [detail, ledgerPayload] = await Promise.all([
        fetchAdminUserDetail(userId),
        fetchAdminUserCreditLedger(userId, { page: 1, pageSize: 20 }),
      ]);
      setSelectedUser(detail);
      setLedger(ledgerPayload.items || []);
      setEditForm({ nickname: detail.user?.nickName || "" });
      setPasswordForm({ password: "123456" });
      setAdjustForm({ delta: -3, note: "" });
    } catch (err) {
      setError(err.message || "加载用户详情失败");
    }
  }

  async function handleCreate(event) {
    event.preventDefault();
    try {
      await createAdminUser(createForm);
      setShowCreate(false);
      setCreateForm({ nickname: "", password: "123456" });
      await refreshList();
    } catch (err) {
      setError(err.message || "创建用户失败");
    }
  }

  async function handleSave(event) {
    event.preventDefault();
    if (!selectedUser?.user?.id) return;
    try {
      await updateAdminUser(selectedUser.user.id, editForm);
      await openUser(selectedUser.user.id);
      await refreshList();
    } catch (err) {
      setError(err.message || "保存用户失败");
    }
  }

  async function handleResetPassword(event) {
    event.preventDefault();
    if (!selectedUser?.user?.id) return;
    try {
      await resetAdminUserPassword(selectedUser.user.id, passwordForm);
      setPasswordForm({ password: "123456" });
    } catch (err) {
      setError(err.message || "重置密码失败");
    }
  }

  async function handleDelete() {
    if (!selectedUser?.user?.id || !window.confirm("确认软删除这个用户吗？")) return;
    try {
      await deleteAdminUser(selectedUser.user.id);
      await openUser(selectedUser.user.id);
      await refreshList();
    } catch (err) {
      setError(err.message || "删除用户失败");
    }
  }

  async function handleRestore() {
    if (!selectedUser?.user?.id) return;
    try {
      await restoreAdminUser(selectedUser.user.id);
      await openUser(selectedUser.user.id);
      await refreshList();
    } catch (err) {
      setError(err.message || "恢复用户失败");
    }
  }

  async function handleAdjust(event) {
    event.preventDefault();
    if (!selectedUser?.user?.id) return;
    try {
      await adjustAdminUserCredit(selectedUser.user.id, {
        delta: Number(adjustForm.delta),
        note: adjustForm.note.trim(),
      });
      await openUser(selectedUser.user.id);
      await refreshList();
    } catch (err) {
      setError(err.message || "调整信誉分失败");
    }
  }

  function updateFilters(patch) {
    setFilters((prev) => ({ ...prev, ...patch, page: patch.page || 1 }));
  }

  return (
    <div className="management-shell">
      <section className="panel panel-scroll">
        <div className="panel-header">
          <div>
            <h1>用户管理</h1>
            <p>点整行打开详情；筛选变化会自动回到第一页，创建用户走顶部弹窗。</p>
          </div>
        </div>

        <div className="filter-row filter-row-wrap">
          <input
            className="filter-input"
            placeholder="搜索昵称、角色或状态"
            value={filters.keyword}
            onChange={(event) => updateFilters({ keyword: event.target.value })}
          />
          <select value={filters.status} onChange={(event) => updateFilters({ status: event.target.value })}>
            <option value="active">正常</option>
            <option value="deleted">已删除</option>
            <option value="all">全部</option>
          </select>
          <div className="filter-row__spacer" />
          <button className="primary-button" type="button" onClick={() => setShowCreate(true)}>
            创建用户
          </button>
        </div>

        {error ? <div className="error-banner">{error}</div> : null}

        <div className="table-scroll">
          <table className="data-table">
            <thead>
              <tr>
                <th>昵称</th>
                <th>角色</th>
                <th>信誉分</th>
                <th>评价分</th>
                <th>状态</th>
                <th>注册时间</th>
              </tr>
            </thead>
            <tbody>
              {data.items?.map((user) => (
                <tr
                  key={user.id}
                  className={`${selectedUser?.user?.id === user.id ? "table-row-active" : ""} table-row-clickable`}
                  onClick={() => openUser(user.id)}
                >
                  <td>{user.nickName}</td>
                  <td>{getRoleLabel(user.role)}</td>
                  <td>{user.creditScore}</td>
                  <td>{formatScore(user.ratingScore)}</td>
                  <td>
                    <span className={`status-tag ${getUserStatusTone(user)}`}>{getUserStatusLabel(user)}</span>
                  </td>
                  <td>{formatDateTime(user.createdAt)}</td>
                </tr>
              ))}
              {!data.items?.length ? (
                <tr>
                  <td colSpan="6" className="empty-cell">当前没有用户数据</td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>

        <PaginationBar
          page={data.page || filters.page}
          pageSize={data.pageSize || filters.pageSize}
          total={data.total || 0}
          onPageChange={(page) => setFilters((prev) => ({ ...prev, page }))}
          onPageSizeChange={(pageSize) => setFilters((prev) => ({ ...prev, page: 1, pageSize }))}
        />
      </section>

      <aside className="panel detail-panel panel-scroll">
        <div className="panel-header">
          <div>
            <h2>用户详情</h2>
            <p>编辑、调分、重置密码都放在这里，信誉流水也只保留在这里查看。</p>
          </div>
        </div>

        {!selectedUser ? (
          <div className="empty-state">点击左侧用户，右侧会展开完整详情。</div>
        ) : (
          <div className="detail-scroll">
            <div className="detail-stack">
              <div className="detail-block">
                <div className="detail-card-header">
                  <div>
                    <div className="detail-label">基础信息</div>
                    <div className="detail-title">{selectedUser.user.nickName}</div>
                    <div className="muted-text">
                      {getRoleLabel(selectedUser.user.role)} / {getUserStatusLabel(selectedUser.user)}
                    </div>
                  </div>
                  <div className="detail-card-actions">
                    {Number(selectedUser.user.deletedAt) > 0 ? (
                      <button className="secondary-button" type="button" onClick={handleRestore}>恢复用户</button>
                    ) : (
                      <button className="danger-button" type="button" onClick={handleDelete}>软删除</button>
                    )}
                  </div>
                </div>
                <div className="detail-kpis detail-kpis-wrap">
                  <span>信誉分：{selectedUser.user.creditScore}</span>
                  <span>评价分：{formatScore(selectedUser.user.ratingScore)}</span>
                  <span>注册时间：{formatDateTime(selectedUser.user.createdAt)}</span>
                </div>
              </div>

              <div className="detail-block">
                <div className="detail-label">活动概况</div>
                <div className="detail-kpis detail-kpis-wrap">
                  <span>发起活动 {selectedUser.initiatedPostCount}</span>
                  <span>参与活动 {selectedUser.joinedPostCount}</span>
                  <span>评价记录 {selectedUser.reviewCount}</span>
                </div>
              </div>

              <form className="crud-form" onSubmit={handleSave}>
                <div className="detail-label">编辑资料</div>
                <label className="field">
                  <span>用户昵称</span>
                  <input value={editForm.nickname} onChange={(event) => setEditForm({ nickname: event.target.value })} />
                </label>
                <button className="primary-button">保存资料</button>
              </form>

              <form className="crud-form" onSubmit={handleResetPassword}>
                <div className="detail-label">重置密码</div>
                <label className="field">
                  <span>新密码</span>
                  <input value={passwordForm.password} onChange={(event) => setPasswordForm({ password: event.target.value })} />
                </label>
                <button className="secondary-button">确认重置</button>
              </form>

              <form className="resolve-form" onSubmit={handleAdjust}>
                <div className="detail-label">手动调整信誉分</div>
                <div className="form-grid form-grid-2">
                  <label className="field">
                    <span>分值变化</span>
                    <input type="number" value={adjustForm.delta} onChange={(event) => setAdjustForm((prev) => ({ ...prev, delta: event.target.value }))} />
                  </label>
                  <label className="field">
                    <span>调整说明</span>
                    <input value={adjustForm.note} onChange={(event) => setAdjustForm((prev) => ({ ...prev, note: event.target.value }))} placeholder="说明本次为什么调分" />
                  </label>
                </div>
                <button className="primary-button">确认调分</button>
              </form>

              <div className="detail-block">
                <div className="detail-label">最近信誉流水</div>
                <div className="ledger-list">
                  {ledger.map((item) => (
                    <div key={`${item.postId}_${item.createdAt}_${item.sourceType}`} className="ledger-item">
                      <div>
                        <div className="ledger-title">{item.postTitle || getLedgerSourceLabel(item.sourceType)}</div>
                        <div className="muted-text">{item.note || getLedgerSourceLabel(item.sourceType)}</div>
                      </div>
                      <div className="ledger-side">
                        <div className={Number(item.delta) >= 0 ? "delta-positive" : "delta-negative"}>{formatDelta(item.delta)}</div>
                        <div className="muted-text">{formatDateTime(item.createdAt)}</div>
                      </div>
                    </div>
                  ))}
                  {!ledger.length ? <div className="empty-state slim">当前没有信誉流水</div> : null}
                </div>
              </div>
            </div>
          </div>
        )}
      </aside>

      <Modal
        open={showCreate}
        title="创建用户"
        onClose={() => setShowCreate(false)}
        footer={
          <>
            <button className="ghost-button" type="button" onClick={() => setShowCreate(false)}>取消</button>
            <button className="primary-button" type="submit" form="create-user-form">确认创建</button>
          </>
        }
      >
        <form id="create-user-form" className="form-grid form-grid-2" onSubmit={handleCreate}>
          <label className="field">
            <span>用户昵称</span>
            <input value={createForm.nickname} onChange={(event) => setCreateForm((prev) => ({ ...prev, nickname: event.target.value }))} placeholder="请输入不重复的中文昵称" />
          </label>
          <label className="field">
            <span>登录密码</span>
            <input value={createForm.password} onChange={(event) => setCreateForm((prev) => ({ ...prev, password: event.target.value }))} placeholder="至少 6 位" />
          </label>
        </form>
      </Modal>
    </div>
  );
}