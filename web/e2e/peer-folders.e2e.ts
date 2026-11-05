import { rm } from "node:fs/promises";
import path from "node:path";

import { type APIRequestContext, expect, type Page, test } from "@playwright/test";

import { type Node, nodeURL, secondNode, thirdNode } from "./nodes";
import {
  indexFolder,
  type MediaFolder,
  type PeerLinkBody,
  peerLinks,
  remoteOffset,
  removeFolders,
  required,
  type RunningNode,
  setUpOwner,
  startNode,
  unpairAll,
  type Video,
} from "./peer-nodes";

// Reads two real servers through the main node at once and switches a local and a remote media folder off
// and on from Settings, then stops one server and reads the rest. quickstart.md covers the other criteria by hand.

// The folder and video titles of media-fixtures.mjs, generateMultiPeerFixtures.
const query = "multipeer";
const localOff = "multipeer-local-a";
const localKept = "multipeer-local-b";
const remoteKept = "multipeer-b-kept";
const remoteOff = "multipeer-b-off";
const thirdTitle = "multipeer-c";
const allTitles = [localOff, localKept, remoteKept, remoteOff, thirdTitle];
// Longer than a read through a connected link, shorter than the 10-second wait for one that does not answer.
const promptly = 5_000;

/** A folder and its one video, with the video's id as the main node reads it once its link has a slot. */
interface Indexed {
  folder: MediaFolder;
  video: Video;
  node?: RunningNode;
}

const indexed = new Map<string, Indexed>();
let second: RunningNode | undefined;
let third: RunningNode | undefined;

function mediaRoot(): string {
  const root = process.env.MDM_E2E_MULTI_PEER_MEDIA_DIR;
  if (root === undefined)
    throw new Error("MDM_E2E_MULTI_PEER_MEDIA_DIR is not configured");
  return root;
}

function item(title: string): Indexed {
  return required(indexed.get(title), title);
}

async function index(request: APIRequestContext, title: string, node?: RunningNode) {
  const found = await indexFolder(request, path.join(mediaRoot(), title), title);
  indexed.set(title, { ...found, node });
}

async function startRemote(node: Node, titles: string[]): Promise<RunningNode> {
  const running = await startNode(node);
  await setUpOwner(running.request);
  for (const title of titles) await index(running.request, title, running);
  return running;
}

/** The main node's link to node, found by the address it was paired at. */
async function linkOf(
  request: APIRequestContext,
  node: RunningNode,
): Promise<PeerLinkBody> {
  const links = await peerLinks(request);
  return required(
    links.find((link) => link.address?.includes(node.node.addr) ?? false),
    `link to ${node.node.addr}`,
  );
}

/** The id the main node reads the video as: its own id for a local video, offset by its link's slot otherwise. */
async function shownId(request: APIRequestContext, title: string): Promise<number> {
  const { video, node } = item(title);
  if (node === undefined) return video.id;
  return video.id + (await linkOf(request, node)).slot * remoteOffset;
}

function card(page: Page, id: number) {
  return page.locator(`article[data-video-id="${String(id)}"]`);
}

function results(page: Page) {
  return page.getByRole("region", { name: "Search results", exact: true });
}

/** Checks that the search lists exactly the shown titles, and its count says so. */
async function expectSearch(
  page: Page,
  request: APIRequestContext,
  shown: string[],
  timeout?: number,
) {
  await page.goto(`/?q=${encodeURIComponent(query)}`);
  for (const title of shown) {
    await expect(card(page, await shownId(request, title))).toBeVisible({ timeout });
  }
  await expect(results(page).locator("article[data-video-id]")).toHaveCount(shown.length);
  await expect(results(page).getByRole("status", { name: "Search results" })).toHaveText(
    `${String(shown.length)} items`,
  );
}

function rootFolderCard(page: Page, name: string) {
  return page
    .locator("article[data-folder-path]")
    .filter({ has: page.locator("h3", { hasText: name }) });
}

/** The local video titled off is on every list or on none, and the other local video stays. */
async function expectLocalShown(page: Page, request: APIRequestContext, shown: boolean) {
  const off = await shownId(request, localOff);
  const kept = await shownId(request, localKept);
  await expectSearch(
    page,
    request,
    allTitles.filter((title) => shown || title !== localOff),
  );
  await page.goto("/");
  await expect(card(page, kept)).toBeVisible();
  await expect(card(page, off)).toHaveCount(shown ? 1 : 0);
  await page.goto("/folders");
  await expect(rootFolderCard(page, localKept)).toBeVisible();
  await expect(rootFolderCard(page, localOff)).toHaveCount(shown ? 1 : 0);
}

async function switchFolder(page: Page, label: string, enabled: boolean) {
  await page.goto("/settings/library");
  const control = page.getByRole("switch", { name: label, exact: true });
  await expect(control).toBeChecked({ checked: !enabled });
  // The switch moves before its save answers; the next page must read the saved switch.
  const saved = page.waitForResponse(
    (response) =>
      response.request().method() === "PUT" &&
      new URL(response.url()).pathname.endsWith("/enabled"),
  );
  await control.click();
  expect((await saved).status()).toBe(200);
  await expect(control).toBeChecked({ checked: enabled });
}

/** The Settings section of a link, found by the switch of one of its folders. */
function linkSection(page: Page, switchLabel: string) {
  return page
    .locator('section[data-slot="page-section"]')
    .filter({ has: page.getByRole("switch", { name: switchLabel, exact: true }) });
}

async function remoteSwitchLabel(request: APIRequestContext, title: string) {
  const { folder, node } = item(title);
  const link = await linkOf(request, required(node, `${title}'s node`));
  return `Show videos from ${folder.path} on ${link.name}`;
}

