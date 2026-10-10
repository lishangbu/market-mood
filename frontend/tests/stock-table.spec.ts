import { test, expect } from "@playwright/test";

test("headers sort raw values, preserve missing values last and keep sorting after refresh", async ({
  page,
  request,
}) => {
  const d = await request.get("/api/dashboard").then((r) => r.json());
  const pool = d.pools.find((p: { id: string }) => p.id === "today-sealing");
  pool.ready = true;
  pool.stocks = [
    {
      code: "600001",
      name: "阿股",
      change: 1,
      amount: 2e8,
      break_count: 0,
      first_seal_time: "09:30:00",
      limit_stats: { days: 10, count: 2 },
    },
    {
      code: "600002",
      name: "中股",
      change: 2,
      amount: 10e8,
      break_count: 2,
      first_seal_time: "10:00:00",
      limit_stats: { days: 3, count: 3 },
    },
    {
      code: "600003",
      name: "波股",
      change: 3,
      amount: null,
      break_count: null,
    },
  ];
  await page.route("**/api/dashboard", (r) => r.fulfill({ json: d }));
  await page.goto("/");
  const card = page.locator(".pool-card").filter({
    has: page.getByRole("heading", { name: "主板封板与炸板", exact: true }),
  });
  await card.getByRole("button").first().click();
  const names = () => card.locator(".table-stock-name").allTextContents();
  const header = (name: string) =>
    card
      .getByRole("columnheader")
      .filter({ has: page.getByRole("button", { name, exact: true }) });
  await header("成交额").getByRole("button").click();
  await expect.poll(names).toEqual(["中股", "阿股", "波股"]);
  await expect(header("成交额")).toHaveAttribute("aria-sort", "descending");
  await header("成交额").getByRole("button").click();
  await expect.poll(names).toEqual(["阿股", "中股", "波股"]);
  pool.stocks[0].amount = 20e8;
  await page.getByRole("button", { name: "刷新数据", exact: true }).click();
  await expect.poll(names).toEqual(["中股", "阿股", "波股"]);
  await header("炸板次数").getByRole("button").click();
  await header("炸板次数").getByRole("button").click();
  await expect.poll(names).toEqual(["阿股", "中股", "波股"]);
  await header("首次封板时间").getByRole("button").click();
  await expect.poll(names).toEqual(["阿股", "中股", "波股"]);
  await header("涨停统计").getByRole("button").click();
  await expect.poll(names).toEqual(["中股", "阿股", "波股"]);
  await header("名称").getByRole("button").focus();
  await page.keyboard.press("Enter");
  await expect.poll(names).toEqual(["阿股", "波股", "中股"]);
  await card.getByLabel("搜索主板封板与炸板").fill("600002");
  await expect.poll(names).toEqual(["中股"]);
});

test("complete stock table shows supplied fields and scrolls within mobile viewport", async ({
  page,
  request,
}) => {
  const d = await request.get("/api/dashboard").then((r) => r.json());
  const pool = d.pools.find((p: { id: string }) => p.id === "today-sealing");
  pool.ready = true;
  pool.stocks = [
    {
      code: "000993",
      name: "字段测试",
      change: 10,
      price: 13.88,
      amount: 664912544,
      float_cap: 6356366195,
      market_cap: 6356366209,
      turnover_rate: 10.47,
      sealed: true,
      seal_amount: 135527678,
      first_seal_time: "09:25:00",
      last_seal_time: "09:32:36",
      break_count: 1,
      boards: 3,
      limit_stats: { days: 3, count: 3 },
      industry: "电力",
    },
  ];
  pool.count = 1;
  await page.route("**/api/dashboard", (r) => r.fulfill({ json: d }));
  for (const width of [320, 390, 1280]) {
    await page.setViewportSize({ width, height: 844 });
    await page.goto("/");
    const card = page.locator(".pool-card").filter({
      has: page.getByRole("heading", { name: "主板封板与炸板", exact: true }),
    });
    await card.getByRole("button").first().click();
    const table = card.getByRole("table");
    await expect(table.getByRole("columnheader")).toHaveCount(17);
    await expect(table).toContainText("09:25:00");
    await expect(table).toContainText("09:32:36");
    await expect(table).toContainText("3天3板");
    await expect(table).toContainText("1.36 亿");
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBeTruthy();
    await card
      .getByRole("region", { name: "股票完整字段" })
      .evaluate((el) => (el.scrollLeft = 900));
    await expect(table.getByText("09:32:36", { exact: true })).toBeVisible();
    if (width === 390)
      await page.screenshot({
        path: "test-results/stock-table-mobile.png",
        fullPage: true,
      });
    await card.getByRole("button", { name: "列表视图" }).click();
    await expect(card.locator(".stock-row")).toHaveCount(1);
  }
});
