export const num = (v: number | null | undefined, d = 2) =>
  v == null ? "—" : v.toFixed(d);
export const pct = (v: number | null | undefined) =>
  v == null ? "—" : `${v > 0 ? "+" : ""}${v.toFixed(2)}%`;
export const tone = (v: number | null | undefined) =>
  v == null || v === 0 ? "" : v > 0 ? "up" : "down";
export const amount = (v: number | null | undefined) =>
  v == null ? "—" : `${(v / 1e8).toFixed(2)} 亿`;
export const time = (s: string | null | undefined) =>
  s ? s.replace("T", " ").slice(0, 19) : "尚未采集";
