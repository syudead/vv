import { type APIRequestContext, expect, test } from "@playwright/test";

/** generateScanIssueFixtures（media-fixtures.mjs）が作るファイルの名前。 */
const UNREADABLE = "scan-issue-unreadable.mp4";
const BROKEN = "scan-issue-broken.mp4";

interface MediaFolder {
  id: number;
  version: number;
  path: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

async function currentStatus(request: APIRequestContext): Promise<string> {
  const response = await request.get("/api/scans/current");
  if (response.status() === 404) return "none";
  return ((await response.json()) as { status: string }).status;
}

test.describe.serial("scan issues with the real server", () => {
  let folder: MediaFolder | null = null;

  test.afterAll(async ({ request }) => {
    if (folder === null) return;
    const response = await request.get("/api/media-folders");
    const current = ((await response.json()) as MediaFolder[]).find(
      (item) => item.id === folder?.id,
    );
    if (current === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(current.id)}?version=${String(current.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("読めないファイルと解析できない動画は一部失敗になり、一覧から影響と理由を読み、再生画面へ移れる", async ({
    page,
    request,
  }) => {
    test.setTimeout(360_000);
    const root = process.env.MDM_E2E_SCAN_ISSUES_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_SCAN_ISSUES_MEDIA_DIR is not configured");

    // 先に走った検査の取り込みが終わるのを待ってから始める。
    await expect
      .poll(() => currentStatus(request), { timeout: 300_000 })
      .not.toMatch(/^(finding|running)$/);

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;

    await page.goto("/settings#scan-status");
    await expect(page.getByRole("button", { name: "Scan library" })).toBeEnabled();
    await page.getByRole("button", { name: "Scan library" }).click();

    // 解析はやり直しの上限まで失敗してから終わる。
    const section = page.locator("#scan-status");
    await expect(section.getByText("Some failed", { exact: true })).toBeVisible({
      timeout: 300_000,
    });
    expect(await currentStatus(request)).toBe("partial");

    const expectRows = async () => {
      const list = section.getByRole("region", { name: "Videos with problems" });
      await expect(list).toBeVisible();
      const unreadable = list.getByTestId("scan-issue").filter({ hasText: UNREADABLE });
      await expect(unreadable).toHaveCount(1);
      await expect(unreadable).toContainText("Failed");
      await expect(unreadable).toContainText(
        "Not added to the library. The file couldn't be read.",
      );
      await expect(unreadable.getByRole("link")).toHaveCount(0);
      const broken = list.getByTestId("scan-issue").filter({ hasText: BROKEN });
      await expect(broken).toHaveCount(1);
      await expect(broken).toContainText("Failed");
      await expect(broken).toContainText(
        "It may not play. It couldn't be analyzed as a video.",
      );
      return broken;
    };

    await expectRows();
    // 再読み込みのあとも、同じ一覧が出る。
    await page.reload();
    const broken = await expectRows();

    await broken.getByRole("link").click();
    await expect(page).toHaveURL(/\/videos\/\d+$/);
  });
});
