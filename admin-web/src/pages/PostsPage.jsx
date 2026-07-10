import { useEffect, useMemo, useState } from "react";
import Modal from "../components/Modal";
import PaginationBar from "../components/PaginationBar";
import {
  createAdminPost,
  deleteAdminPost,
  fetchAdminPostDetail,
  fetchAdminPosts,
  restoreAdminPost,
  updateAdminPost,
} from "../lib/api";
import { getPostStatusLabel, getPostStatusTone, getTimeModeLabel } from "../lib/display";
import { formatDateTime } from "../lib/format";

const initialFilters = { page: 1, pageSize: 20, keyword: "", status: "active" };
const emptyPostForm = {
  authorNickname: "",
  title: "",
  description: "",
  category: "运动",
  subCategory: "",
  address: "",
  maxCount: 4,
  status: "open",
  timeInfo: {
    mode: "range",
    days: 3,
    fixedTime: "",
  },
};

function toDateTimeInput(value) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const pad = (n) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function toFixedPayload(value) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toISOString();
}

function toPostForm(detail) {
  const post = detail?.post || {};
  return {
    authorNickname: detail?.author?.nickName || "",
    title: post.title || "",
    description: post.description || "",
    category: post.category || "运动",
    subCategory: post.subCategory || "",
    address: post.address || "",
    maxCount: post.maxCount || 4,
    status: post.status || "open",
    timeInfo: {
      mode: post.timeMode || "range",
      days: post.timeDays || 3,
      fixedTime: toDateTimeInput(post.fixedTime),
    },
  };
}

function buildPayload(form) {
  return {
    authorNickname: form.authorNickname.trim(),
    title: form.title.trim(),
    description: form.description.trim(),
    category: form.category.trim(),
    subCategory: form.subCategory.trim(),
    address: form.address.trim(),
    maxCount: Number(form.maxCount) || 2,
    status: form.status,
    timeInfo: {
      mode: form.timeInfo.mode,
      days: form.timeInfo.mode === "range" ? Number(form.timeInfo.days) || 1 : 0,
      fixedTime: form.timeInfo.mode === "fixed" ? toFixedPayload(form.timeInfo.fixedTime) : "",
    },
  };
}

function formatTimeText(post) {
  if (!post) return "-";
  if (post.timeMode === "fixed" && post.fixedTime) {
    return formatDateTime(new Date(post.fixedTime).getTime());
  }
  return `范围 ${post.timeDays || 0} 天`;
}

