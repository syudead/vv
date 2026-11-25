import { expect, test, type APIRequestContext } from "@playwright/test";

// The paused state reaches the scan, the counts and the library's indicator on a real server;
// component tests cover each part's texts. Fixtures come from generateBackgroundPauseFixtures.

interface MediaFolder {
  id: number;
  version: number;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
let folder: MediaFolder | undefined;

async function setPaused(request: APIRequestContext, paused: boolean) {
  const response = await request.put("/api/background/pause", {
    headers: mutationHeaders,
    data: { paused },
  });
  expect(response.status()).toBe(200);
}

test.describe.serial("background pause", () => {
  test.beforeAll(async ({ request }) => {
    const root = process.env.MDM_E2E_BACKGROUND_PAUSE_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_BACKGROUND_PAUSE_MEDIA_DIR is not configured");
    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;
  });

  // Later specs on this server need background work running and none of these videos.
  test.afterAll(async ({ request }) => {
    await setPaused(request, false);
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("a scan asked for while paused waits, the library shows the pause, and a resume finishes it", async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/settings/scan");
    const section = page.getByRole("region", { name: "Background work" });
    await section.getByRole("switch", { name: "Pause background work" }).click();
    await expect(section.getByText("Paused", { exact: true })).toBeVisible();

    const scanStatus = page.getByRole("region", { name: "Scan status" });
    await scanStatus.getByRole("button", { name: "Scan library", exact: true }).click();
    await expect(
      scanStatus.getByText("Waiting for resume", { exact: true }),
    ).toBeVisible();
    await expect(
      section
        .getByRole("term")
        .filter({ hasText: /^Scan/ })
        .getByText("Waiting for resume", { exact: true }),
    ).toBeVisible();
    const scanId = (
      (await (await request.get("/api/scans/current")).json()) as {
        id: number;
      }
    ).id;

    await page.goto("/");
    const indicator = page.getByRole("button", { name: /^Background work is paused/ });
    await expect(indicator).toBeVisible();
    await indicator.hover();
    const summary = page.getByRole("dialog");
    await summary.getByRole("switch", { name: "Pause background work" }).click();
    await expect(indicator).toHaveCount(0);

    await expect
      .poll(
        async () => {
          const scan = (await (await request.get("/api/scans/current")).json()) as {
            id: number;
            state: string;
          };
          return scan.id === scanId ? scan.state : `other scan ${String(scan.id)}`;
        },
        { timeout: 60_000 },
      )
      .toBe("done");
    await page.goto("/settings/scan");
    await expect(
      page
        .getByRole("region", { name: "Scan status" })
        .getByText("Done", { exact: true }),
    ).toBeVisible();
  });
});
