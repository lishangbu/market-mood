export interface Stock {
  bids: { price: number; shares: number }[];
  asks: { price: number; shares: number }[];
  stale?: boolean; // 仅股票详情接口额外提供，基础Stock不包含此字段。
  code: string;
  name: string;
  industry: string;
  price: number | null;
  change: number | null;
  amount: number | null;
  seal_amount: number | null;
  break_count: number | null;
  float_cap: number | null;
  market_cap: number | null;
  turnover_rate: number | null;
  first_seal_time: string | null;
  last_seal_time: string | null;
  limit_stats: { days: number; count: number } | null;
  open: number | null;
  previous_close: number | null;
  source_time: string | null;
  provider: string | null;
  fallback_reasons: string[];
  boards: number | null;
  sealed: boolean | null;
  eligible: boolean;
}
export interface Pool {
  id: string;
  title: string;
  description: string;
  ready: boolean;
  stocks: Stock[];
  count: number;
  valid_count: number;
  average: number | null;
  yang_rate: number | null;
}
export interface Factor {
  name: string;
  weight: number;
  value: number | null;
  score: number | null;
  unit: string;
  range: [number, number];
}
export interface Sealing {
  sealed: number;
  broken: number;
  rate: number | null;
  height: number | null;
}
export interface Turnover {
  amount: number | null;
  previous_same_amount: number | null;
  volume_change: number | null;
  forecast: number | null;
  source_time: string | null;
  previous_day: string | null;
  provider: string;
  captured_at: string | null;
  stale: boolean;
  error: string | null;
}
export interface Dashboard {
  top50_rebuild: {
    day: string;
    checked: number;
    total: number;
    complete: boolean;
    running: boolean;
    failed: number;
    error: string | null;
  } | null;
  yesterday_rebuild: {
    day: string;
    checked: number;
    total: number;
    excluded: number;
    complete: boolean;
    running: boolean;
    failed: number;
    error: string | null;
  } | null;
  turnover: Turnover | null;
  day: string | null;
  generated_at: string;
  observed_at: string | null;
  session: string;
  sources: {
    name: string;
    captured_at: string | null;
    source_time: string | null;
    provider: string | null;
    fallback_reasons: string[];
    error: string | null;
    stale: boolean;
  }[];
  pools: Pool[];
  score: { version: string; total: number | null; factors: Factor[] };
  breadth: {
    up: number;
    down: number;
    flat: number;
    yang: number;
    yin: number;
    valid: number;
    excluded: number;
    amount: number | null;
    previous_same_amount: number | null;
    volume_change: number | null;
    forecast: number | null;
  } | null;
  capacity: Sealing | null;
  sealing: Sealing | null;
  previous_day: string | null;
  notes: string[];
}

export interface HistoricalDay {
  day: string;
  imported_at: string;
  source: string;
  limitation: string | null;
  shanghai: {
    open: number;
    close: number;
    high: number;
    low: number;
    volume: number;
  };
  shenzhen: {
    open: number;
    close: number;
    high: number;
    low: number;
    volume: number;
  };
  limits: { day: string; sealed: Stock[]; broken: Stock[] } | null;
}
