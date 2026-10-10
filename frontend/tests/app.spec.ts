import { test, expect } from "@playwright/test";

test("real server mobile shell and empty states", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "市场情绪查看器" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "刷新数据", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "评分明细" }).click();
  await expect(
    page.getByText("昨日十亿首板表现", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "自选", exact: true }).click();
  await expect(page.getByPlaceholder("搜索股票名称 / 代码")).toBeVisible();
  await page.getByRole("button", { name: "复盘", exact: true }).click();
  await expect(page.locator("#day")).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  expect(errors).toEqual([]);
  await page.getByRole("button", { name: "看盘", exact: true }).click();
  await page.screenshot({ path: "test-results/mobile.png", fullPage: true });
});
test("seal orders and turnover have separate labels and missing orders stay unknown", async ({
  page,
  request,
}) => {
  const base = await request.get("/api/dashboard").then((r) => r.json());
  const pool = base.pools.find((p: { id: string }) => p.id === "today-sealing");
  pool.ready = true;
  pool.stocks = [
    {
      code: "600000",
      name: "封单测试",
      sealed: true,
      amount: 1200000000,
      seal_amount: 35000000,
      change: 10,
    },
    {
      code: "600001",
      name: "缺失测试",
      sealed: true,
      amount: 1200000000,
      change: 10,
    },
    {
      code: "600002",
      name: "炸板测试",
      sealed: false,
      amount: 1200000000,
      change: 5,
    },
  ];
  pool.count = 3;
  await page.route("**/api/dashboard", (r) => r.fulfill({ json: base }));
  await page.goto("/");
  const card = page.locator(".pool-card").filter({
    has: page.getByRole("heading", { name: "主板封板与炸板", exact: true }),
  });
  await card.getByRole("button").first().click();
  await card.getByRole("button", { name: "列表视图", exact: true }).click();
  const row = (name: string) =>
    card.locator(".stock-row").filter({ hasText: name });
  await expect(row("封单测试")).toContainText("封单金额 0.35 亿");
  await expect(row("封单测试")).toContainText("成交额 12.00 亿");
  await expect(row("缺失测试")).toContainText("封单金额 暂无");
  await expect(row("炸板测试")).toContainText("炸板");
  await expect(row("炸板测试")).not.toContainText("封单金额");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
});
test("exchange turnover works without stock breadth and refreshes with its own timestamp", async ({
  page,
  request,
}) => {
  const base = await request.get("/api/dashboard").then((r) => r.json());
  let amount = 800000000000;
  await page.clock.install();
  await page.route("**/api/dashboard", (r) =>
    r.fulfill({
      json: {
        ...base,
        breadth: null,
        turnover: {
          amount,
          previous_same_amount: 1000000000000,
          volume_change: (amount / 1000000000000 - 1) * 100,
          forecast: amount * 2,
          source_time: "2026-09-08T10:30:00+08:00",
          previous_day: "2026-09-07",
          provider: "东方财富",
          captured_at: "2026-09-08T10:30:05+08:00",
          stale: false,
          error: null,
        },
      },
    }),
  );
  await page.goto("/");
  const volume = page.locator(".volume-grid").first();
  await expect(volume).toContainText("8000.00 亿");
  await expect(volume).toContainText("-20.00%");
  await expect(page.getByText(/上一交易日 2026-09-07 同刻/)).toContainText(
    "10000.00 亿",
  );
  await expect(page.getByText(/沪深指数成交额合计/)).toContainText("10:30:00");
  amount = 900000000000;
  await page.clock.runFor(10050);
  await expect(volume).toContainText("9000.00 亿");
  await expect(volume).toContainText("-10.00%");
});
test("reconstructed yesterday pools display today's performance without top50 history", async ({
  page,
  request,
}) => {
  const base = await request.get("/api/dashboard").then((r) => r.json());
  const pool = base.pools.find(
    (p: { id: string }) => p.id === "yesterday-cap-first",
  );
  Object.assign(pool, {
    ready: true,
    count: 1,
    valid_count: 1,
    average: 3,
    stocks: [
      {
        code: "600000",
        name: "昨日首板测试",
        change: 3,
        amount: 10000000,
        price: 11.3,
      },
    ],
  });
  base.yesterday_rebuild = {
    day: "2026-09-07",
    complete: true,
    running: false,
    checked: 5000,
    total: 5000,
    failed: 0,
    excluded: 100,
    error: null,
  };
  await page.route("**/api/dashboard", (r) => r.fulfill({ json: base }));
  await page.goto("/");
  const card = page.locator(".pool-card").filter({
    has: page.getByRole("heading", { name: "昨日十亿首板", exact: true }),
  });
  await expect(card.locator(".pool-value")).toContainText("+3.00%");
  await card.getByRole("button").first().click();
  await card.getByRole("button", { name: "列表视图", exact: true }).click();
  await expect(card.locator(".stock-row")).toContainText("昨日首板测试");
  await expect(page.getByText(/昨日封炸板池已按历史日线重建/)).toBeVisible();
});

test("@live real pool search, sorting, detail and watch persistence", async ({
  page,
  request,
}) => {
  await page.goto("/");
  const pool = page.locator("section.pool-card").filter({
    has: page.getByRole("heading", { name: "主板封板与炸板", exact: true }),
  });
  await pool.getByRole("button").first().click();
  await pool.getByRole("button", { name: "列表视图", exact: true }).click();
  await expect(pool.locator(".stock-row").first()).toBeVisible();
  await pool.getByLabel("主板封板与炸板排序").selectOption("name");
  const first = pool.locator(".stock-row").first();
  const code = (
    await first.locator(".stock-main small").first().innerText()
  ).slice(0, 6);
  const existing = await request.get("/api/watchlist").then((r) => r.json());
  const wasWatched = existing.codes.includes(code);
  await pool.getByLabel("搜索主板封板与炸板").fill(code);
  await expect(pool.locator(".stock-row")).toHaveCount(1);
  await first.locator(".stock-main").click();
  await expect(page.getByRole("dialog")).toBeVisible();
  if (!wasWatched)
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "加入自选", exact: true })
      .click();
  await page.getByRole("button", { name: "关闭股票详情" }).click();
  await page.getByRole("button", { name: "自选", exact: true }).click();
  await expect(
    page.locator(".watch-stocks .stock-row").filter({ hasText: code }),
  ).toBeVisible();
  await page.reload();
  await page.getByRole("button", { name: "自选", exact: true }).click();
  await expect(
    page.locator(".watch-stocks .stock-row").filter({ hasText: code }),
  ).toBeVisible();
  if (!wasWatched) await request.delete(`/api/watchlist/${code}`);
});
