import { useEffect, useState } from "react";
import Modal from "../components/Modal";
import PaginationBar from "../components/PaginationBar";
import {
  createAdminAccount,
  deleteAdminAccount,
  fetchAdminAccounts,
  fetchAdminUserCreditLedger,
  fetchAdminUserDetail,
  resetAdminAccountPassword,
  restoreAdminAccount,
  updateAdminAccount,
} from "../lib/api";
import { formatDateTime, formatDelta, formatScore } from "../lib/format";
import { getLedgerSourceLabel, getRoleLabel, getUserStatusLabel, getUserStatusTone } from "../lib/display";
import { getSession } from "../lib/session";

const initialFilters = { page: 1, pageSize: 20, status: "all", keyword: "" };

export default function AdminAccountsPage() {
  const session = getSession();
  const canManage = Boolean(session?.user?.rootAdmin);
  const [filters, setFilters] = useState(initialFilters);
  const [data, setData] = useState({ items: [], total: 0, page: 1, pageSize: 20 });
  const [selected, setSelected] = useState(null);
  const [ledger, setLedger] = useState([]);
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
      const payload = await fetchAdminAccounts(filters);
      setData(payload);
    } catch (err) {
      setError(err.message || "加载管理员账号失败");
    }
  }

  async function selectAccount(item) {
    try {
      const [detail, ledgerPayload] = await Promise.all([
        fetchAdminUserDetail(item.id),
        fetchAdminUserCreditLedger(item.id, { page: 1, pageSize: 20 }),
      ]);
      setSelected(detail);
      setLedger(ledgerPayload.items || []);
      setEditForm({ nickname: detail.user?.nickName || "" });
      setPasswordForm({ password: "123456" });
    } catch (err) {
      setError(err.message || "加载管理员详情失败");
    }
  }

  async function handleCreate(event) {
    event.preventDefault();
    try {
      await createAdminAccount(createForm);
      setShowCreate(false);
      setCreateForm({ nickname: "", password: "123456" });
      await refreshList();
    } catch (err) {
      setError(err.message || "创建管理员失败");
    }
  }

  async function handleSave(event) {
    event.preventDefault();
    if (!selected?.user?.id) return;
    try {
      await updateAdminAccount(selected.user.id, { nickname: editForm.nickname.trim() });
      await selectAccount({ id: selected.user.id });
      await refreshList();
    } catch (err) {
      setError(err.message || "保存管理员失败");
    }
  }

  async function handleResetPassword(event) {
    event.preventDefault();
    if (!selected?.user?.id) return;
    try {
      await resetAdminAccountPassword(selected.user.id, passwordForm);
      setPasswordForm({ password: "123456" });
    } catch (err) {
      setError(err.message || "重置管理员密码失败");
    }
  }

  async function handleDelete() {
    if (!selected?.user?.id || !window.confirm("确认软删除这个管理员账号吗？")) return;
    try {
      await deleteAdminAccount(selected.user.id);
      await selectAccount({ id: selected.user.id });
      await refreshList();
    } catch (err) {
      setError(err.message || "删除管理员失败");
    }
  }

  async function handleRestore() {
    if (!selected?.user?.id) return;
    try {
      await restoreAdminAccount(selected.user.id);
      await selectAccount({ id: selected.user.id });
      await refreshList();
    } catch (err) {
      setError(err.message || "恢复管理员失败");
    }
  }

  function updateFilters(patch) {
    setFilters((prev) => ({ ...prev, ...patch, page: patch.page || 1 }));
  }

  const isSelf = selected?.user?.id === session?.user?.id;
  const isRootTarget = Boolean(selected?.user?.rootAdmin);
  const readOnly = !canManage || isSelf || isRootTarget;

  return (
    <div className="management-shell">
      <section className="panel panel-scroll">
        <div className="panel-header">
          <div>
            <h1>管理员账号</h1>
            <p>只有 root admin，也就是 admin 账号，才可以创建和管理其他管理员。</p>
          </div>
        </div>

        <div className="filter-row filter-row-wrap">
          <input
            className="filter-input"
            placeholder="搜索管理员昵称"
            value={filters.keyword}
            onChange={(event) => updateFilters({ keyword: event.target.value })}
          />
          <select value={filters.status} onChange={(event) => updateFilters({ status: event.target.value })}>
            <option value="all">全部</option>
            <option value="active">正常</option>
            <option value="deleted">已删除</option>
          </select>
          <div className="filter-row__spacer" />
          {canManage ? (
            <button className="primary-button" type="button" onClick={() => setShowCreate(true)}>
              创建管理员
            </button>
          ) : null}
        </div>

        {!canManage ? <div className="notice-banner">当前账号只有查看权限，不能管理其他管理员。</div> : null}
        {error ? <div className="error-banner">{error}</div> : null}

        <div className="table-scroll">
          <table className="data-table">
            <thead>
              <tr>
                <th>昵称</th>
                <th>角色</th>
                <th>最高权限</th>
                <th>状态</th>
                <th>信誉分</th>
                <th>创建时间</th>
              </tr>
            </thead>
            <tbody>
              {data.items?.map((item) => (
                <tr
                  key={item.id}
                  className={`${selected?.user?.id === item.id ? "table-row-active" : ""} table-row-clickable`}
                  onClick={() => selectAccount(item)}
                >
                  <td>{item.nickName}</td>
                  <td>{getRoleLabel(item.role)}</td>
                  <td>{item.rootAdmin ? "是" : "否"}</td>
                  <td>
                    <span className={`status-tag ${getUserStatusTone(item)}`}>{getUserStatusLabel(item)}</span>
                  </td>
                  <td>{item.creditScore}</td>
                  <td>{formatDateTime(item.createdAt)}</td>
                </tr>
              ))}
              {!data.items?.length ? (
                <tr>
                  <td colSpan="6" className="empty-cell">当前没有管理员账号</td>
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
            <h2>管理员详情</h2>
            <p>root admin 账号不能在这里被删除；你自己当前登录的账号也不能删自己。</p>
          </div>
        </div>
        {!selected ? (
          <div className="empty-state">点击左侧管理员账号，右侧会展开详情。</div>
        ) : (
          <div className="detail-scroll">
            <div className="detail-stack">
              <div className="detail-block">
                <div className="detail-card-header">
                  <div>
                    <div className="detail-label">基础信息</div>
                    <div className="detail-title">{selected.user.nickName}</div>
                    <div className="muted-text">
                      {selected.user.rootAdmin ? "最高管理员" : "普通管理员"} / {getUserStatusLabel(selected.user)}
                    </div>
                  </div>
                  <div className="detail-card-actions">
                    {!readOnly ? (
                      Number(selected.user.deletedAt) > 0 ? (
                        <button className="secondary-button" type="button" onClick={handleRestore}>恢复账号</button>
                      ) : (
                        <button className="danger-button" type="button" onClick={handleDelete}>软删除</button>
                      )
                    ) : null}
                  </div>
                </div>
                <div className="detail-kpis detail-kpis-wrap">
                  <span>角色：{getRoleLabel(selected.user.role)}</span>
                  <span>最高权限：{selected.user.rootAdmin ? "是" : "否"}</span>
                  <span>信誉分：{selected.user.creditScore}</span>
                  <span>评价分：{formatScore(selected.user.ratingScore)}</span>
                  <span>创建时间：{formatDateTime(selected.user.createdAt)}</span>
                </div>
              </div>

              {!canManage ? <div className="notice-banner">当前账号只有查看权限。</div> : null}
              {isRootTarget ? <div className="notice-banner">最高管理员账号只允许查看和重置密码，不允许被删除。</div> : null}
              {isSelf ? <div className="notice-banner">当前登录账号不能删除自己。</div> : null}

              {canManage ? (
                <>
                  <form className="crud-form" onSubmit={handleSave}>
                    <div className="detail-label">编辑昵称</div>
                    <label className="field">
                      <span>管理员昵称</span>
                      <input value={editForm.nickname} onChange={(event) => setEditForm({ nickname: event.target.value })} disabled={isRootTarget || isSelf} />
                    </label>
                    <button className="primary-button" disabled={isRootTarget || isSelf}>保存资料</button>
                  </form>

                  <form className="crud-form" onSubmit={handleResetPassword}>
                    <div className="detail-label">重置密码</div>
                    <label className="field">
                      <span>新密码</span>
                      <input value={passwordForm.password} onChange={(event) => setPasswordForm({ password: event.target.value })} />
                    </label>
                    <button className="secondary-button">确认重置</button>
                  </form>
                </>
              ) : null}

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
        title="创建管理员"
        onClose={() => setShowCreate(false)}
        footer={
          <>
            <button className="ghost-button" type="button" onClick={() => setShowCreate(false)}>取消</button>
            <button className="primary-button" type="submit" form="create-admin-form">确认创建</button>
          </>
        }
      >
        <form id="create-admin-form" className="form-grid form-grid-2" onSubmit={handleCreate}>
          <label className="field">
            <span>管理员昵称</span>
            <input value={createForm.nickname} onChange={(event) => setCreateForm((prev) => ({ ...prev, nickname: event.target.value }))} placeholder="请输入唯一昵称" />
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