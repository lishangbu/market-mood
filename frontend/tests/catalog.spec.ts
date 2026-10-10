import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

test("directory searches by name, filters Beijing and paginates on mobile", async ({
  page,
}) => {
  await page.route("**/api/stocks?*", async (route) => {
    const params = new URL(route.request().url()).searchParams;
    const q = params.get("q") ?? "";
    const market = params.get("exchange");
    const second = params.get("page") === "2";
    const items =
      q === "不存在"
        ? []
        : [
            {
              code: market === "bj" ? "920000" : second ? "600021" : "600000",
              name: market === "bj" ? "安徽凤凰" : "浦发银行",
              exchange: market ?? "sh",
              board: market === "bj" ? "北交所" : "主板",
              industry: null,
              listing_date: null,
              listing_status: null,
              provider: "新浪",
            },
          ];
    await route.fulfill({
      json: {
        items,
        total: q ? items.length : 21,
        page: second ? 2 : 1,
        page_size: 20,
        status: {
          updated_at: "2026-09-08T09:00:00+08:00",
          error: null,
          exchange_checks: { bj: "官网核对未完成；使用聚合目录" },
        },
      },
    });
  });
  await page.goto("/");
  await page.getByRole("button", { name: "自选", exact: true }).click();
  const catalog = page.getByRole("region", { name: "全市场股票目录" });
  await expect(
    catalog.getByRole("button", { name: /浦发银行.*600000/ }),
  ).toBeVisible();
  await catalog.getByRole("button", { name: "下一页" }).click();
  await expect(catalog.getByText("600021 · 主板")).toBeVisible();
  await page.getByPlaceholder("搜索股票名称 / 代码").fill("银行");
  await expect(catalog.getByText("600000 · 主板")).toBeVisible();
  await page.getByPlaceholder("搜索股票名称 / 代码").fill("");
  await catalog.getByLabel("筛选股票市场").selectOption("bj");
  await expect(
    catalog.getByRole("button", { name: /安徽凤凰.*920000/ }),
  ).toBeVisible();
  await expect(
    catalog.getByRole("button", { name: "加入自选 安徽凤凰" }),
  ).toBeVisible();
  await expect(catalog.getByText("名单核对状态")).toHaveCount(0);
  await expect(catalog.getByText(/官网核对未完成/)).toHaveCount(0);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  const audit = await new AxeBuilder({ page })
    .include(".catalog")
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    .analyze();
  expect(audit.violations).toEqual([]);
  await page.getByPlaceholder("搜索股票名称 / 代码").fill("不存在");
  await expect(catalog.getByText("没有匹配的股票")).toBeVisible();
});

test("directory distinguishes first sync and failure with cached rows", async ({
  page,
}) => {
  await page.route("**/api/stocks?*", (route) =>
    route.fulfill({
      json: {
        items: [],
        total: 0,
        status: { updated_at: null, error: null, exchange_checks: {} },
      },
    }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: "自选", exact: true }).click();
  await expect(page.getByText("目录就绪后可按名称或代码搜索")).toBeVisible();
  await page.route("**/api/stocks?*", (route) =>
    route.fulfill({
      json: {
        items: [{ code: "600000", name: "浦发银行", board: "主板" }],
        total: 1,
        status: {
          updated_at: "2026-09-07T09:00:00+08:00",
          error: "免费来源暂不可用",
          exchange_checks: {},
        },
      },
    }),
  );
  await page.getByPlaceholder("搜索股票名称 / 代码").fill("浦发");
  const catalog = page.getByRole("region", { name: "全市场股票目录" });
  await expect(catalog.getByText(/保留上次结果/)).toBeVisible();
  await expect(
    catalog.getByRole("button", { name: /浦发银行.*600000/ }),
  ).toBeVisible();
});
test("@live real directory supports Chinese search, Beijing details and watch persistence", async ({
  page,
  request,
}) => {
  const data = await request
    .get("/api/stocks?exchange=bj&page_size=1")
    .then((r) => r.json());
  expect(data.total).toBeGreaterThan(100);
  const stock = data.items[0];
  const before = await request.get("/api/watchlist").then((r) => r.json());
  const existing = before.codes.includes(stock.code);
  try {
    await page.goto("/");
    await page.getByRole("button", { name: "自选", exact: true }).click();
    const input = page.getByPlaceholder("搜索股票名称 / 代码");
    await input.fill("银行");
    const catalog = page.getByRole("region", { name: "全市场股票目录" });
    await expect(catalog.locator(".stock-main").first()).toContainText("银行");
    await input.fill(stock.name);
    await catalog.getByLabel("筛选股票市场").selectOption("bj");
    await expect(catalog.locator(".stock-main").first()).toContainText(
      stock.code,
    );
    await catalog.locator(".stock-main").first().click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText(/档案来源：新浪/)).toBeVisible();
    if (!existing)
      await dialog
        .getByRole("button", { name: "加入自选", exact: true })
        .click();
    await page.getByRole("button", { name: "关闭股票详情" }).click();
    await page.reload();
    await page.getByRole("button", { name: "自选", exact: true }).click();
    await expect(
      page.locator(".watch-stocks .stock-row").filter({ hasText: stock.code }),
    ).toBeVisible();
    await input.fill(stock.name);
    await page.screenshot({
      path: "test-results/catalog-mobile.png",
      fullPage: true,
    });
  } finally {
    if (!existing) await request.delete(`/api/watchlist/${stock.code}`);
  }
});
test("background directory refresh preserves rows during loading and failure, but not for a new query", async ({
  page,
}) => {
  let fail = false;
  let release: (() => void) | undefined;
  let started = false;
  await page.route("**/api/stocks?*", async (r) => {
    if (fail) {
      started = true;
      await new Promise<void>((resolve) => {
        release = resolve;
      });
      return r.fulfill({ status: 503, body: "目录暂不可用" });
    }
    return r.fulfill({
      json: {
        items: [{ code: "600000", name: "浦发银行", board: "主板" }],
        total: 1,
        status: { updated_at: "2026-09-21T09:00:00+08:00", error: null },
      },
    });
  });
  await page.goto("/");
  await page.getByRole("button", { name: "自选", exact: true }).click();
  const catalog = page.getByRole("region", { name: "全市场股票目录" });
  const row = catalog.getByRole("button", { name: /浦发银行.*600000/ });
  await expect(row).toBeVisible();
  fail = true;
  await page.evaluate(() =>
    document.dispatchEvent(new Event("visibilitychange")),
  );
  await expect.poll(() => started).toBe(true);
  await expect(row).toBeVisible();
  await expect(catalog.getByRole("status")).toContainText("正在更新");
  release!();
  await expect(catalog.getByRole("alert")).toContainText("保留上次结果");
  await expect(row).toBeVisible();
  await page.getByPlaceholder("搜索股票名称 / 代码").fill("不存在");
  await expect(row).toHaveCount(0);
  fail = false;
  release?.();
});
