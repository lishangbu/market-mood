import { useMemo, useState } from "react";
import { Bookmark, ArrowUp, ArrowDown, ArrowUpDown } from "lucide-react";
import type { Stock } from "./types";

const number = (n: number | null | undefined) =>
  n == null ? "暂无" : n.toFixed(2);
const money = (n: number | null | undefined) =>
  n == null ? "暂无" : `${(n / 1e8).toFixed(2)} 亿`;
type SortValue = string | number | readonly number[] | null | undefined;
const columns: {
  label: string;
  value: (s: Stock, index: number) => SortValue;
  ascending?: boolean;
}[] = [
  { label: "序号", value: (_, i) => i, ascending: true },
  { label: "代码", value: (s) => s.code, ascending: true },
  { label: "名称", value: (s) => s.name || s.code, ascending: true },
  { label: "涨跌幅", value: (s) => s.change },
  { label: "最新价", value: (s) => s.price },
  { label: "成交额", value: (s) => s.amount },
  { label: "流通市值", value: (s) => s.float_cap },
  { label: "总市值", value: (s) => s.market_cap },
  { label: "换手率", value: (s) => s.turnover_rate },
  {
    label: "封板资金",
    value: (s) => (s.sealed === true ? s.seal_amount : null),
  },
  { label: "首次封板时间", value: (s) => s.first_seal_time, ascending: true },
  { label: "最后封板时间", value: (s) => s.last_seal_time, ascending: true },
  { label: "炸板次数", value: (s) => s.break_count },
  {
    label: "涨停统计",
    value: (s) =>
      s.limit_stats ? [s.limit_stats.count, s.limit_stats.days] : null,
  },
  { label: "连板数", value: (s) => s.boards },
  { label: "所属行业", value: (s) => s.industry, ascending: true },
];
const collator = new Intl.Collator("zh-CN", { numeric: true });
function compare(a: SortValue, b: SortValue, ascending: boolean) {
  const missing = (v: SortValue) =>
    v == null || v === "" || (typeof v === "number" && !Number.isFinite(v));
  if (missing(a) || missing(b)) return Number(missing(a)) - Number(missing(b));
  const result =
    typeof a === "number" && typeof b === "number"
      ? a - b
      : Array.isArray(a) && Array.isArray(b)
        ? a[0] - b[0] || a[1] - b[1]
        : collator.compare(String(a), String(b));
  return ascending ? result : -result;
}
export function StockTable({
  stocks,
  onSelect,
  watched,
  busy,
  onWatch,
}: {
  stocks: Stock[];
  onSelect: (code: string) => void;
  watched: Set<string>;
  busy: string | null;
  onWatch: (stock: Stock) => void;
}) {
  const [sort, setSort] = useState({ column: 0, ascending: true });
  const rows = useMemo(
    () =>
      stocks
        .map((stock, index) => ({ stock, index }))
        .sort(
          (a, b) =>
            compare(
              columns[sort.column].value(a.stock, a.index),
              columns[sort.column].value(b.stock, b.index),
              sort.ascending,
            ) || a.index - b.index,
        ),
    [stocks, sort],
  );
  return (
    <div
      className="stock-table-scroll"
      tabIndex={0}
      role="region"
      aria-label="股票完整字段"
    >
      <table className="stock-table">
        <thead>
          <tr>
            {columns.map((column, index) => (
              <th
                scope="col"
                key={column.label}
                aria-sort={
                  sort.column === index
                    ? sort.ascending
                      ? "ascending"
                      : "descending"
                    : "none"
                }
              >
                <button
                  className="table-sort"
                  onClick={() =>
                    setSort((current) => ({
                      column: index,
                      ascending:
                        current.column === index
                          ? !current.ascending
                          : !!column.ascending,
                    }))
                  }
                  title={
                    column.label === "涨停统计"
                      ? "先按板数，再按天数排序"
                      : `${column.label}排序`
                  }
                >
                  {column.label}
                  {sort.column === index ? (
                    sort.ascending ? (
                      <ArrowUp size={14} />
                    ) : (
                      <ArrowDown size={14} />
                    )
                  ) : (
                    <ArrowUpDown size={14} />
                  )}
                </button>
              </th>
            ))}
            <th scope="col">自选</th>
          </tr>
        </thead>
        <tbody>
          {rows.map(({ stock: s, index }) => (
            <tr key={s.code}>
              <td>{index + 1}</td>
              <td>{s.code}</td>
              <td>
                <button
                  className="table-stock-name"
                  onClick={() => onSelect(s.code)}
                >
                  {s.name || s.code}
                </button>
              </td>
              <td
                className={
                  s.change == null || s.change === 0
                    ? ""
                    : s.change > 0
                      ? "up"
                      : "down"
                }
              >
                {s.change == null
                  ? "暂无"
                  : `${s.change > 0 ? "+" : ""}${number(s.change)}%`}
              </td>
              <td>{number(s.price)}</td>
              <td>{money(s.amount)}</td>
              <td>{money(s.float_cap)}</td>
              <td>{money(s.market_cap)}</td>
              <td>
                {s.turnover_rate == null
                  ? "暂无"
                  : `${number(s.turnover_rate)}%`}
              </td>
              <td>{s.sealed === true ? money(s.seal_amount) : "—"}</td>
              <td>{s.first_seal_time ?? "暂无"}</td>
              <td>{s.last_seal_time ?? "暂无"}</td>
              <td>{s.break_count ?? "暂无"}</td>
              <td>
                {s.limit_stats
                  ? `${s.limit_stats.days}天${s.limit_stats.count}板`
                  : "暂无"}
              </td>
              <td>{s.boards ?? "暂无"}</td>
              <td>{s.industry || "暂无"}</td>
              <td>
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
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {!stocks.length && <p className="empty">暂无匹配股票</p>}
    </div>
  );
}
/**
 * 股票目录搜索，与行情查询分离：没有当前报价也能搜索基础资料并加入自选。
 * 分页/市场筛选交给后端，不能仅过滤当前一页后冒充全库检索。
 * 请求失败、首次同步和有缓存但同步失败分别展示，避免将缺数据当零只股票。
 */
