import { test, expect } from "@playwright/test";

test("top50 progress, failure and completed pool remain usable on narrow screens", async ({
  page,
  request,
}) => {
  const dashboard = await request.get("/api/dashboard").then((r) => r.json());
  const pool = dashboard.pools.find(
    (p: { id: string }) => p.id === "yesterday-top50",
  );
  pool.ready = false;
  pool.stocks = [];
  pool.count = 0;
  pool.valid_count = 0;
  pool.average = null;
  pool.yang_rate = null;
  dashboard.top50_rebuild = {
    day: "2026-09-10",
    checked: 1200,
    total: 5013,
    complete: false,
    running: true,
    failed: 0,
    error: null,
  };
  await page.route("**/api/dashboard", (route) =>
    route.fulfill({ json: dashboard }),
  );
  await page.setViewportSize({ width: 320, height: 780 });
  await page.goto("/");
  await expect(page.getByRole("status")).toContainText(
    "正在补齐昨日成交前50：1200 / 5013",
  );
  dashboard.top50_rebuild.running = false;
  dashboard.top50_rebuild.error =
    "昨日成交额待核验：1只失败，5012只有效样本；600000：昨日成交额缺失";
  await page.getByRole("button", { name: "刷新数据", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("昨日成交额待核验");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  dashboard.top50_rebuild.complete = true;
  dashboard.top50_rebuild.error = null;
  pool.ready = true;
  pool.count = 50;
  pool.valid_count = 50;
  pool.average = 2;
  pool.yang_rate = 100;
  pool.stocks = Array.from({ length: 50 }, (_, i) => ({
    code: `600${String(i).padStart(3, "0")}`,
    name: `样本${i}`,
    price: 10,
    change: 2,
    open: 9,
    amount: 1000000,
    eligible: true,
  }));
  await page.getByRole("button", { name: "刷新数据", exact: true }).click();
  await expect(
    page.getByText("昨日成交额待核验", { exact: false }),
  ).toHaveCount(0);
  const card = page.locator("section.pool-card").filter({
    has: page.getByRole("heading", { name: "昨日成交前50", exact: true }),
  });
  await card.getByRole("button").first().click();
  await card.getByRole("button", { name: "列表视图", exact: true }).click();
  await card.getByLabel("搜索昨日成交前50").fill("600049");
  await expect(card.locator(".stock-row")).toHaveCount(1);
  await expect(card).toContainText("样本49");
  await page.screenshot({
    path: "test-results/top50-mobile.png",
    fullPage: true,
  });
});
