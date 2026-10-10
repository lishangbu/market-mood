// Offline browser-test server: fixed synthetic API responses and built UI only.
// Never connects to a market source or opens an application database.
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { resolve, extname, sep } from "node:path";
const root = resolve("frontend/dist");
const dashboard = JSON.parse(
  await readFile("frontend/tests/fixtures/dashboard.json", "utf8"),
);
const json = (res, body) => {
  res.setHeader("Content-Type", "application/json");
  res.end(JSON.stringify(body));
};
const server = createServer(async (req, res) => {
  const path = new URL(req.url, "http://localhost").pathname;
  if (path.startsWith("/api/")) {
    if (req.method !== "GET") {
      res.writeHead(405);
      res.end();
      return;
    }
    if (path === "/api/tasks") return json(res, { status: "ok", tasks: [] });
    if (path === "/api/dashboard") return json(res, dashboard);
    if (path === "/api/watchlist")
      return json(res, { stocks: [], codes: [], error: null });
    if (path === "/api/history") return json(res, [dashboard.day]);
    if (path === `/api/history/${dashboard.day}`)
      return json(res, { snapshots: [], close: dashboard, historical: null });
    if (path === "/api/backfill")
      return json(res, { completed: 0, running: false, error: null });
    if (path === "/api/sources")
      return json(res, {
        quotes: ["tencent", "eastmoney"],
        stock_quotes: ["tdx", "tencent", "eastmoney"],
      });
    if (path === "/api/stocks")
      return json(res, {
        items: [],
        total: 0,
        page: 1,
        page_size: 20,
        status: { updated_at: null, error: null },
      });
    res.writeHead(404);
    res.end();
    return;
  }
  const file = resolve(
    root,
    "." + decodeURIComponent(path === "/" ? "/index.html" : path),
  );
  if (!file.startsWith(root + sep)) {
    res.writeHead(403);
    res.end();
    return;
  }
  try {
    const body = await readFile(file);
    res.setHeader(
      "Content-Type",
      { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" }[
        extname(file)
      ] ?? "application/octet-stream",
    );
    res.end(body);
  } catch {
    res.writeHead(404);
    res.end();
  }
});
server.listen(4173, "127.0.0.1");
for (const signal of ["SIGINT", "SIGTERM"])
  process.on(signal, () => server.close());
