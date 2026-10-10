/**
 * 页面状态与展示入口。
 *
 * 1. 看盘每10秒读后端缓存，不等于每10秒让供应商重新采集；行情时间来自后端。
 * 2. 历史详情只读历史快照，不能请求今日报价补旧记录；未知字段以null表示。
 * 3. amount是成交额，seal_amount是封单金额；封板状态与涨幅均不是金额字段。
 * 4. 昨日池的成员在后端固定，前端搜索/排序只影响可见列表，不重算汇总家数。
 * 5. 数据错误时保留上一屏，明确标明失败，不能让空响应抹去可用历史观测。
 * 6. 语音为用户主动开启的本地行为，离开前台停止，不因刷新重复播报同一事件。
 */
import React, { useEffect, useMemo, useState } from "react";
import {
  Activity,
  Bookmark,
  ChartNoAxesCombined,
  ChevronDown,
  Clock,
  Search,
  Volume2,
  VolumeX,
  RefreshCw,
} from "lucide-react";
import type { Dashboard, Stock } from "./types";
import { StockSearch } from "./StockSearch";
import { TaskStatus } from "./TaskStatus";
import { SourceSettings } from "./SourceSettings";
import { DashboardView } from "./DashboardView";
import { reviewDashboard } from "./reviewDashboard";
import { StockList } from "./StockList";
import { Trend } from "./Trend";
import { Detail } from "./Detail";
import { useVoice } from "./useVoice";
import { useReview } from "./useReview";
import { usePollingResource } from "./usePollingResource";
import { api } from "./api";
import { num, time } from "./format";
export function App() {
  const [tab, setTab] = useState("看盘"),
    [selected, setSelected] = useState<string | null>(null),
    [toast, setToast] = useState(""),
    [sourceOpen, setSourceOpen] = useState(false);
  const [archiveStock, setArchiveStock] = useState<Stock | null>(null);
  const {
    days,
    day,
    setDay,
    history,
    historical,
    backfill,
    close,
    historyError,
    historyLoading,
  } = useReview(tab);
  const [query, setQuery] = useState(""),
    [busy, setBusy] = useState<string | null>(null);
  const dashboard = usePollingResource<Dashboard>("/dashboard");
  const watchlist = usePollingResource<{
    stocks: Stock[];
    error: string | null;
  }>("/watchlist");
  const { data, error, loading } = dashboard;
  const watch = watchlist.data?.stocks ?? [];
  const watchError = watchlist.error || watchlist.data?.error || "";
  const watched = useMemo(() => new Set(watch.map((s) => s.code)), [watch]);
  const refresh = async () => {
    await Promise.all([dashboard.refresh(), watchlist.refresh()]);
  };
  const { voice, toggleVoice } = useVoice(data, error, setToast);
  useEffect(() => {
    if (!toast) return;
    const id = setTimeout(() => setToast(""), 3500);
    return () => clearTimeout(id);
  }, [toast]);
  async function toggleWatch(s: Stock) {
    if (busy) return;
    setBusy(s.code);
    try {
      await api(`/watchlist/${s.code}`, watched.has(s.code) ? "DELETE" : "PUT");
      setToast(watched.has(s.code) ? "已移出自选" : "已加入自选");
      await refresh();
    } catch (e) {
      setToast((e as Error).message);
    } finally {
      setBusy(null);
    }
  }
  const current = useMemo(() => {
    if (tab !== "复盘") return data;
    return reviewDashboard(close ?? history.at(-1) ?? null, historical);
  }, [tab, data, close, history, historical]);
  return (
    <>
      <div className="app-shell">
        <header className="topbar">
          <div className="brand">
            <span className="brand-symbol">
              <Activity size={22} />
            </span>
            <div>
              <span className="eyebrow">MARKET MOOD</span>
              <h1>市场情绪查看器</h1>
            </div>
          </div>
          <button
            className={`icon-button ${voice ? "active" : ""}`}
            onClick={toggleVoice}
            aria-label={voice ? "关闭语音播报" : "开启语音播报"}
            aria-pressed={voice}
          >
            {voice ? <Volume2 size={20} /> : <VolumeX size={20} />}
          </button>
        </header>
        <div className="session-row">
          <span>
            <i className={data?.session === "交易中" ? "live-dot" : "dot"} />
            {tab === "复盘" ? "历史复盘" : (data?.session ?? "连接中")}
          </span>
          <span>
            {tab === "复盘"
              ? day || "暂无记录"
              : (data?.day ?? "等待交易日数据")}
          </span>
        </div>
        <main>
          <TaskStatus />
          <div className="page-heading compact">
            {tab !== "复盘" && (
              <button
                className="icon-button"
                aria-label="刷新数据"
                onClick={() => void refresh()}
              >
                <RefreshCw size={18} />
              </button>
            )}
          </div>
          {tab !== "复盘" && error && (
            <div className="notice error" role="alert">
              连接暂时中断：{error}。
              {data ? "已保留上次数据。" : "请确认本地服务正在运行。"}
              <button onClick={() => void refresh()}>重试</button>
            </div>
          )}
          {tab === "看盘" && loading && !data ? (
            <div className="skeleton" aria-label="正在加载行情">
              <div />
              <div />
              <div />
            </div>
          ) : (
            <>
              {tab === "看盘" && (
                <>
                  <button
                    className="source-summary"
                    onClick={() => setSourceOpen(!sourceOpen)}
                    aria-expanded={sourceOpen}
                  >
                    <span>
                      <Clock size={14} />
                      {!data ||
                      error ||
                      data.sources.some((s) => s.error || s.stale)
                        ? "部分行情待更新"
                        : "行情已同步"}{" "}
                      · 查看数据时间
                    </span>
                    <ChevronDown size={16} />
                  </button>
                  <SourceSettings />
                  {sourceOpen && (
                    <section className="source-panel">
                      {data?.sources.map((s) => (
                        <div key={s.name}>
                          <strong>{s.name}</strong>
                          <p>数据来源：{s.provider ?? "待核验"}</p>
                          {!!s.fallback_reasons.length && (
                            <p className="muted small">
                              已切换备用源：{s.fallback_reasons.join("；")}
                            </p>
                          )}
                          <p>行情时间：{time(s.source_time)}</p>
                          <p>采集时间：{time(s.captured_at)}</p>
                          {(s.error || s.stale) && (
                            <p className="warning">
                              {s.error ?? "数据已过期，请勿当作实时行情"}
                            </p>
                          )}
                        </div>
                      ))}
                    </section>
                  )}
                  {current && (
                    <DashboardView
                      data={current}
                      watched={watched}
                      busy={busy}
                      onWatch={toggleWatch}
                      onSelect={setSelected}
                    />
                  )}
                </>
              )}
              {tab === "复盘" && (
                <>
                  {backfill && (
                    <p className="notice">
                      最近5个交易日补录：{backfill.completed} / 5 日
                      {backfill.running ? " · 进行中" : ""}
                      {backfill.error ? ` · ${backfill.error}` : ""}
                      。日线补录不包含盘中曲线。
                    </p>
                  )}
                  <div className="history-toolbar">
                    <label htmlFor="day">交易日</label>
                    <select
                      id="day"
                      value={day}
                      onChange={(e) => setDay(e.target.value)}
                      disabled={!days.length}
                    >
                      {!days.length && <option value="">暂无记录</option>}
                      {days.map((d) => (
                        <option key={d}>{d}</option>
                      ))}
                    </select>
                  </div>
                  <p className="muted small">
                    本页“今日”“昨日”均以所选交易日为基准，个股详情展示当时记录。
                  </p>
                  {historyError && (
                    <p role="alert" className="notice error">
                      {historyError}
                    </p>
                  )}
                  {historical && (
                    <section className="card breadth">
                      <h3>{historical.day} · 补录日线</h3>
                      <div className="metric-grid">
                        {(
                          [
                            ["上证指数", historical.shanghai],
                            ["深证成指", historical.shenzhen],
                          ] as const
                        ).map(([name, b]) => (
                          <div className="metric" key={name}>
                            <span>{name}</span>
                            <strong>{num(b.close)}</strong>
                            <small>
                              开 {num(b.open)} · 高 {num(b.high)} · 低{" "}
                              {num(b.low)}
                            </small>
                          </div>
                        ))}
                      </div>
                      <p className="small muted">{historical.limitation}</p>
                      <p className="small muted">
                        来源：{historical.source} · 补录时间{" "}
                        {time(historical.imported_at)}
                      </p>
                    </section>
                  )}
                  {historyLoading ? (
                    <p className="empty">正在读取当日记录…</p>
                  ) : history.length || close ? (
                    <>
                      {history.length ? (
                        <Trend history={history} />
                      ) : (
                        <p className="notice">
                          当日仅有收盘快照，未采集盘中趋势。
                        </p>
                      )}
                      <p className="muted small">
                        {close ? "收盘快照采集时间" : "最后盘中记录"}{" "}
                        {time(current?.generated_at)} · 曲线断开表示采集缺口
                      </p>
                    </>
                  ) : historical ? null : (
                    <Empty
                      icon={<ChartNoAxesCombined />}
                      title="你的复盘记录，从这里开始"
                      text="交易时段保持电脑上的服务运行，系统会自动记录市场变化。尚未采集的历史不会补成模拟数据。"
                    />
                  )}
                  {!historyLoading && current && (
                    <DashboardView
                      key={day}
                      data={current}
                      review
                      watched={watched}
                      busy={busy}
                      onWatch={toggleWatch}
                      onSelect={setSelected}
                    />
                  )}
                </>
              )}
              {tab === "自选" && (
                <>
                  <form
                    className="search-box"
                    onSubmit={(e) => {
                      e.preventDefault();
                      if (/^\d{6}$/.test(query)) setSelected(query);
                    }}
                  >
                    <Search size={18} />
                    <input
                      aria-label="搜索股票名称或代码"
                      placeholder="搜索股票名称 / 代码"
                      value={query}
                      onChange={(e) => setQuery(e.target.value)}
                    />
                    {/^\d{6}$/.test(query) && (
                      <button type="submit">查看</button>
                    )}
                  </form>
                  {watchError && (
                    <p className="notice warning">
                      自选行情更新失败，保留最后数据：{watchError}
                    </p>
                  )}
                  <section className="watch-stocks" aria-label="我的自选">
                    {watch.length ? (
                      <StockList
                        stocks={watch.filter((s) =>
                          `${s.code}${s.name}`.includes(query),
                        )}
                        watched={watched}
                        busy={busy}
                        onWatch={toggleWatch}
                        onSelect={setSelected}
                      />
                    ) : (
                      <Empty
                        icon={<Bookmark />}
                        title="把关注留给重要的股票"
                        text="在下方股票目录按名称或代码搜索，点击加入自选。"
                      />
                    )}
                  </section>
                  <StockSearch
                    query={query}
                    watched={watched}
                    busy={busy}
                    onSelect={setSelected}
                    onWatch={toggleWatch}
                  />
                </>
              )}
            </>
          )}
          <footer>
            MARKET MOOD <span>观察 · 记录 · 复盘</span>
          </footer>
        </main>
      </div>
      <nav className="bottom-nav" aria-label="主导航">
        {[
          { label: "看盘", Icon: Activity },
          { label: "复盘", Icon: ChartNoAxesCombined },
          { label: "自选", Icon: Bookmark },
        ].map(({ label, Icon }) => (
          <button
            key={label}
            onClick={() => setTab(label)}
            aria-current={tab === label ? "page" : undefined}
            className={tab === label ? "selected" : ""}
          >
            <Icon size={21} />
            <span>{label}</span>
          </button>
        ))}
      </nav>
      {selected && (
        <Detail
          code={selected}
          historical={tab === "复盘" ? current : null}
          archiveStock={tab === "复盘" ? archiveStock : null}
          archiveDay={historical?.day}
          watched={watched}
          busy={busy}
          onWatch={toggleWatch}
          onClose={() => {
            setSelected(null);
            setArchiveStock(null);
          }}
        />
      )}
      {toast && (
        <div role="status" className="toast">
          {toast}
        </div>
      )}
    </>
  );
}
/** 宽度和指数量能可能异步就绪，成交额区域不能被breadth为空的分支隐藏。 */
function Empty({
  icon,
  title,
  text,
}: {
  icon: React.ReactNode;
  title: string;
  text: string;
}) {
  return (
    <section className="empty-state">
      <div>{icon}</div>
      <h3>{title}</h3>
      <p>{text}</p>
    </section>
  );
}
