import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { spawn } from "node:child_process";
// 构造完整交易分钟、合法家数与封板率的最小样本，各测试复制后仅破坏所关注条件。
function fixture() {
  const snapshots = [];
  for (let m = 570; m <= 900; m++) {
    if (m > 690 && m < 780) continue;
    const t = `2026-09-07T${String(Math.floor(m / 60)).padStart(2, "0")}:${String(m % 60).padStart(2, "0")}:00+08:00`;
    snapshots.push({
      day: "2026-09-07",
      observed_at: t,
      generated_at: t,
      sources: [
        { stale: false, error: null },
        { stale: false, error: null },
      ],
      score: { total: null, factors: [] },
      sealing: { sealed: 8, broken: 2, rate: 80 },
      breadth: { up: 6, down: 3, flat: 1, yang: 5, yin: 5, valid: 10 },
    });
  }
  return { snapshots, close: snapshots.at(-1) };
}
// 临时HTTP服务返回指定样本，运行真实审计子进程并同时读取退出码/报告；finally释放端口。
async function audit(review) {
  const server = createServer((req, res) => {
    res.setHeader("content-type", "application/json");
    res.end(JSON.stringify(review));
  });
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  try {
    const child = spawn(process.execPath, [
      "scripts/audit-day.mjs",
      "2026-09-07",
      `http://127.0.0.1:${server.address().port}`,
    ]);
    let output = "";
    child.stdout.on("data", (s) => (output += s));
    const code = await new Promise((r) => child.on("exit", r));
    return { code, report: JSON.parse(output) };
  } finally {
    await new Promise((r) => server.close(r));
  }
}
// 正例只证明审计规则可接受自洽且全覆盖的记录。
test("full synthetic coverage passes the audit logic only", async () => {
  const { code, report } = await audit(fixture());
  assert.equal(code, 0);
  assert.equal(report.coverage, 1);
});
// 仅有合法收盘不能代替完整盘中观测覆盖。
test("close-only day cannot pass as a complete trading day", async () => {
  const f = fixture();
  f.snapshots = [];
  const { code, report } = await audit(f);
  assert.equal(code, 1);
  assert.equal(report.close_valid, true);
  assert.equal(report.passed, false);
});
// 即使分钟覆盖完整，错误封板率也必须使审计失败。
test("incorrect ratio fails even with complete coverage", async () => {
  const f = fixture();
  f.snapshots[5].sealing.rate = 99;
  const { code, report } = await audit(f);
  assert.equal(code, 1);
  assert.ok(report.issues.some((s) => s.includes("ratio differs")));
});

// 重复快照不能增加有效覆盖率且应单独报错。
test("duplicate observations are rejected", async () => {
  const f = fixture();
  f.snapshots.push(f.snapshots[0]);
  const { code, report } = await audit(f);
  assert.equal(code, 1);
  assert.ok(report.issues.some((s) => s.includes("duplicate source minute")));
});
// 没有来源状态的快照不能被确认为完整可靠数据。
test("missing source status cannot certify complete data", async () => {
  const f = fixture();
  f.snapshots[3].sources = [];
  const { code } = await audit(f);
  assert.equal(code, 1);
});
// 非负约束独立于总量平衡，防止负家数绕过合计检查。
test("negative counts cannot pass a balanced total", async () => {
  const f = fixture();
  f.snapshots[2].breadth.up = -1;
  f.snapshots[2].breadth.down = 10;
  const { code, report } = await audit(f);
  assert.equal(code, 1);
  assert.ok(report.issues.some((s) => s.includes("invalid breadth count")));
});
