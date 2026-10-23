import { expect, test, type Locator, type Page } from "@playwright/test";

// Acceptance criterion 1 of virtual videos: one made from a title alone is found, marked, in the library and in search results.

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

function virtualCard(scope: Locator | Page, id: number, title: string) {
  const card = scope.locator(`article[data-video-id="${String(id)}"]`);
  return { card, link: card.getByRole("link", { name: title, exact: true }) };
}

test("タイトルだけでバーチャルな動画を作り、ライブラリと検索結果に印付きで見つかる", async ({
  page,
  request,
}) => {
  const title = `e2e virtual ${String(Date.now())}`;
  let created: { id: number; contentVersion: string } | undefined;
  try {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/");
    await page.getByRole("button", { name: "New virtual video" }).click();
    const dialog = page.getByRole("dialog", { name: "New virtual video" });
    await dialog.getByRole("textbox", { name: "Title" }).fill(title);
    await dialog.getByRole("button", { name: "Create" }).click();

    await expect(page).toHaveURL(/\/videos\/\d+$/);
    const id = Number(new URL(page.url()).pathname.split("/").at(-1));
    const read = await request.get(`/api/videos/${String(id)}`);
    expect(read.ok()).toBe(true);
    created = (await read.json()) as { id: number; contentVersion: string };

    await page.goto("/");
    const home = virtualCard(page, id, title);
    await expect(home.link).toHaveAccessibleDescription("Virtual video, no file");
    await expect(home.card.getByText("Virtual", { exact: true })).toBeVisible();

    const field = page.getByRole("searchbox", { name: "Search videos" });
    await field.fill(title);
    await field.press("Enter");
    const results = page.getByRole("region", { name: "Search results", exact: true });
    const found = virtualCard(results, id, title);
    await expect(found.link).toHaveAccessibleDescription("Virtual video, no file");
    await expect(results.locator("article[data-video-id]")).toHaveCount(1);
  } finally {
    // Other suites count the library's videos, so the virtual video does not outlive this test.
    if (created !== undefined) {
      const removed = await request.delete(
        `/api/videos/${String(created.id)}?contentVersion=${encodeURIComponent(created.contentVersion)}`,
        { headers: mutationHeaders },
      );
      expect(removed.status()).toBe(204);
    }
  }
});
