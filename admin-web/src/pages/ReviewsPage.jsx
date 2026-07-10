import { useEffect, useState } from "react";
import { fetchAdminReviews } from "../lib/api";
import { formatDateTime } from "../lib/format";

export default function ReviewsPage() {
  const [filters, setFilters] = useState({ page: 1, pageSize: 20, keyword: "" });
  const [data, setData] = useState({ items: [] });
  const [error, setError] = useState("");

  useEffect(() => {
    fetchAdminReviews(filters)
      .then(setData)
      .catch((err) => setError(err.message || "加载评价失败"));
  }, [filters]);

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <h1>评价查看</h1>
          <p>评价只读，不在后台直接修改原始评分。</p>
        </div>
      </div>

      <div className="filter-row">
        <input
          placeholder="搜索活动、评价人、被评价人、评论"
          value={filters.keyword}
          onChange={(event) => setFilters((prev) => ({ ...prev, page: 1, keyword: event.target.value }))}
        />
      </div>

      {error ? <div className="error-banner">{error}</div> : null}

      <table className="data-table">
        <thead>
          <tr>
            <th>活动</th>
            <th>评价人</th>
            <th>被评价人</th>
            <th>星级</th>
            <th>评论</th>
            <th>时间</th>
          </tr>
        </thead>
        <tbody>
          {data.items?.map((item) => (
            <tr key={item.id}>
              <td>{item.postTitle}</td>
              <td>{item.fromNickname}</td>
              <td>{item.toNickname}</td>
              <td>{item.rating}</td>
              <td>{item.comment}</td>
              <td>{formatDateTime(item.createdAt)}</td>
            </tr>
          ))}
          {!data.items?.length ? (
            <tr>
              <td colSpan="6" className="empty-cell">暂无评价记录</td>
            </tr>
          ) : null}
        </tbody>
      </table>
    </section>
  );
}
