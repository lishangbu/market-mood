import { test, expect } from "@playwright/test";

const market = {
  id: "market",
  name: "全市场行情",
  state: "running",
  started_at: "2026-09-07T09:00:00+08:00",
  last_checked_at: "2026-09-07T10:02:00+08:00",
  last_success_at: "2026-09-07T10:01:00+08:00",
  error: null as string | null,
};

test("stopped collection is visible even when dashboard requests fail", async ({
  page,
}) => {
  await page.route("**/api/tasks", (r) =>
    r.fulfill({
      json: {
        status: "degraded",
        tasks: [
          { ...market, state: "failed", error: "任务异常退出：测试错误" },
        ],
      },
    }),
  );
  await page.route("**/api/dashboard", (r) =>
    r.fulfill({ status: 500, json: { error: "读取失败" } }),
  );
  await page.goto("/");
  const status = page.getByRole("region", { name: "后台采集状态" });
  await expect(status.getByRole("alert")).toContainText(
    "后台采集已停止：全市场行情",
  );
  await status.locator("summary").click();
  await expect(status).toContainText("任务异常退出：测试错误");
  await expect(status).toContainText("10:01:00");
});

test("task polling failure cannot leave a healthy label and recovers on next poll", async ({
  page,
}) => {
  await page.clock.install();
  let failing = false;
  await page.route("**/api/tasks", (r) =>
    failing
      ? r.fulfill({ status: 503, json: { error: "状态暂不可用" } })
      : r.fulfill({ json: { status: "ok", tasks: [market] } }),
  );
  await page.goto("/");
  const status = page.getByRole("region", { name: "后台采集状态" });
  await expect(status.locator("summary")).toHaveText("后台采集运行中");
  failing = true;
  await page.clock.runFor(10000);
  await expect(status.getByRole("alert")).toContainText("无法确认后台采集状态");
  await expect(status.locator("summary")).toHaveText("后台状态待确认");
  failing = false;
  await page.clock.runFor(10000);
  await expect(status.getByRole("alert")).toHaveCount(0);
  await expect(status.locator("summary")).toHaveText("后台采集运行中");
});
