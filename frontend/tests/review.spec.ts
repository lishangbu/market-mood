import { test, expect } from "@playwright/test";

test("backfilled review uses dashboard pools and historical quotes only", async ({
  page,
}) => {
  const stock = {
    bids: [],
    asks: [],
    fallback_reasons: [],
    code: "600001",
    name: "历史样本",
    industry: "",
    price: 12.34,
    change: 10,
    amount: 1200000000,
    boards: 1,
    sealed: true,
    eligible: true,
  };
  const day = "2026-09-17";
  const index = { open: 3000, close: 3010, high: 3020, low: 2990, volume: 100 };
  await page.route("**/api/history", (r) =>
    r.fulfill({ json: [day, "2026-09-16"] }),
  );
  await page.route("**/api/history/*", (r) =>
    r.fulfill({
      json: {
        snapshots: [],
        close: null,
        historical: {
          day: r.request().url().endsWith(day) ? day : "2026-09-16",
          imported_at: "2026-09-20T00:00:00+08:00",
          source: "同花顺历史封炸池",
          limitation: "未保存历史评分",
          shanghai: index,
          shenzhen: index,
          limits: r.request().url().endsWith(day)
            ? {
                day,
                sealed: [
                  stock,
                  { ...stock, code: "300001", name: "创业板样本" },
                ],
                broken: [],
              }
            : null,
        },
      },
    }),
  );
  let live = 0;
  await page.route("**/api/stocks/**", (r) => {
    live++;
    return r.abort();
  });
  await page.goto("/");
  await page.getByRole("button", { name: "复盘", exact: true }).click();
  await expect(page.locator(".session-row")).toContainText(day);
  await expect(page.getByText("当日评分未记录")).toBeVisible();
  const metric = page
    .locator("section.metric")
    .filter({ hasText: "主板封板率" });
  await expect(metric).toContainText("100.0%");
  await expect(metric).toContainText("封 1 / 炸 0");
  const pool = page.locator(".pool-card").filter({
    has: page.getByRole("heading", { name: "历史涨停全名单", exact: true }),
  });
  await pool.locator(".pool-header").click();
  await expect(pool.locator("tbody tr")).toHaveCount(2);
  await pool.getByLabel("搜索历史涨停全名单").fill("600001");
  await pool.getByRole("button", { name: "列表视图", exact: true }).click();
  await pool.locator(".stock-main").click();
  await expect(
    page.getByRole("dialog").locator(".detail-price > strong"),
  ).toHaveText("12.34");
  await page.keyboard.press("Escape");
  expect(live).toBe(0);
  await page.locator("#day").selectOption("2026-09-16");
  await expect(page.locator(".session-row")).toContainText("2026-09-16");
  await expect(metric).toContainText("当日封炸板数据未记录");
  await expect(page.getByRole("img", { name: /当日.*曲线/ })).toHaveCount(0);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
test("review updates after backfill without changing dates and retains data on failure", async ({
  page,
}) => {
  let complete = false;
  let fail = false;
  let requests = 0;
  await page.route("**/api/history", (r) =>
    r.fulfill({ json: ["2026-09-17"] }),
  );
  await page.route("**/api/backfill", (r) =>
    r.fulfill({
      json: {
        completed: complete ? 1 : 0,
        running: !complete,
        error: fail ? "重试" : null,
        updated_at: fail ? "3" : complete ? "2" : "1",
      },
    }),
  );
  await page.route("**/api/history/2026-09-17", (r) => {
    requests++;
    if (fail) return r.fulfill({ status: 503, body: "暂不可用" });
    return r.fulfill({
      json: {
        snapshots: [],
        close: null,
        historical: complete
          ? {
              day: "2026-09-17",
              imported_at: "2026-09-20T00:00:00+08:00",
              source: "测试历史源",
              limitation: "未记录评分",
              shanghai: {
                open: 3000,
                close: 3010,
                high: 3020,
                low: 2990,
                volume: 100,
              },
              shenzhen: {
                open: 3000,
                close: 3010,
                high: 3020,
                low: 2990,
                volume: 100,
              },
              limits: {
                day: "2026-09-17",
                sealed: [
                  {
                    code: "600001",
                    name: "补齐样本",
                    eligible: true,
                    sealed: true,
                  },
                ],
                broken: [],
              },
            }
          : null,
      },
    });
  });
  await page.goto("/");
  await page.getByRole("button", { name: "复盘", exact: true }).click();
  await expect.poll(() => requests).toBeGreaterThan(0);
  complete = true;
  await page.evaluate(() =>
    document.dispatchEvent(new Event("visibilitychange")),
  );
  const pool = page.locator(".pool-card").filter({
    has: page.getByRole("heading", { name: "历史涨停全名单", exact: true }),
  });
  await expect(pool).toBeVisible();
  await pool.locator(".pool-header").click();
  await expect(pool.getByText("补齐样本", { exact: true })).toBeVisible();
  fail = true;
  const before = requests;
  await page.evaluate(() =>
    document.dispatchEvent(new Event("visibilitychange")),
  );
  await expect.poll(() => requests).toBeGreaterThan(before);
  await expect(pool.getByText("补齐样本", { exact: true })).toBeVisible();
});
