#!/usr/bin/env node
// Read-only unattended observation. Does not synthesize, backfill, or alter market data.
// 只读轮询服务并记录证据文件；不补录行情，也不写应用数据库。
import { mkdir, appendFile, writeFile } from "node:fs/promises";
import { spawn } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
// 参数依次为服务根地址、观测目录；--once采样一次，默认持续每分钟采样。
const base = process.argv[2] ?? "http://127.0.0.1:8080";
const directory = resolve(process.argv[3] ?? "data/observations");
const once = process.argv.includes("--once");
await mkdir(directory, { recursive: true });
// 信号同时设置停止标记并唤醒定时等待，避免退出要再等满一分钟。
let stopping = false;
let wake;
process.on("SIGINT", () => {
  stopping = true;
  wake?.();
});
process.on("SIGTERM", () => {
  stopping = true;
  wake?.();
});
// 保存本进程每个交易日最近审计时间，同日最多每15分钟触发一次收盘审计。
const audited = new Map();
// 统一使用上海时区判断交易日与15:10审计边界，避免部署机器时区影响判定。
const dateParts = () =>
  Object.fromEntries(
    new Intl.DateTimeFormat("en-GB", {
      timeZone: "Asia/Shanghai",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    })
      .formatToParts(new Date())
      .map((p) => [p.type, p.value]),
  );
// 获取一份看板并记录连通性、响应耗时、数据日和各来源状态；异常也会形成失败证据。
async function sample() {
  const started = Date.now();
  const p = dateParts();
  const day = `${p.year}-${p.month}-${p.day}`;
  const minute = Number(p.hour) * 60 + Number(p.minute);
  let entry = { at: new Date().toISOString(), day, reachable: false };
  try {
    const r = await fetch(`${base}/api/dashboard`, {
      signal: AbortSignal.timeout(15000),
    });
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    const d = await r.json();
    entry = {
      ...entry,
      reachable: true,
      data_day: d.day,
      session: d.session,
      observed_at: d.observed_at,
      sources: d.sources,
      response_ms: Date.now() - started,
      turnover: d.turnover,
      yesterday_rebuild: d.yesterday_rebuild,
      top50_rebuild: d.top50_rebuild,
    };
    // 只审计当天且已收盘的数据，最早15:10执行；服务误返旧日快照时不把它当作今天收盘。
    if (
      d.day === day &&
      d.session === "已收盘" &&
      minute >= 910 &&
      Date.now() - (audited.get(day) ?? 0) > 900000
    ) {
      const path = resolve(directory, `${day}-audit.json`);
      const child = spawn(
        process.execPath,
        [
          fileURLToPath(new URL("./audit-day.mjs", import.meta.url)),
          day,
          base,
          path,
        ],
        { stdio: ["ignore", "ignore", "pipe"] },
      );
      let stderr = "";
      child.stderr.on("data", (s) => (stderr += s));
      const code = await new Promise((res, rej) => {
        child.on("error", rej);
        child.on("close", res);
      });
      entry.audit_exit_code = code;
      if (stderr) entry.audit_error = stderr.slice(0, 2000);
      audited.set(day, Date.now());
    }
  } catch (error) {
    entry.error = String(error);
  }
  // 按天追加JSONL保留时间序列，同时覆盖latest.json供快速查看最新状态。
  await appendFile(
    resolve(directory, `${day}-health.jsonl`),
    JSON.stringify(entry) + "\n",
  );
  await writeFile(
    resolve(directory, "latest.json"),
    JSON.stringify(entry, null, 2) + "\n",
  );
  console.log(
    JSON.stringify({
      at: entry.at,
      reachable: entry.reachable,
      day: entry.data_day,
      audit_exit_code: entry.audit_exit_code,
      error: entry.error,
    }),
  );
}
// 每轮结束后等60秒，不重入采样；停止信号可清除计时器并提前唤醒。
do {
  await sample();
  if (once || stopping) break;
  await new Promise((r) => {
    wake = r;
    const timer = setTimeout(r, 60000);
    wake = () => {
      clearTimeout(timer);
      r();
    };
  });
} while (!stopping);
