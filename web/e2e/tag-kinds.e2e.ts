import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// タグ管理画面のタグの種類（親 Issue #1136 の受け入れ条件 1、子 Issue #1154）を、画面で
// 種類を付ける・サーバーが残す・その種類の絞り込みで出るという 1 本の流れだけ実ブラウザに
// 通す（docs/design-docs/web-testing.md）。まとめて付ける・名札・絞り込みの細部は
// src/tags/TagsPage.kindFilter.test.tsx・TagsPage.bulk.test.tsx と Go のストアのテスト
// （internal/store/tag_kind_batch_test.go）が確かめる。
// サーバーとデータベースはほかの e2e と共有するので、作ったタグは終わりに消す。

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

interface TagKind {
  id: number;
  name: string;
}

interface Tag {
  id: number;
  name: string;
}

const created: number[] = [];

/** ensureKind は名前 `name` の種類を返す。新しいデータベースの既定の種類が無ければ作る。 */
async function ensureKind(request: APIRequestContext, name: string): Promise<TagKind> {
  const list = await request.get("/api/tag-kinds");
  expect(list.ok()).toBe(true);
  const { items } = (await list.json()) as { items: TagKind[] };
  const found = items.find((item) => item.name === name);
  if (found !== undefined) return found;
  const response = await request.post("/api/tag-kinds", {
    headers: mutationHeaders,
    data: { name },
  });
  expect(response.status()).toBe(201);
  return (await response.json()) as TagKind;
}

async function createTag(request: APIRequestContext, name: string): Promise<Tag> {
  const response = await request.post("/api/tags", {
    headers: mutationHeaders,
    data: { name },
  });
  expect(response.status()).toBe(201);
  const tag = (await response.json()) as Tag;
  created.push(tag.id);
  return tag;
}

function tagRow(page: Page, name: string) {
  return page
    .getByRole("link", { name: `Open the library filtered by ${name}`, exact: true })
    .locator("xpath=ancestor::*[@data-tag-id][1]");
}

/** chooseKind は「Filter」の吹き出しの「Kind」で `option` を選び、吹き出しを閉じる。 */
async function chooseKind(page: Page, option: string) {
  await page.getByRole("button", { name: /^Filter/ }).click();
  await page.getByRole("combobox", { name: "Kind" }).click();
  await page.getByRole("option", { name: option, exact: true }).click();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
}

/** setKind は選択バーの「Set kind…」のメニューで `name` を選ぶ。 */
async function setKind(page: Page, name: string) {
  const bar = page.getByRole("region", { name: "Selected tags" });
  await bar.getByRole("button", { name: "Set kind…" }).click();
  await page.getByRole("menuitemradio", { name, exact: true }).click();
}

test.describe.serial("tag kinds on the tag management screen", () => {
  test.beforeAll(async ({ request }) => {
    await ensureKind(request, "Genre");
    await createTag(request, "Robot");
  });

  test.afterAll(async ({ request }) => {
    for (const id of created) {
      await request.delete(`/api/tags/${String(id)}`, { headers: mutationHeaders });
    }
  });

  test("1: Robot に Genre を付けると Genre の絞り込みで出て、No kind では出ない", async ({
    page,
  }) => {
    await page.goto("/tags");
    const search = page.getByRole("searchbox", { name: "Search tags" });
    await search.fill("Robot");
    const robot = tagRow(page, "Robot");
    await robot.getByRole("checkbox", { name: 'Select "Robot"' }).click();
    await setKind(page, "Genre");
    await expect(page.getByText('Set the kind "Genre" on 1 tag')).toBeVisible();
    await expect(robot.getByTitle("Kind: Genre")).toBeVisible();

    await search.fill("");
    await chooseKind(page, "Genre");
    await expect(
      page.getByRole("button", { name: 'Remove the filter "Kind: Genre"' }),
    ).toBeVisible();
    await expect(page).toHaveURL(/[?&]kind=\d+/);
    await expect(tagRow(page, "Robot")).toBeVisible();
    await expect(page.getByRole("button", { name: "Filter (1 applied)" })).toBeVisible();

    // 再読み込みしても URL から絞り込みが戻る。
    await page.reload();
    await expect(
      page.getByRole("button", { name: 'Remove the filter "Kind: Genre"' }),
    ).toBeVisible();
    await expect(tagRow(page, "Robot")).toBeVisible();

    await chooseKind(page, "No kind");
    await expect(
      page.getByRole("button", { name: 'Remove the filter "No kind"' }),
    ).toBeVisible();
    await expect(page).toHaveURL(/[?&]kind=none/);
    await page.getByRole("searchbox", { name: "Search tags" }).fill("Robot");
    await expect(
      page.getByRole("heading", { name: "No tags match the current search and filters" }),
    ).toBeVisible();
    await expect(tagRow(page, "Robot")).toHaveCount(0);
  });
});
