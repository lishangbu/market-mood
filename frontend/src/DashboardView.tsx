import { useMemo, useState } from "react";
import { ChevronDown, Info, Search, Table2, List } from "lucide-react";
import type { Dashboard, Pool, Stock } from "./types";
import { StockTable } from "./StockTable";
import { StockList } from "./StockList";
import { num, pct, tone, amount, time } from "./format";
export function DashboardView({
  data,
  review = false,
  watched,
  busy,
  onWatch,
  onSelect,
}: {
  data: Dashboard;
  review?: boolean;
  watched: Set<string>;
  busy: string | null;
  onWatch: (s: Stock) => void;
  onSelect: (s: string) => void;
}) {
  const [factors, setFactors] = useState(false);
  const b = data.breadth,
    total = b ? b.up + b.down + b.flat : 0;
  return (
    <>
      <section className="hero-card">
        <div className="hero-label">
          <span>容量情绪指数</span>
          <span className="tag">
            {review ? "历史记录" : data.score.version.toUpperCase()} · 观察指标
          </span>
        </div>
        <div className="hero-content">
          <div className="score">
            <strong>{num(data.score.total, 0)}</strong>
            <span>/ 100</span>
          </div>
          <div className="score-caption">
            <span>
              {data.score.total == null
                ? review
                  ? "当日评分未记录"
                  : "等待完整样本"
                : review
                  ? "当日市场温度"
                  : "今日市场温度"}
            </span>
            <p>
              {data.score.total == null
                ? review
                  ? "缺少完整历史样本，暂无评分"
                  : "数据齐备后生成评分"
                : "八个维度，读懂容量资金表现"}
            </p>
          </div>
        </div>
        <div className="score-track">
          <span style={{ width: `${data.score.total ?? 0}%` }} />
        </div>
        <div className="hero-bottom">
          <span>
            弱势 <span className="muted">← 情绪刻度 →</span> 强势
          </span>
          <button onClick={() => setFactors(!factors)} aria-expanded={factors}>
            评分明细
            <ChevronDown size={15} />
          </button>
        </div>
        {factors && (
          <div className="factor-list">
            {data.score.factors.map((f) => (
              <div key={f.name}>
                <div>
                  <span>{f.name}</span>
                  <small>
                    {num(f.value)}
                    {f.value == null ? "" : f.unit} · {f.range[0]}–{f.range[1]}
                    {f.unit} 线性计分
                  </small>
                </div>
                <strong>
                  {num(f.score, 1)}
                  <small> / {f.weight}</small>
                </strong>
              </div>
            ))}
            <p>
              超出区间按零分或满分计算。任一因子缺失时不生成总分；参数尚未回测。
            </p>
          </div>
        )}
      </section>
      <div className="metric-grid">
        <Metric
          title="主板封板率"
          value={
            data.sealing?.rate == null ? "—" : `${num(data.sealing.rate, 1)}%`
          }
          hint={
            data.sealing
              ? `封 ${data.sealing.sealed} / 炸 ${data.sealing.broken}`
              : review
                ? "当日封炸板数据未记录"
                : "等待封炸板数据"
          }
        />
        <Metric
          title="十亿首板封板率"
          value={
            data.capacity?.rate == null ? "—" : `${num(data.capacity.rate, 1)}%`
          }
          hint={
            data.capacity
              ? `封 ${data.capacity.sealed} / 炸 ${data.capacity.broken} · 容量高度 ${data.capacity.height ?? "—"} 板`
              : review
                ? "当日容量样本未记录"
                : "等待完整昨日样本"
          }
        />
      </div>
      <section className="card breadth">
        <div className="section-title">
          <h3>市场宽度与量能</h3>
          <span className="muted small">沪深 A 股</span>
        </div>
        {b ? (
          <>
            <div className="breadth-numbers">
              <div>
                <span className="up">上涨</span>
                <strong className="up">{b.up}</strong>
              </div>
              <div>
                <span>平盘</span>
                <strong>{b.flat}</strong>
              </div>
              <div>
                <span className="down">下跌</span>
                <strong className="down">{b.down}</strong>
              </div>
            </div>
            <div
              className="breadth-bar"
              role="img"
              aria-label={`上涨${b.up}家，平盘${b.flat}家，下跌${b.down}家`}
            >
              <i style={{ width: `${total ? (b.up / total) * 100 : 0}%` }} />
              <i style={{ width: `${total ? (b.flat / total) * 100 : 0}%` }} />
              <i style={{ width: `${total ? (b.down / total) * 100 : 0}%` }} />
            </div>
            <p className="muted small">
              阳线 {b.yang} / 阴线 {b.yin} · 有效 {b.valid} 家 · 排除{" "}
              {b.excluded} 家
            </p>
          </>
        ) : (
          <p className="empty">
            {review
              ? "当日未保存完整市场样本，宽度与量能暂无。"
              : "全市场行情尚未齐备，暂不生成宽度与量能统计。"}
          </p>
        )}
        <div className="volume-grid">
          <div>
            <span>两市成交</span>
            <strong>
              {amount(data.turnover ? data.turnover.amount : b?.amount)}
            </strong>
          </div>
          <div>
            <span>较昨同刻</span>
            <strong
              className={tone(
                data.turnover ? data.turnover.volume_change : b?.volume_change,
              )}
            >
              {pct(
                data.turnover ? data.turnover.volume_change : b?.volume_change,
              )}
            </strong>
          </div>
          <div>
            <span>预计全天</span>
            <strong>
              {amount(data.turnover ? data.turnover.forecast : b?.forecast)}
            </strong>
          </div>
        </div>
        {data.turnover ? (
          <>
            <p className="muted small">
              沪深指数成交额合计 · {data.turnover.provider || "等待数据源"} ·
              数据时间 {time(data.turnover.source_time)}
            </p>
            {data.turnover.previous_same_amount != null && (
              <p className="muted small">
                上一交易日 {data.turnover.previous_day} 同刻{" "}
                {amount(data.turnover.previous_same_amount)} ·
                预计全天按昨日成交进度估算
              </p>
            )}
            {(data.turnover.stale || data.turnover.error) && (
              <p className="notice">
                {data.turnover.error || "成交额数据待更新"}
                ，暂不计算同刻变化与全天估算。
              </p>
            )}
            {data.turnover.previous_same_amount == null && (
              <p className="muted small">等待上一交易日对应分钟的沪深成交额</p>
            )}
          </>
        ) : (
          <p className="muted small">
            历史快照按当时股票样本统计，未保存沪深指数分时成交额。
          </p>
        )}
      </section>
      {data.yesterday_rebuild && !data.yesterday_rebuild.complete && (
        <p className="muted small">
          {data.yesterday_rebuild.running
            ? `正在补齐昨日股票池：${data.yesterday_rebuild.checked} / ${data.yesterday_rebuild.total}`
            : "昨日股票池历史数据待补齐"}
        </p>
      )}
      {data.yesterday_rebuild?.complete && (
        <p className="muted small">
          昨日封炸板池已按历史日线重建；按普通涨跌停规则识别，特殊交易规则样本排除。
        </p>
      )}
      <div className="section-heading">
        <h3>追踪资金的去向</h3>
        <span>股票池 · 点击展开</span>
      </div>
      {data.top50_rebuild &&
        !data.pools.find((p) => p.id === "yesterday-top50")?.ready && (
          <p className="muted small" role="status">
            {data.top50_rebuild.running
              ? `正在补齐昨日成交前50：${data.top50_rebuild.checked} / ${data.top50_rebuild.total}`
              : data.top50_rebuild.error || "昨日成交前50数据待补齐"}
          </p>
        )}
      <div className="pool-grid">
        {data.pools.map((p) => (
          <PoolCard
            key={p.id}
            pool={p}
            watched={watched}
            busy={busy}
            onWatch={onWatch}
            onSelect={onSelect}
          />
        ))}
      </div>
      <div className="notes">
        <Info size={16} />
        <div>
          {data.notes.map((n) => (
            <p key={n}>{n}</p>
          ))}
        </div>
      </div>
    </>
  );
}
function Metric({
  title,
  value,
  hint,
}: {
  title: string;
  value: string;
  hint: string;
}) {
  return (
    <section className="card metric">
      <span>{title}</span>
      <strong>{value}</strong>
      <small>{hint}</small>
    </section>
  );
}
/** 池头汇总使用完整pool，搜索后的stocks只供列表显示；当日封炸池显示家数。 */
function PoolCard({
  pool,
  watched,
  busy,
  onWatch,
  onSelect,
}: {
  pool: Pool;
  watched: Set<string>;
  busy: string | null;
  onWatch: (s: Stock) => void;
  onSelect: (s: string) => void;
}) {
  const showSealingCounts =
    pool.id === "today-sealing" ||
    pool.id === "today-cap" ||
    pool.id.startsWith("historical-");
  const [table, setTable] = useState(true);
  const [open, setOpen] = useState(false),
    [query, setQuery] = useState(""),
    [sort, setSort] = useState("change");
  const stocks = useMemo(
    () =>
      pool.stocks
        .filter((s) => `${s.name}${s.code}${s.industry}`.includes(query))
        .sort((a, b) =>
          sort === "name"
            ? a.code.localeCompare(b.code)
            : (b[sort as "change" | "amount"] ?? -Infinity) -
              (a[sort as "change" | "amount"] ?? -Infinity),
        ),
    [pool.stocks, query, sort],
  );
  return (
    <section className={`card pool-card ${open ? "expanded" : ""}`}>
      <button
        className="pool-header"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <div>
          <h4>{pool.title}</h4>
          <span>
            {pool.ready
              ? `${pool.count} 家 · 有效行情 ${pool.valid_count} 家`
              : "样本尚未建立"}
          </span>
        </div>
        <div className="pool-value">
          {showSealingCounts ? (
            <div className="pool-sealing-counts">
              <span>
                封板{" "}
                {pool.ready
                  ? pool.stocks.filter((s) => s.sealed === true).length
                  : "—"}{" "}
                家
              </span>
              <span>
                炸板{" "}
                {pool.ready
                  ? pool.stocks.filter((s) => s.sealed === false).length
                  : "—"}{" "}
                家
              </span>
            </div>
          ) : (
            <strong className={tone(pool.average)}>{pct(pool.average)}</strong>
          )}
          <ChevronDown size={18} className={open ? "rotated" : ""} />
        </div>
      </button>
      {open && (
        <div className="pool-body">
          <p className="muted small">{pool.description}</p>
          <div className="pool-tools">
            <label className="search-box">
              <Search size={16} />
              <input
                aria-label={`搜索${pool.title}`}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="名称 / 代码 / 行业"
              />
            </label>
            {!table && (
              <select
                aria-label={`${pool.title}排序`}
                value={sort}
                onChange={(e) => setSort(e.target.value)}
              >
                <option value="change">涨幅优先</option>
                <option value="amount">成交额优先</option>
                <option value="name">代码顺序</option>
              </select>
            )}
            <button
              className="icon-button"
              title={table ? "列表视图" : "完整字段表格"}
              aria-label={table ? "列表视图" : "完整字段表格"}
              onClick={() => setTable(!table)}
            >
              {table ? <List size={18} /> : <Table2 size={18} />}
            </button>
          </div>
          {table ? (
            <StockTable
              stocks={stocks}
              onSelect={onSelect}
              watched={watched}
              busy={busy}
              onWatch={onWatch}
            />
          ) : (
            <StockList
              stocks={stocks}
              watched={watched}
              busy={busy}
              onWatch={onWatch}
              onSelect={onSelect}
            />
          )}
          {!pool.ready && (
            <p className="muted small">
              当前列表可能不完整，汇总指标待样本齐备后显示。
            </p>
          )}
        </div>
      )}
    </section>
  );
}
/** 股票行严格区分状态、成交额、封单；炸板状态不显示当前封单，缺字段显示暂无。 */
