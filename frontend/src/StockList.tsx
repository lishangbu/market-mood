import { Bookmark } from "lucide-react";
import type { Stock } from "./types";
import { pct, tone, amount } from "./format";
export function StockList({
  stocks,
  watched,
  busy,
  onWatch,
  onSelect,
}: {
  stocks: Stock[];
  watched: Set<string>;
  busy: string | null;
  onWatch: (s: Stock) => void;
  onSelect: (s: string) => void;
}) {
  return (
    <div className="stock-list">
      {stocks.length ? (
        stocks.map((s) => (
          <div className="stock-row" key={s.code}>
            <button className="stock-main" onClick={() => onSelect(s.code)}>
              <div>
                <strong>{s.name || s.code}</strong>
                <small>
                  {s.code} {s.industry}
                  {s.stale && <span className="warning"> · 行情待更新</span>}
                </small>
              </div>
              <div className="stock-price">
                <strong className={tone(s.change)}>{pct(s.change)}</strong>
                {s.sealed != null && (
                  <small>{s.sealed ? "封板" : "炸板"}</small>
                )}
                {s.sealed === true && (
                  <small>
                    封单金额{" "}
                    {s.seal_amount == null ? "暂无" : amount(s.seal_amount)}
                  </small>
                )}
                {s.sealed != null && (
                  <small>
                    炸板次数{" "}
                    {s.break_count == null ? "暂无" : `${s.break_count}次`}
                  </small>
                )}
                <small>成交额 {amount(s.amount)}</small>
              </div>
            </button>
            <button
              className={`icon-button bookmark ${watched.has(s.code) ? "active" : ""}`}
              disabled={busy === s.code}
              aria-label={`${watched.has(s.code) ? "移出" : "加入"}自选 ${s.name}`}
              onClick={() => onWatch(s)}
            >
              <Bookmark
                size={18}
                fill={watched.has(s.code) ? "currentColor" : "none"}
              />
            </button>
          </div>
        ))
      ) : (
        <p className="empty">暂无匹配股票</p>
      )}
    </div>
  );
}
