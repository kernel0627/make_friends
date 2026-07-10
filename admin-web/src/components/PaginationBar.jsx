export default function PaginationBar({ page = 1, pageSize = 20, total = 0, onPageChange, onPageSizeChange }) {
  const totalPages = Math.max(1, Math.ceil((Number(total) || 0) / (Number(pageSize) || 1)));
  const canPrev = page > 1;
  const canNext = page < totalPages;

  return (
    <div className="pagination-bar">
      <div className="pagination-meta">
        共 <strong>{total}</strong> 条，当前第 <strong>{page}</strong> / <strong>{totalPages}</strong> 页
      </div>
      <div className="pagination-actions">
        <select value={pageSize} onChange={(event) => onPageSizeChange?.(Number(event.target.value))}>
          {[10, 20, 50, 100].map((size) => (
            <option key={size} value={size}>{size} 条 / 页</option>
          ))}
        </select>
        <button className="ghost-button" type="button" disabled={!canPrev} onClick={() => canPrev && onPageChange?.(page - 1)}>
          上一页
        </button>
        <button className="ghost-button" type="button" disabled={!canNext} onClick={() => canNext && onPageChange?.(page + 1)}>
          下一页
        </button>
      </div>
    </div>
  );
}