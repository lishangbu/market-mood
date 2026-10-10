import { test, expect } from "@playwright/test";
import type { Dashboard } from "../src/types";

// Read actual persisted history; do not intercept or create market responses.
test("@live persisted intraday history renders actual scores and gaps", async ({
  page,
  request,
}) => {
  const current: Dashboard = await request
    .get("/api/dashboard")
    .then((r) => r.json());
  expect(current.day).not.toBeNull();
  const responsePromise = page.waitForResponse(
    (r) => r.url().endsWith(`/api/history/${current.day}`) && r.ok(),
  );
  await page.goto("/");
  await page.getByRole("button", { name: "复盘", exact: true }).click();
  await page.locator("#day").selectOption(current.day!);
  const review: { snapshots: Dashboard[] } = await (
    await responsePromise
  ).json();
  expect(review.snapshots.length).toBeGreaterThan(10);
  const times = review.snapshots.map((s) => s.observed_at);
  expect(new Set(times).size).toBe(times.length);
  for (const snapshot of review.snapshots) {
    expect(snapshot.day).toBe(current.day);
    expect(snapshot.sources.every((s) => !s.stale && !s.error)).toBe(true);
    if (snapshot.score.total !== null) {
      expect(snapshot.score.factors.every((f) => f.score !== null)).toBe(true);
      expect(snapshot.score.total).toBeCloseTo(
        snapshot.score.factors.reduce((sum, f) => sum + (f.score ?? 0), 0),
        6,
      );
    }
  }
  const scored = review.snapshots.filter(
    (s) => s.observed_at !== null && s.score.total !== null,
  );
  expect(scored.length).toBeGreaterThan(10);
  await expect(page.locator(".trend svg circle")).toHaveCount(scored.length);
  // Each rendered segment must stop across absent observations (>90 seconds).
  let segments = 0;
  let previous: Dashboard | undefined;
  for (const snapshot of review.snapshots) {
    if (snapshot.observed_at && snapshot.score.total !== null) {
      if (
        !previous?.observed_at ||
        previous.score.total === null ||
        Date.parse(snapshot.observed_at) - Date.parse(previous.observed_at) >
          90000
      )
        segments++;
    }
    previous = snapshot;
  }
  await expect(page.locator(".trend svg path")).toHaveCount(segments);
  await expect(
    page.getByRole("img", { name: "当日情绪评分曲线，断开处为缺失" }),
  ).toBeVisible();
});