test.describe.serial("several read VVMDMs and folder switches", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(300_000);
    // A retry finds the main node still paired with the previous run's nodes.
    await unpairAll(request);
    second = await startRemote(secondNode, [remoteKept, remoteOff]);
    third = await startRemote(thirdNode, [thirdTitle]);
    await index(request, localOff);
    await index(request, localKept);
  });

  test.afterAll(async ({ request }) => {
    for (const node of [second, third]) {
      await node?.stop();
      await node?.request.dispose();
    }
    await unpairAll(request);
    await removeFolders(
      request,
      [localOff, localKept].flatMap((title) => indexed.get(title)?.folder ?? []),
    );
    await rm(secondNode.dataDir, { recursive: true, force: true });
    await rm(thirdNode.dataDir, { recursive: true, force: true });
  });

  test("受け入れ条件 1: 2台を続けてペアリングすると、どちらもつながり、ライブラリに3台の動画がそれぞれの印で並ぶ", async ({
    page,
    request,
  }) => {
    await page.goto("/settings/pairing");
    const nodes = [required(second, "second node"), required(third, "third node")];
    for (const [paired, node] of nodes.entries()) {
      const issued = await node.request.post("/api/peer/code", { data: {} });
      expect(issued.status()).toBe(201);
      const { code } = (await issued.json()) as { code: string };
      await page.getByLabel("Address of the other VVMDM").fill(nodeURL(node.node));
      await page.getByLabel("Its pairing code").fill(code);
      await page.getByRole("button", { name: "Pair", exact: true }).click();
      await expect(page.getByText("Connected", { exact: true })).toHaveCount(paired + 1, {
        timeout: 15_000,
      });
    }
    const links = await peerLinks(request);
    expect(links.map((link) => link.state)).toEqual(["connected", "connected"]);

    await expectSearch(page, request, allTitles);
    for (const title of allTitles) {
      const { node } = item(title);
      const link = card(page, await shownId(request, title))
        .getByRole("link")
        .first();
      if (node === undefined) {
        await expect(link).not.toHaveAccessibleDescription(/On /);
      } else {
        // Both test nodes run on one host, so their names match; the slot in the id names the link.
        const name = (await linkOf(request, node)).name;
        await expect(link).toHaveAccessibleDescription(
          new RegExp(`On ${name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`),
        );
      }
    }
  });

  test("受け入れ条件 4: 手元のフォルダをオフにすると、その動画がライブラリ・検索・フォルダ・ホームから消えて件数が減り、オンに戻すと戻る", async ({
    page,
    request,
  }) => {
    const { folder, video } = item(localOff);
    const label = `Show videos from ${folder.path}`;
    const scanBefore = (await (await request.get("/api/scans/current")).json()) as {
      id: number;
    };

    await switchFolder(page, label, false);
    await expectLocalShown(page, request, false);
    // The switch only narrows the reads: no scan ran, and the video is kept and answered by its id.
    const kept = await request.get(`/api/videos/${String(video.id)}`);
    expect(kept.status()).toBe(200);
    expect(await kept.json()).toMatchObject({
      title: localOff,
      probeState: "done",
      folderOff: true,
    });

    await switchFolder(page, label, true);
    await expectLocalShown(page, request, true);
    const scanAfter = (await (await request.get("/api/scans/current")).json()) as {
      id: number;
    };
    expect(scanAfter.id).toBe(scanBefore.id);
  });

  test("受け入れ条件 5: リモートのフォルダを1つオフにすると、そのフォルダの動画だけが消え、同じリモートのほかのフォルダの動画は出たまま", async ({
    page,
    request,
  }) => {
    const label = await remoteSwitchLabel(request, remoteOff);

    await switchFolder(page, label, false);
    await expectSearch(
      page,
      request,
      allTitles.filter((title) => title !== remoteOff),
    );
    // The switch changes only this node: the second node still lists the video itself.
    const own = await required(second, "second node").request.get(
      `/api/videos?limit=50&query=${encodeURIComponent(remoteOff)}`,
    );
    const { items } = (await own.json()) as { items: Video[] };
    expect(items.map((listed) => listed.title)).toEqual([remoteOff]);

    await switchFolder(page, label, true);
    await expectSearch(page, request, allTitles);
  });

  test("受け入れ条件 7: 1台を止めると、ライブラリと動画ページが待たずに手元ともう1台の分で出て、メディアフォルダ画面で止めた台がつながっていないと分かる", async ({
    page,
    request,
  }) => {
    const stopped = required(third, "third node");
    const stoppedLabel = await remoteSwitchLabel(request, thirdTitle);
    const runningLabel = await remoteSwitchLabel(request, remoteKept);
    const stoppedNodeId = (await linkOf(request, stopped)).nodeId;
    await stopped.stop();
    await expect
      .poll(
        async () =>
          (await peerLinks(request)).find((link) => link.nodeId === stoppedNodeId)?.state,
        { timeout: 30_000 },
      )
      .toBe("unreachable");

    await expectSearch(
      page,
      request,
      allTitles.filter((title) => title !== thirdTitle),
      promptly,
    );
    for (const title of [localKept, remoteKept]) {
      await page.goto(`/videos/${String(await shownId(request, title))}`);
      await expect(page.getByRole("heading", { level: 1 })).toHaveText(title, {
        timeout: promptly,
      });
    }

    await page.goto("/settings/library");
    const stoppedSection = linkSection(page, stoppedLabel);
    await expect(
      stoppedSection.getByText("Not connected", { exact: true }),
    ).toBeVisible();
    await expect(
      stoppedSection.getByRole("switch", { name: stoppedLabel, exact: true }),
    ).toBeChecked();
    await expect(
      linkSection(page, runningLabel).getByText("Connected", { exact: true }),
    ).toBeVisible();
  });
});
