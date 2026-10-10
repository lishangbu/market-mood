import { useEffect, useRef, useState } from "react";
import { Bookmark, X } from "lucide-react";
import type { Dashboard, Stock } from "./types";
import type { Security } from "./StockSearch";
import { api } from "./api";
import { num, pct, tone, amount, time } from "./format";
export function Detail({
  archiveStock,
  archiveDay,
  code,
  historical,
  watched,
  busy,
  onWatch,
  onClose,
}: {
  archiveStock?: Stock | null;
  archiveDay?: string;
  code: string;
  historical?: Dashboard | null;
  watched: Set<string>;
  busy: string | null;
  onWatch: (s: Stock) => void;
  onClose: () => void;
}) {
  const [detail, setDetail] = useState<{
      stock: Stock;
      pools: { title: string; description: string }[];
      basic?: Security | null;
    } | null>(null),
    [error, setError] = useState("");
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setError("");
    setDetail(null);
    if (archiveStock) {
      setDetail({ stock: archiveStock, pools: [] });
    } else if (historical) {
      const pools = historical.pools.filter((p) =>
        p.stocks.some((s) => s.code === code),
      );
      const stock = pools.flatMap((p) => p.stocks).find((s) => s.code === code);
      if (stock)
        setDetail({
          stock,
          pools: pools.map((p) => ({
            title: p.title,
            description: p.description,
          })),
        });
      else setError("该历史快照没有保存这只股票的行情");
    } else {
      api<{
        stock: Stock;
        pools: { title: string; description: string }[];
        basic?: Security | null;
      }>(`/stocks/${code}`, "GET", controller.signal)
        .then((d) => {
          if (active) setDetail(d);
        })
        .catch((e) => {
          if (active) setError(e.message);
        });
    }
    const previous = document.activeElement as HTMLElement;
    const old = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    ref.current?.focus();
    function key(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
      if (e.key === "Tab") {
        const nodes = ref.current?.querySelectorAll<HTMLElement>(
          'button:not([disabled]),[tabindex="0"]',
        );
        if (!nodes?.length) return;
        const first = nodes[0],
          last = nodes[nodes.length - 1];
        if (
          e.shiftKey &&
          (document.activeElement === first ||
            document.activeElement === ref.current)
        ) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    }
    document.addEventListener("keydown", key);
    return () => {
      active = false;
      controller.abort();
      document.body.style.overflow = old;
      document.removeEventListener("keydown", key);
      previous?.focus();
    };
  }, [code, historical, archiveStock]);
  return (
    <div
      className="modal-backdrop"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        className="detail-sheet"
        role="dialog"
        aria-modal="true"
        aria-labelledby="detail-title"
        tabIndex={-1}
        ref={ref}
      >
        <div className="section-title">
          <h2 id="detail-title">{detail?.stock.name ?? code}</h2>
          <button
            className="icon-button"
            aria-label="关闭股票详情"
            onClick={onClose}
          >
            <X />
          </button>
        </div>
        {error ? (
          <p role="alert" className="notice error">
            {error}
          </p>
        ) : detail ? (
          <>
            <p className="muted">
              {code} · {detail.stock.industry || "行业暂无数据"}
            </p>
            {(historical || archiveStock) && (
              <p className="notice">
                历史快照 · {archiveStock ? archiveDay : historical?.day} ·
                仅显示当时保存的行情
              </p>
            )}
            <div className="detail-price">
              <strong>{num(detail.stock.price)}</strong>
              <span className={tone(detail.stock.change)}>
                {pct(detail.stock.change)}
              </span>
            </div>
            <div className="volume-grid">
              {[
                ["流通市值", amount(detail.stock.float_cap ?? null)],
                ["总市值", amount(detail.stock.market_cap ?? null)],
                [
                  "换手率",
                  detail.stock.turnover_rate == null
                    ? "暂无"
                    : `${num(detail.stock.turnover_rate)}%`,
                ],
                ["首次封板时间", detail.stock.first_seal_time ?? "暂无"],
                ["最后封板时间", detail.stock.last_seal_time ?? "暂无"],
                [
                  "涨停统计",
                  detail.stock.limit_stats
                    ? `${detail.stock.limit_stats.days}天${detail.stock.limit_stats.count}板`
                    : "暂无",
                ],
                ["连板数", detail.stock.boards ?? "暂无"],
              ].map(([label, value]) => (
                <div key={label}>
                  <span>{label}</span>
                  <strong>{value}</strong>
                </div>
              ))}
              <div>
                <span>炸板次数</span>
                <strong>
                  {detail.stock.break_count == null
                    ? "暂无"
                    : `${detail.stock.break_count}次`}
                </strong>
              </div>
              {detail.stock.sealed === true && (
                <div>
                  <span>封单金额</span>
                  <strong>
                    {detail.stock.seal_amount == null
                      ? "暂无"
                      : amount(detail.stock.seal_amount)}
                  </strong>
                </div>
              )}
              <div>
                <span>成交额</span>
                <strong>{amount(detail.stock.amount)}</strong>
              </div>
              <div>
                <span>今开</span>
                <strong>{num(detail.stock.open)}</strong>
              </div>
              <div>
                <span>昨收</span>
                <strong>{num(detail.stock.previous_close)}</strong>
              </div>
            </div>
            <p className="muted small">
              数据来源：{detail.stock.provider ?? "待核验"} · 行情时间：
              {time(detail.stock.source_time)}
              {detail.stock.stale && (
                <span className="warning"> · 行情已过期或尚未取得</span>
              )}
            </p>
            {!!detail.stock.fallback_reasons.length && (
              <p className="muted small">
                已切换备用源：{detail.stock.fallback_reasons.join("；")}
              </p>
            )}
            <button
              className="primary-button"
              disabled={busy === code}
              onClick={() => onWatch(detail.stock)}
            >
              <Bookmark size={18} />
              {watched.has(code) ? "移出自选" : "加入自选"}
            </button>
            {detail.basic && (
              <p className="muted small">
                {detail.basic.board} · 行业：{detail.basic.industry ?? "暂无"} ·
                上市日期：{detail.basic.listing_date ?? "暂无"} · 档案来源：
                {detail.basic.provider}
              </p>
            )}
            <h3>股票池与指标口径</h3>
            {detail.pools.length ? (
              detail.pools.map((p) => (
                <div className="detail-pool" key={p.title}>
                  <strong>{p.title}</strong>
                  <p>{p.description}</p>
                </div>
              ))
            ) : (
              <p className="muted">当前未出现在已建立的股票池中</p>
            )}
            <p className="muted small">
              阳线：现价 ≥ 今开；涨跌幅基于昨收。缺失或停牌行情不参与池内平均。
            </p>
          </>
        ) : (
          <p className="empty">正在读取行情…</p>
        )}
      </div>
    </div>
  );
}
