import { useEffect, useState } from "react";
import { api } from "./api";
import type { Stock } from "./types";
export interface Security {
  code: string;
  name: string;
  exchange: string;
  board: string;
  industry: string | null;
  listing_date: string | null;
  listing_status: string | null;
  provider: string;
}
interface Results {
  items: Security[];
  total: number;
  status: {
    updated_at: string | null;
    error: string | null;
  };
}
export function StockSearch({
  query,
  watched,
  busy,
  onSelect,
  onWatch,
}: {
  query: string;
  watched: Set<string>;
  busy: string | null;
  onSelect: (code: string) => void;
  onWatch: (stock: Stock) => void;
}) {
  const [exchange, setExchange] = useState("");
  const [page, setPage] = useState(1);
  const [cached, setCached] = useState<{ key: string; data: Results } | null>(
    null,
  );
  const resultKey = JSON.stringify([query.trim(), exchange, page]);
  const result = cached?.key === resultKey ? cached.data : null;
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    const update = () => {
      if (document.visibilityState === "visible") setRefresh((n) => n + 1);
    };
    const timer = setInterval(update, 30000);
    document.addEventListener("visibilitychange", update);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", update);
    };
  }, []);
  useEffect(() => {
    setPage(1);
  }, [query, exchange]);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    const timer = setTimeout(async () => {
      try {
        const params = new URLSearchParams({
          q: query.trim(),
          page: String(page),
          page_size: "20",
        });
        if (exchange) params.set("exchange", exchange);
        const data = await api<Results>(
          `/stocks?${params}`,
          "GET",
          controller.signal,
        );
        if (!controller.signal.aborted) {
          setCached({ key: resultKey, data });
          setError("");
        }
      } catch (e) {
        if (!controller.signal.aborted) {
          setError(e instanceof Error ? e.message : "股票目录读取失败");
        }
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    }, 250);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [query, exchange, page, refresh, resultKey]);
  return (
    <section aria-label="全市场股票目录" className="pool-card catalog">
      <div className="section-title">
        <h2>股票目录</h2>
        <select
          aria-label="筛选股票市场"
          value={exchange}
          onChange={(e) => setExchange(e.target.value)}
        >
          <option value="">沪深北全部</option>
          <option value="sh">沪市</option>
          <option value="sz">深市</option>
          <option value="bj">北交所</option>
        </select>
      </div>
      {result && loading && (
        <p role="status" className="muted small">
          正在更新股票目录…
        </p>
      )}
      {result && error && (
        <p role="alert" className="warning">
          更新失败，保留上次结果：{error}
        </p>
      )}
      {!result && loading ? (
        <p role="status">正在搜索股票…</p>
      ) : !result && error ? (
        <p role="alert">{error}</p>
      ) : (
        result && (
          <>
            <p className="muted small">
              {result.total} 只 ·{" "}
              {result.status.updated_at
                ? `目录更新：${result.status.updated_at.slice(0, 16).replace("T", " ")}`
                : "首次目录同步中"}
            </p>
            {result.status.error && (
              <p className="warning">
                目录更新失败，保留上次结果：{result.status.error}
              </p>
            )}
            {!result.items.length && (
              <p>
                {result.status.updated_at
                  ? "没有匹配的股票"
                  : "目录就绪后可按名称或代码搜索"}
              </p>
            )}
            {result.items.map((s) => (
              <div className="stock-row" key={s.code}>
                <button className="stock-main" onClick={() => onSelect(s.code)}>
                  <strong>{s.name}</strong>
                  <small>
                    {s.code} · {s.board}
                  </small>
                </button>
                <button
                  className="text-button"
                  disabled={busy === s.code}
                  onClick={() =>
                    onWatch({
                      ...s,
                      bids: [],
                      asks: [],
                      seal_amount: null,
                      break_count: null,
                      float_cap: null,
                      market_cap: null,
                      turnover_rate: null,
                      first_seal_time: null,
                      last_seal_time: null,
                      limit_stats: null,
                      provider: null,
                      fallback_reasons: [],
                      industry: s.industry ?? "",
                      price: null,
                      change: null,
                      amount: null,
                      open: null,
                      previous_close: null,
                      source_time: null,
                      boards: null,
                      sealed: null,
                      eligible: false,
                    })
                  }
                  aria-label={`${watched.has(s.code) ? "移出自选" : "加入自选"} ${s.name}`}
                >
                  {watched.has(s.code) ? "已自选" : "+ 自选"}
                </button>
              </div>
            ))}
            {result.total > 20 && (
              <div className="section-title">
                <button
                  disabled={page <= 1}
                  onClick={() => setPage((p) => p - 1)}
                >
                  上一页
                </button>
                <span>
                  {page} / {Math.ceil(result.total / 20)}
                </span>
                <button
                  disabled={page * 20 >= result.total}
                  onClick={() => setPage((p) => p + 1)}
                >
                  下一页
                </button>
              </div>
            )}
          </>
        )
      )}
    </section>
  );
}
/**
 * 来源配置编辑器。页面展示的是持久化配置，不用写死的默认顺序覆盖后端。
 * 两个列表分别代表“完整全市场”和“单只股票”，能力与校验条件不同。
 * 上移/停用先改本地草稿；只有保存成功才影响后续采集，失败保持当前草稿。
 * 历史日线和指数分时有独立策略，不随这里的实时行情排序一起变化。
 */
