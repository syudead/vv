import { expect, test } from "@playwright/test";

// The organization screens across routes and the real server: the card grid creates, the edit page saves, the page shows it.

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const created: number[] = [];

test.afterEach(async ({ request }) => {
  for (const id of created.splice(0)) {
    await request.delete(`/api/organizations/${String(id)}`, {
      headers: mutationHeaders,
    });
  }
});

test("creates an organization from the grid, edits it and finds the change on its page", async ({
  page,
}, testInfo) => {
  // A retry meets the server the failed attempt left, so each attempt names its own organization.
  const name = `E2E Studio ${String(testInfo.retry)}-${String(Date.now())}`;

  await page.goto("/organizations");
  await page.getByRole("button", { name: "New organization" }).first().click();
  const dialog = page.getByRole("dialog", { name: "New organization" });
  await dialog.getByRole("textbox", { name: "Name" }).fill(name);
  await dialog.getByRole("button", { name: "Create" }).click();

  await expect(page.getByRole("heading", { name, level: 1 })).toBeVisible();
  const id = Number(/\/organizations\/(\d+)$/.exec(new URL(page.url()).pathname)?.[1]);
  expect(id).toBeGreaterThan(0);
  created.push(id);

  // A new organization has no profile, so the facts card offers Edit beside the actions row.
  await page.getByRole("link", { name: "Edit" }).first().click();
  await expect(page).toHaveURL(new RegExp(`/organizations/${String(id)}/edit$`));
  await page.getByRole("textbox", { name: "Location" }).fill("Kyoto");
  await page.getByRole("button", { name: "Save" }).click();

  await expect(page).toHaveURL(new RegExp(`/organizations/${String(id)}$`));
  await expect(page.getByRole("heading", { name, level: 1 })).toBeVisible();
  await expect(page.getByText("Kyoto", { exact: true })).toBeVisible();

  await page.goto(`/organizations?q=${encodeURIComponent(name)}`);
  await expect(page.getByRole("link", { name: new RegExp(name) })).toHaveAttribute(
    "href",
    `/organizations/${String(id)}`,
  );
});
