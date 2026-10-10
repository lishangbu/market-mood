import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";
const fixture = JSON.parse(
  readFileSync(new URL("./fixtures/dashboard.json", import.meta.url), "utf8"),
);

test("dashboard updates despite failed watchlist and preserves its own last success", async ({
  page,
}) => {
  const data = structuredClone(fixture);
  data.score.total = 42;
  await page.route("**/api/dashboard", (r) => r.fulfill({ json: data }));
  await page.route("**/api/watchlist", (r) =>
    r.fulfill({ status: 503, json: { error: "自选暂不可用" } }),
  );
  await page.goto("/");
  await expect(page.locator(".score > strong")).toHaveText("42");
  data.score.total = 65;
  await page.getByRole("button", { name: "刷新数据", exact: true }).click();
  await expect(page.locator(".score > strong")).toHaveText("65");
  await page.getByRole("button", { name: "自选", exact: true }).click();
  await expect(
    page.getByText("自选行情更新失败，保留最后数据：自选暂不可用", {
      exact: true,
    }),
  ).toBeVisible();
});

test("manual refresh supersedes old response while automatic polling does not overlap", async ({
  page,
}) => {
  await page.clock.install();
  let count = 0;
  let release!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/dashboard", async (route) => {
    const n = ++count;
    if (n === 2) await held;
    const data = structuredClone(fixture);
    data.score.total = n === 2 ? 12 : 80;
    await route.fulfill({ json: data }).catch(() => {});
  });
  await page.goto("/");
  await expect(page.locator(".score > strong")).toHaveText("80");
  // First automatic poll is held; another interval must not start another request.
  await page.clock.runFor(10000);
  await expect.poll(() => count).toBe(2);
  await page.clock.runFor(10000);
  expect(count).toBe(2);
  await page.getByRole("button", { name: "刷新数据", exact: true }).click();
  await expect.poll(() => count).toBe(3);
  release();
  await expect(page.locator(".score > strong")).toHaveText("80");
});

test("hidden page pauses requests and becoming visible refreshes immediately", async ({
  page,
}) => {
  await page.clock.install();
  let count = 0;
  await page.route("**/api/dashboard", (r) => {
    count++;
    return r.fulfill({ json: fixture });
  });
  await page.goto("/");
  await expect(page.locator(".score > strong")).toBeVisible();
  await page.evaluate(() => {
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  const before = count;
  await page.clock.runFor(60000);
  expect(count).toBe(before);
  await page.evaluate(() => {
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await expect.poll(() => count).toBe(before + 1);
});

test("request timeout preserves data and allows a later retry", async ({
  page,
}) => {
  await page.clock.install();
  await page.goto("/");
  await expect(page.locator(".score > strong")).toBeVisible();
  const before = await page.locator(".score > strong").textContent();
  await page.route("**/api/dashboard", () => {});
  await page.getByRole("button", { name: "刷新数据", exact: true }).click();
  await page.clock.runFor(16000);
  await expect(page.getByRole("alert")).toContainText("超时");
  await expect(page.locator(".score > strong")).toHaveText(before!);
  await page.unroute("**/api/dashboard");
  await page.getByRole("button", { name: "重试", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveCount(0);
});
