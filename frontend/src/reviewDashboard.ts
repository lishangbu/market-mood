import type { Dashboard, HistoricalDay, Pool, Stock } from "./types";

/** 复用看盘组件；补录只填可验证的当日字段，不读取实时看板。 */
export function reviewDashboard(
  snapshot: Dashboard | null,
  historical: HistoricalDay | null,
): Dashboard | null {
  if (!historical) return snapshot;
  const limits = historical.limits;
  const pool = (
    id: string,
    title: string,
    stocks: Stock[],
    ready: boolean,
  ): Pool => ({
    id,
    title,
    stocks,
    ready,
    count: stocks.length,
    description: ready
      ? "所选交易日收盘记录；缺失字段显示暂无。"
      : "该日未保存此股票池所需的历史样本。",
    valid_count: stocks.filter((s) => s.price != null && s.change != null)
      .length,
    average: null,
    yang_rate: null,
  });
  if (snapshot)
    return {
      ...snapshot,
      pools:
        limits && snapshot.day === historical.day
          ? [
              ...snapshot.pools,
              pool("historical-sealing", "历史涨停全名单", limits.sealed, true),
              pool("historical-broken", "历史炸板全名单", limits.broken, true),
            ]
          : snapshot.pools,
    };
  const main = (s: Stock) => s.eligible && /^(60|000|001|002|003)/.test(s.code);
  const sealed = limits?.sealed.filter(main) ?? [];
  const broken = limits?.broken.filter(main) ?? [];
  const total = sealed.length + broken.length;
  return {
    top50_rebuild: null,
    yesterday_rebuild: null,
    turnover: null,
    day: historical.day,
    generated_at: historical.imported_at,
    observed_at: null,
    session: "历史复盘",
    sources: [],
    previous_day: null,
    score: { version: "", total: null, factors: [] },
    breadth: null,
    capacity: null,
    sealing: limits
      ? {
          sealed: sealed.length,
          broken: broken.length,
          rate: total ? (sealed.length * 100) / total : null,
          height: null,
        }
      : null,
    pools: [
      ...[
        ["yesterday-cap-first", "昨日十亿首板"],
        ["yesterday-cap-broken", "昨日十亿炸板"],
        ["yesterday-20", "昨日20cm首板"],
        ["yesterday-first", "昨日主板首板"],
        ["yesterday-top50", "昨日成交前50"],
        ["today-cap", "今日十亿首板"],
      ].map(([id, title]) => pool(id, title, [], false)),
      pool("today-sealing", "主板封板与炸板", [...sealed, ...broken], !!limits),
      pool(
        "historical-sealing",
        "历史涨停全名单",
        limits?.sealed ?? [],
        !!limits,
      ),
      pool(
        "historical-broken",
        "历史炸板全名单",
        limits?.broken ?? [],
        !!limits,
      ),
    ],
    notes: [
      "本页今日、昨日均以所选交易日为基准。",
      historical.limitation ?? "仅展示已保存的历史数据。",
    ],
  };
}
