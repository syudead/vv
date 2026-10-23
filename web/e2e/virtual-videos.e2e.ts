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
    const response = page.waitForResponse(
      (answer) =>
        answer.url().endsWith("/api/virtual-videos") &&
        answer.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: "Create" }).click();
    const answer = await response;
    expect(answer.status()).toBe(201);
    // Keep the cleanup identity as soon as the video exists, before any later step can fail.
    created = (await answer.json()) as { id: number; contentVersion: string };
    const id = created.id;

    await expect(page).toHaveURL(new RegExp(`/videos/${String(id)}$`));

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
