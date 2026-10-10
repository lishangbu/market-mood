import { test, expect } from "@playwright/test";
test.use({ timezoneId: "America/New_York" });
test("voice is opt-in, deduplicated and cancels in background", async ({
  page,
  request,
}) => {
  const d = await request.get("/api/dashboard").then((r) => r.json());
  d.session = "交易中";
  d.sealing = { sealed: 8, broken: 2, rate: 80, height: 2 };
  d.sources.forEach((s: { stale: boolean; error: null }) => {
    s.stale = false;
    s.error = null;
  });
  await page.clock.install({ time: new Date("2026-09-07T02:00:00Z") });
  await page.addInitScript(() => {
    const calls: string[] = [];
    Object.assign(window, { voiceCalls: calls, voiceCancels: 0 });
    Object.defineProperty(window, "speechSynthesis", {
      value: {
        speak: (u: SpeechSynthesisUtterance) => calls.push(u.text),
        cancel: () => {
          (window as unknown as { voiceCancels: number }).voiceCancels++;
        },
      },
    });
  });
  await page.route("**/api/dashboard", (r) => r.fulfill({ json: d }));
  await page.goto("/");
  await expect(
    page.getByRole("button", { name: "开启语音播报" }),
  ).toBeVisible();
  const spoken = () =>
    page.evaluate(
      () => (window as unknown as { voiceCalls: string[] }).voiceCalls,
    );
  expect(await spoken()).toEqual([]);
  await page.getByRole("button", { name: "开启语音播报" }).click();
  await expect.poll(spoken).toHaveLength(2);
  expect((await spoken())[1]).toContain("百分之80.0");
  await page.getByRole("button", { name: "刷新数据", exact: true }).click();
  await page.clock.runFor(11000);
  expect(await spoken()).toHaveLength(2);
  await page.evaluate(() => {
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  const cancels = await page.evaluate(
    () => (window as unknown as { voiceCancels: number }).voiceCancels,
  );
  expect(cancels).toBeGreaterThan(0);
  await page.clock.fastForward(600000);
  expect(await spoken()).toHaveLength(2);
  await page.evaluate(() => {
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
  });
  await page.getByRole("button", { name: "关闭语音播报" }).click();
  expect(
    await page.evaluate(() => localStorage.getItem("market-mood-voice")),
  ).toBe("false");
  await page.reload();
  await expect(
    page.getByRole("button", { name: "开启语音播报" }),
  ).toBeVisible();
});
