import { useEffect, useState } from "react";
import { fetchAdminUserCreditLedger, fetchAdminUsers } from "../lib/api";
import { formatDateTime, formatDelta } from "../lib/format";

export default function LedgersPage() {
  const [users, setUsers] = useState([]);
  const [selectedUserId, setSelectedUserId] = useState("");
  const [ledger, setLedger] = useState([]);
  const [error, setError] = useState("");

  useEffect(() => {
    fetchAdminUsers({ page: 1, pageSize: 100 })
      .then((payload) => {
        setUsers(payload.items || []);
        if (payload.items?.[0]?.id) {
          setSelectedUserId(payload.items[0].id);
        }
      })
      .catch((err) => setError(err.message || "加载用户列表失败"));
  }, []);

  useEffect(() => {
    if (!selectedUserId) {
      return;
    }
    fetchAdminUserCreditLedger(selectedUserId, { page: 1, pageSize: 50 })
      .then((payload) => setLedger(payload.items || []))
      .catch((err) => setError(err.message || "加载信誉流水失败"));
  }, [selectedUserId]);

  return (
    <div className="two-column">
      <section className="panel">
        <div className="panel-header">
          <div>
            <h1>信誉流水</h1>
            <p>这里看的是真正的信用变化证据，不是原始评价星级。</p>
          </div>
        </div>

        <div className="user-list">
          {users.map((user) => (
            <button
              key={user.id}
              className={`user-list-item${selectedUserId === user.id ? " user-list-item-active" : ""}`}
              onClick={() => setSelectedUserId(user.id)}
            >
              <span>{user.nickName}</span>
              <span className="muted-text">信誉 {user.creditScore}</span>
            </button>
          ))}
        </div>
      </section>

      <aside className="panel detail-panel">
        <div className="panel-header">
          <div>
            <h2>流水明细</h2>
            <p>选一个用户，就能看最近的信用变化。</p>
          </div>
        </div>
        {error ? <div className="error-banner">{error}</div> : null}
        <div className="ledger-list">
          {ledger.map((item) => (
            <div key={`${item.postId}_${item.createdAt}_${item.sourceType}`} className="ledger-item">
              <div>
                <div className="ledger-title">{item.postTitle || item.sourceType}</div>
                <div className="muted-text">{item.note || item.sourceType}</div>
              </div>
              <div className="ledger-side">
                <div className={Number(item.delta) >= 0 ? "delta-positive" : "delta-negative"}>
                  {formatDelta(item.delta)}
                </div>
                <div className="muted-text">{formatDateTime(item.createdAt)}</div>
              </div>
            </div>
          ))}
          {!ledger.length ? <div className="empty-state slim">暂无信誉流水</div> : null}
        </div>
      </aside>
    </div>
  );
}
