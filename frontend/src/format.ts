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
/**
 * 后端JSON的前端契约。金额/价格按后端规范单位直接接收，只在格式化时换算亿元。
 * 字段按当前后端契约提供；null表示业务未知，不能用0替代。
 * 供应商差异在后端适配层处理，React不读取fXX或通达信原始二进制字段。
 */
/** 报价统一为元、成交额为元；盘口委托量为股。缺失报价显式为null，盘口无数据时为空数组。 */
