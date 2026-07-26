import { useCallback, useEffect, useRef, useState } from "react";

const KEYWORD_DEBOUNCE_MS = 300;

export function requestDelayForFilters(filters) {
  return filters?.keyword ? KEYWORD_DEBOUNCE_MS : 0;
}

export function fallbackPageForEmptyResult(activeFilters, payload) {
  const page = Number(activeFilters?.page) || 1;
  const pageSize = Number(payload?.pageSize || activeFilters?.pageSize) || 20;
  const total = Math.max(0, Number(payload?.total) || 0);
  const items = Array.isArray(payload?.items) ? payload.items : [];
  const lastPage = Math.max(1, Math.ceil(total / pageSize));
  if (page > 1 && items.length === 0 && page > lastPage) {
    return lastPage;
  }
  return 0;
}

/**
 * Drives a filtered, paginated admin list.
 *
 * Handles the three things every list page needs and each was getting wrong on
 * its own: keyword input is debounced, out-of-order responses are discarded
 * (a slow early request could otherwise overwrite a newer result), and a
 * successful load clears any stale error banner.
 *
 * @param fetcher async (filters) => { items, total, page, pageSize }
 * @param initialFilters starting filter state, must include page/pageSize
 */
export default function usePagedList(fetcher, initialFilters) {
  const [filters, setFilters] = useState(initialFilters);
  const [data, setData] = useState({ items: [], total: 0, page: 1, pageSize: initialFilters.pageSize });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const requestSeq = useRef(0);
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;

  const load = useCallback(async (activeFilters) => {
    const seq = requestSeq.current + 1;
    requestSeq.current = seq;
    setLoading(true);
    try {
      const payload = await fetcherRef.current(activeFilters);
      if (seq !== requestSeq.current) {
        return;
      }
      const fallbackPage = fallbackPageForEmptyResult(activeFilters, payload);
      if (fallbackPage > 0) {
        // A destructive action may remove the last item on the last page.
        // Move to the new last page and let the existing effect load it.
        setFilters((prev) => ({ ...prev, page: fallbackPage }));
        return;
      }
      setData(payload);
      setError("");
    } catch (err) {
      if (seq !== requestSeq.current) {
        return;
      }
      setError(err.message || "加载列表失败");
    } finally {
      if (seq === requestSeq.current) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    // Typing in the keyword box should not fire a request per keystroke;
    // every other filter change applies immediately.
    const delay = requestDelayForFilters(filters);
    const timer = setTimeout(() => load(filters), delay);
    return () => clearTimeout(timer);
  }, [filters, load]);

  const updateFilters = useCallback((patch) => {
    setFilters((prev) => {
      const next = { ...prev, ...patch };
      // Any change other than paging returns to the first page, otherwise a
      // narrowed result set can leave the view on an out-of-range page.
      if (!("page" in patch)) {
        next.page = 1;
      }
      return next;
    });
  }, []);

  const refresh = useCallback(() => load(filters), [load, filters]);

  return { filters, setFilters, updateFilters, data, loading, error, setError, refresh };
}