export default function PostsPage() {
  const [filters, setFilters] = useState(initialFilters);
  const [data, setData] = useState({ items: [], total: 0, page: 1, pageSize: 20 });
  const [detail, setDetail] = useState(null);
  const [form, setForm] = useState(emptyPostForm);
  const [createForm, setCreateForm] = useState(emptyPostForm);
  const [showCreate, setShowCreate] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    refreshList();
  }, [filters]);

  async function refreshList() {
    try {
      const payload = await fetchAdminPosts(filters);
      setData(payload);
    } catch (err) {
      setError(err.message || "加载活动列表失败");
    }
  }

  async function openDetail(postId) {
    try {
      const payload = await fetchAdminPostDetail(postId);
      setDetail(payload);
      setForm(toPostForm(payload));
    } catch (err) {
      setError(err.message || "加载活动详情失败");
    }
  }

  async function handleCreate(event) {
    event.preventDefault();
    try {
      await createAdminPost(buildPayload(createForm));
      setShowCreate(false);
      setCreateForm(emptyPostForm);
      await refreshList();
    } catch (err) {
      setError(err.message || "创建活动失败");
    }
  }

  async function handleSave(event) {
    event.preventDefault();
    if (!detail?.post?.id) return;
    try {
      await updateAdminPost(detail.post.id, buildPayload(form));
      await openDetail(detail.post.id);
      await refreshList();
    } catch (err) {
      setError(err.message || "保存活动失败");
    }
  }

  async function handleDelete() {
    if (!detail?.post?.id || !window.confirm("确认软删除这个活动吗？")) return;
    try {
      await deleteAdminPost(detail.post.id);
      await openDetail(detail.post.id);
      await refreshList();
    } catch (err) {
      setError(err.message || "删除活动失败");
    }
  }

  async function handleRestore() {
    if (!detail?.post?.id) return;
    try {
      await restoreAdminPost(detail.post.id);
      await openDetail(detail.post.id);
      await refreshList();
    } catch (err) {
      setError(err.message || "恢复活动失败");
    }
  }

  function updateFilters(patch) {
    setFilters((prev) => ({ ...prev, ...patch, page: patch.page || 1 }));
  }

  function updateTimeInfo(target, key, value) {
    const setter = target === "create" ? setCreateForm : setForm;
    setter((prev) => ({
      ...prev,
      timeInfo: {
        ...prev.timeInfo,
        [key]: value,
      },
    }));
  }

  function renderPostForm(currentForm, onChange, target) {
    const isFixed = currentForm.timeInfo.mode === "fixed";
    return (
      <>
        <div className="form-grid form-grid-2">
          <label className="field">
            <span>发起人昵称</span>
            <input value={currentForm.authorNickname} onChange={(event) => onChange((prev) => ({ ...prev, authorNickname: event.target.value }))} placeholder="请输入唯一昵称" />
          </label>
          <label className="field">
            <span>活动标题</span>
            <input value={currentForm.title} onChange={(event) => onChange((prev) => ({ ...prev, title: event.target.value }))} />
          </label>
          <label className="field">
            <span>活动分类</span>
            <input value={currentForm.category} onChange={(event) => onChange((prev) => ({ ...prev, category: event.target.value }))} placeholder="如 运动 / 娱乐 / 学习" />
          </label>
          <label className="field">
            <span>活动子类</span>
            <input value={currentForm.subCategory} onChange={(event) => onChange((prev) => ({ ...prev, subCategory: event.target.value }))} placeholder="如 羽毛球 / 桌游 / 编程" />
          </label>
          <label className="field">
            <span>活动地址</span>
            <input value={currentForm.address} onChange={(event) => onChange((prev) => ({ ...prev, address: event.target.value }))} />
          </label>
          <label className="field">
            <span>最大人数</span>
            <input type="number" value={currentForm.maxCount} onChange={(event) => onChange((prev) => ({ ...prev, maxCount: Number(event.target.value) }))} />
          </label>
          <label className="field">
            <span>活动状态</span>
            <select value={currentForm.status} onChange={(event) => onChange((prev) => ({ ...prev, status: event.target.value }))}>
              <option value="open">进行中</option>
              <option value="closed">已关闭</option>
            </select>
          </label>
          <label className="field">
            <span>时间模式</span>
            <select value={currentForm.timeInfo.mode} onChange={(event) => updateTimeInfo(target, "mode", event.target.value)}>
              <option value="range">范围天数</option>
              <option value="fixed">固定时间</option>
            </select>
          </label>
          <label className="field field-span-2">
            <span>{getTimeModeLabel(currentForm.timeInfo.mode)}</span>
            {isFixed ? (
              <input type="datetime-local" value={currentForm.timeInfo.fixedTime} onChange={(event) => updateTimeInfo(target, "fixedTime", event.target.value)} />
            ) : (
              <input type="number" value={currentForm.timeInfo.days} onChange={(event) => updateTimeInfo(target, "days", Number(event.target.value))} />
            )}
          </label>
        </div>

        <label className="field">
          <span>活动介绍</span>
          <textarea rows="4" value={currentForm.description} onChange={(event) => onChange((prev) => ({ ...prev, description: event.target.value }))} />
        </label>
      </>
    );
  }

  const reviewRows = useMemo(() => detail?.reviews || [], [detail]);

  return (
    <div className="management-shell">
      <section className="panel panel-scroll">
        <div className="panel-header">
          <div>
            <h1>活动管理</h1>
            <p>点击整行查看右侧详情；活动内评价直接并入详情区，不再单独做一页。</p>
          </div>
        </div>

        <div className="filter-row filter-row-wrap">
          <input
            className="filter-input"
            placeholder="搜索活动标题、发起人昵称或地址"
            value={filters.keyword}
            onChange={(event) => updateFilters({ keyword: event.target.value })}
          />
          <select value={filters.status} onChange={(event) => updateFilters({ status: event.target.value })}>
            <option value="active">正常</option>
            <option value="deleted">已删除</option>
            <option value="all">全部</option>
            <option value="open">进行中</option>
            <option value="closed">已关闭</option>
          </select>
          <div className="filter-row__spacer" />
          <button className="primary-button" type="button" onClick={() => setShowCreate(true)}>
            创建活动
          </button>
        </div>

        {error ? <div className="error-banner">{error}</div> : null}

        <div className="table-scroll">
          <table className="data-table">
            <thead>
              <tr>
                <th>活动标题</th>
                <th>发起人</th>
                <th>状态</th>
                <th>人数</th>
                <th>时间模式</th>
                <th>创建时间</th>
              </tr>
            </thead>
            <tbody>
              {data.items?.map((item) => (
                <tr
                  key={item.id}
                  className={`${detail?.post?.id === item.id ? "table-row-active" : ""} table-row-clickable`}
                  onClick={() => openDetail(item.id)}
                >
                  <td>{item.title}</td>
                  <td>{item.author?.nickName || "-"}</td>
                  <td>
                    <span className={`status-tag ${getPostStatusTone(item)}`}>{getPostStatusLabel(item)}</span>
                  </td>
                  <td>{item.currentCount}/{item.maxCount}</td>
                  <td>{formatTimeText(item)}</td>
                  <td>{formatDateTime(item.createdAt)}</td>
                </tr>
              ))}
              {!data.items?.length ? (
                <tr>
                  <td colSpan="6" className="empty-cell">当前没有活动数据</td>
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
            <h2>活动详情</h2>
            <p>这里统一查看基础信息、编辑内容、软删除按钮以及本活动收到的评价。</p>
          </div>
        </div>

        {!detail ? (
          <div className="empty-state">点击左侧活动，右侧会展开完整详情。</div>
        ) : (
          <div className="detail-scroll">
            <div className="detail-stack">
              <div className="detail-block">
                <div className="detail-card-header">
                  <div>
                    <div className="detail-label">基础信息</div>
                    <div className="detail-title">{detail.post.title}</div>
                    <div className="muted-text">发起人：{detail.author.nickName}</div>
                  </div>
                  <div className="detail-card-actions">
                    {Number(detail.post.deletedAt) > 0 ? (
                      <button className="secondary-button" type="button" onClick={handleRestore}>恢复活动</button>
                    ) : (
                      <button className="danger-button" type="button" onClick={handleDelete}>软删除</button>
                    )}
                  </div>
                </div>
                <div className="detail-kpis detail-kpis-wrap">
                  <span>状态：{getPostStatusLabel(detail.post)}</span>
                  <span>人数：{detail.participantCount}/{detail.post.maxCount}</span>
                  <span>聊天消息：{detail.chatMessageCount}</span>
                  <span>活动评价：{detail.reviewCount}</span>
                  <span>地址：{detail.post.address}</span>
                  <span>时间：{formatTimeText(detail.post)}</span>
                  <span>待处理履约：{detail.settlementPendingCount}</span>
                </div>
              </div>

              <form className="crud-form" onSubmit={handleSave}>
                <div className="detail-label">编辑活动</div>
                {renderPostForm(form, setForm, "edit")}
                <button className="primary-button">保存活动</button>
              </form>

              <div className="detail-block">
                <div className="detail-label">活动内评价</div>
                <div className="ledger-list">
                  {reviewRows.map((review) => (
                    <div key={review.id} className="ledger-item">
                      <div>
                        <div className="ledger-title">{review.fromNickname} 评价 {review.toNickname}</div>
                        <div className="muted-text">{review.comment || "暂无评价说明"}</div>
                      </div>
                      <div className="ledger-side">
                        <div className="rating-pill">{review.rating} 星</div>
                        <div className="muted-text">{formatDateTime(review.createdAt)}</div>
                      </div>
                    </div>
                  ))}
                  {!reviewRows.length ? <div className="empty-state slim">当前活动还没有评价记录</div> : null}
                </div>
              </div>
            </div>
          </div>
        )}
      </aside>

      <Modal
        open={showCreate}
        title="创建活动"
        onClose={() => setShowCreate(false)}
        width={860}
        footer={
          <>
            <button className="ghost-button" type="button" onClick={() => setShowCreate(false)}>取消</button>
            <button className="primary-button" type="submit" form="create-post-form">确认创建</button>
          </>
        }
      >
        <form id="create-post-form" className="detail-stack" onSubmit={handleCreate}>
          {renderPostForm(createForm, setCreateForm, "create")}
        </form>
      </Modal>
    </div>
  );
}