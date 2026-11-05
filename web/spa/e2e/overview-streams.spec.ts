import { expect, test } from "./support";

// A stream whose measured health is failing is something the reader must act on, so the Overview's
// "Needs your attention" lists it beside the failing workflows. Mocked: the demo stack has no failing stream.
const attention = (streams: unknown[], streamsMore = 0) => ({
  total: streams.length + streamsMore, items: [], more: 0, targets: [], targetsMore: 0, streams, streamsMore,
});
const stream = (i: number) => ({ gateway: "events_gateway", path: `/v1/stream_${i}`, href: `/ingest/streams/events_gateway/v1/stream_${i}`, since: new Date(Date.now() - 600_000).toISOString() });

test("a failing stream is listed in the attention section and opens its page", async ({ page }) => {
  await page.route("**/api/overview/attention", (r) => r.fulfill({ json: attention([stream(1), stream(2)], 3) }));
  await page.goto("/");
  const section = page.locator("[data-attention]");
  await expect(section.locator("[data-attention-stream]")).toHaveCount(2);
  await expect(section.locator("[data-attention-stream]").first()).toContainText("/v1/stream_1 is failing");
  await expect(section).toContainText("and 3 more failing streams");
  await expect(section.locator("span.tabular-nums")).toHaveText("5");
  await section.getByRole("link", { name: "Open stream" }).first().click();
  await expect(page).toHaveURL(/\/ingest\/streams\/events_gateway\/v1\/stream_1$/);
});

test("with no failing stream the section says nothing about streams", async ({ page }) => {
  await page.route("**/api/overview/attention", (r) => r.fulfill({ json: attention([]) }));
  await page.goto("/");
  await expect(page.locator("[data-attention-empty]")).toBeVisible();
  await expect(page.locator("[data-attention-stream]")).toHaveCount(0);
});
