// Independent reference oracle for testdata only, never used by the Go service.
// Reference: AKShare akshare/stock/cons.py hk_js_decode; MIT license alongside.
const fs = require("node:fs");
const vm = require("node:vm");
const path = require("node:path");
const root = path.resolve(__dirname, "..");
const body = fs.readFileSync(
  path.join(root, "testdata/sina-sh600000-klc2.js"),
  "utf8",
);
const match = body.match(/^\s*var KLC_K2_sh600000\s*=\s*("[^"\r\n]*")\s*;/);
if (!match) throw new Error("Unexpected fixture envelope");
const context = vm.createContext({ payload: JSON.parse(match[1]) });
vm.runInContext(
  fs.readFileSync(path.join(__dirname, "reference/sina-klc.js"), "utf8"),
  context,
  { timeout: 5000 },
);
const bars = vm.runInContext(
  `d(payload).map(b => ({
  day: b.date.toISOString().slice(0, 10), open: b.open, close: b.close,
  high: b.high, low: b.low, volume: b.volume,
  amount: b.amount ?? null, prevclose: b.prevclose ?? null
}))`,
  context,
  { timeout: 5000 },
);
fs.writeFileSync(
  path.join(root, "testdata/sina-sh600000-golden.json"),
  JSON.stringify(bars),
);
console.log(`${bars.length} reference bars written`);
