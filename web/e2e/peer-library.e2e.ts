import { type ChildProcess, spawn } from "node:child_process";
import { once } from "node:events";
import { rm } from "node:fs/promises";
import path from "node:path";
import { DatabaseSync } from "node:sqlite";

import {
  type APIRequestContext,
  expect,
  type Page,
  request as playwrightRequest,
  test,
} from "@playwright/test";

import { nodeEnv, nodeURL, secondNode, serverBinary } from "./nodes";
import { ownerAccount } from "./owner-account";

// Pairs the main node with a second real server from the Settings page, then reads, plays and loses the
// second node's videos through the main node, as the owner sees them in the browser.

interface Video {
  id: number;
  title: string;
  probeState: string;
}

interface MediaFolder {
  id: number;
  version: number;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
// media-fixtures.mjs の generatePeerFixtures と同じ題名。
const query = "pairing";
const localTitle = "pairing-local";
const remoteTitle = "pairing-remote";
const remoteOffset = 2 ** 40;
// The person face recognition found in the second node's video, and the main node's person its face moves to.
const faceName = "pairing-face";
const targetName = "pairing-target";
// A 16 x 16 grey JPEG standing in for the crop face recognition keeps.
const faceCrop = Buffer.from(
  "/9j/4AAQSkZJRgABAgAAAQABAAD//gAQTGF2YzYwLjMxLjEwMgD/2wBDAAgUFBcUFxsbGxsbGyAeICEhISAgICAhISEkJCQqKiokJCQhISQkKCgqKi4vLisrKisvLzIyMjw8OTlGRkhWVmf/xABKAAEAAAAAAAAAAAAAAAAAAAAAAQEAAAAAAAAAAAAAAAAAAAAAEAEAAAAAAAAAAAAAAAAAAAAAEQEAAAAAAAAAAAAAAAAAAAAA/8AAEQgAEAAQAwEiAAIRAAMRAP/aAAwDAQACEQMRAD8AAA//2Q==",
  "base64",
);

let second: ChildProcess | undefined;
let secondRequest: APIRequestContext | undefined;
let localFolder: MediaFolder | undefined;
let peerName = "";
let localVideo: Video | undefined;
let remoteVideo: Video | undefined;
let remoteFacePerson: number | undefined;

function mediaRoot(): string {
  const root = process.env.MDM_E2E_PEER_MEDIA_DIR;
  if (root === undefined) throw new Error("MDM_E2E_PEER_MEDIA_DIR is not configured");
  return root;
}

function required<T>(value: T | undefined, name: string): T {
  if (value === undefined) throw new Error(`${name} is not ready`);
  return value;
}

async function startSecondNode(): Promise<APIRequestContext> {
  await rm(secondNode.dataDir, { recursive: true, force: true });
  const child = spawn(serverBinary, [], {
    env: { ...process.env, ...nodeEnv(secondNode) },
    stdio: ["ignore", "ignore", "inherit"],
  });
  second = child;
  const request = await playwrightRequest.newContext({ baseURL: nodeURL(secondNode) });
  await expect
    .poll(
      async () => {
        try {
          return (await request.get("/api/health")).status();
        } catch {
          return 0;
        }
      },
      { timeout: 60_000 },
    )
    .toBe(200);
  return request;
}

async function stopSecondNode() {
  const child = second;
  second = undefined;
  if (child === undefined || child.exitCode !== null) return;
  const exited = once(child, "exit");
  child.kill("SIGTERM");
  await exited;
}

/** Adds the folder on the node behind request, scans it and returns its videos once probed. */
async function indexFolder(
  request: APIRequestContext,
  folderPath: string,
  title: string,
): Promise<{ folder: MediaFolder; video: Video }> {
  const created = await request.post("/api/media-folders", {
    headers: { "Content-Type": "application/json" },
    data: { path: folderPath },
  });
  expect(created.status()).toBe(201);
  const folder = (await created.json()) as MediaFolder;
  const scan = await request.post("/api/scans", { data: {} });
  expect(scan.status()).toBe(202);
  const scanId = ((await scan.json()) as { id: number }).id;
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/scans/current");
        const current = (await response.json()) as { id: number; state: string };
        return current.id === scanId ? current.state : "running";
      },
      { timeout: 60_000 },
    )
    .toBe("done");
  let found: Video | undefined;
  await expect
    .poll(
      async () => {
        const response = await request.get(
          `/api/videos?limit=50&query=${encodeURIComponent(title)}`,
        );
        const { items } = (await response.json()) as { items: Video[] };
        found = items.find((item) => item.title === title);
        return found?.probeState;
      },
      { timeout: 60_000 },
    )
    .toBe("done");
  return { folder, video: required(found, title) };
}

async function setUpSecondNode(request: APIRequestContext): Promise<Video> {
  const setup = await request.post("/api/auth/setup", { data: ownerAccount });
  expect(setup.status()).toBe(200);
  const onboarding = await request.put("/api/onboarding", { data: { step: "done" } });
  expect(onboarding.status()).toBe(200);
  const { video } = await indexFolder(
    request,
    path.join(mediaRoot(), "remote"),
    remoteTitle,
  );
  return video;
}

async function createPerson(
  request: APIRequestContext,
  name: string,
  headers?: Record<string, string>,
): Promise<number> {
  const created = await request.post("/api/people", { headers, data: { name } });
  expect(created.status()).toBe(201);
  return ((await created.json()) as { id: number }).id;
}

async function findPerson(
  request: APIRequestContext,
  name: string,
): Promise<number | undefined> {
  const found = await request.get(`/api/people?q=${encodeURIComponent(name)}`);
  expect(found.status()).toBe(200);
  const { items } = (await found.json()) as { items: { id: number; name: string }[] };
  return items.find((item) => item.name === name)?.id;
}

/**
 * Stores on the second node one appearance of a new performer faceName in its video, seen from 0:10 to 0:14, as a
 * finished face analysis would; the e2e servers have no face runtime to find it. Returns the person's id there.
 */
async function seedRemoteFace(request: APIRequestContext, video: Video): Promise<number> {
  const personId = await createPerson(request, faceName);
  // The People tab lists credited people, as face analysis credits the person it links.
  const credited = await request.put(
    `/api/videos/${String(video.id - remoteOffset)}/work-info`,
    { data: { people: [{ role: "performer", person: { id: personId } }] } },
  );
  expect(credited.ok()).toBe(true);
  const db = new DatabaseSync(path.join(secondNode.dataDir, "mdm.db"));
  try {
    db.exec("pragma busy_timeout = 5000");
    const row = db
      .prepare("select content_key from videos where id = ?")
      .get(video.id - remoteOffset) as { content_key: string } | undefined;
    const contentKey = required(row, "content key").content_key;
    db.prepare(
      `insert into face_results (content_key, state, recognizer, updated_at)
       values (?, 'done', 'sface_int8', ?)`,
    ).run(contentKey, Date.now());
    db.prepare(
      `insert into face_appearances (content_key, recognizer, person_id, tracks, good_seconds, track_count, crop)
       values (?, 'sface_int8', ?, ?, 4, 1, ?)`,
    ).run(contentKey, personId, JSON.stringify([[10, 14, 0.5, 0.5, 0.2]]), faceCrop);
  } finally {
    db.close();
  }
  return personId;
}

function card(page: Page, video: Video) {
  return page.locator(`article[data-video-id="${String(video.id)}"]`);
}

/** The second node's own id of a video this node reads in the offset form. */
function peerId(video: Video): string {
  return String(video.id - remoteOffset);
}

async function peerHistoryCount(request: APIRequestContext): Promise<number> {
  const response = await request.get("/api/watch-history?limit=50");
  expect(response.status()).toBe(200);
  const { items } = (await response.json()) as { items: { video?: { title: string } }[] };
  return items.filter((item) => item.video?.title === remoteTitle).length;
}

/** Records the media time of the first `playing` event, where the player itself started. */
async function recordFirstPlaying(page: Page) {
  await page.evaluate(() => {
    const record = window as unknown as { firstPlayingAt?: number };
    delete record.firstPlayingAt;
    document.addEventListener(
      "playing",
      (event) => {
        if (
          record.firstPlayingAt === undefined &&
          event.target instanceof HTMLVideoElement
        )
          record.firstPlayingAt = event.target.currentTime;
      },
      { capture: true },
    );
  });
}

async function firstPlayingAt(page: Page): Promise<number | null> {
  return page.evaluate(
    () => (window as unknown as { firstPlayingAt?: number }).firstPlayingAt ?? null,
  );
}

/** Starts playback and waits for the first frames, failing with the player's alert when it shows one. */
async function play(page: Page, video: Video) {
  await page.goto(`/videos/${String(video.id)}`);
  await page.waitForFunction(
    () => {
      const failed = document.querySelector('[data-player-frame] [role="alert"]');
      const element = document.querySelector<HTMLVideoElement>("video.vjs-tech");
      return failed !== null || (element !== null && element.readyState >= 1);
    },
    undefined,
    { timeout: 10_000 },
  );
  await expect(page.locator("[data-player-frame]").getByRole("alert")).toHaveCount(0);
  await page.locator(".vjs-big-play-button").click();
  await page.waitForFunction(() => {
    const element = document.querySelector("video");
    return (
      element !== null &&
      !element.paused &&
      element.readyState >= 2 &&
      element.currentTime > 0.1
    );
  });
}

test.describe.serial("paired library", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(180_000);
    // A retry finds the main node still paired with the previous run's second node.
    expect(
      (await request.delete("/api/peer/link", { headers: mutationHeaders })).status(),
    ).toBe(204);
    secondRequest = await startSecondNode();
    remoteVideo = { ...(await setUpSecondNode(secondRequest)) };
    remoteVideo.id += remoteOffset;
    remoteFacePerson = remoteOffset + (await seedRemoteFace(secondRequest, remoteVideo));

    const local = await indexFolder(request, path.join(mediaRoot(), "local"), localTitle);
    localFolder = local.folder;
    localVideo = local.video;
  });

  test.afterAll(async ({ request }) => {
    await stopSecondNode();
    await secondRequest?.dispose();
    const unpaired = await request.delete("/api/peer/link", { headers: mutationHeaders });
    expect(unpaired.status()).toBe(204);
    if (localFolder !== undefined) {
      const removed = await request.delete(
        `/api/media-folders/${String(localFolder.id)}?version=${String(localFolder.version)}`,
        { headers: mutationHeaders },
      );
      expect(removed.status()).toBe(204);
    }
    await rm(secondNode.dataDir, { recursive: true, force: true });
  });

  test("設定の Pairing で相手のコードを入れると、この台が connected になり、相手にはこの台が読む台として出る", async ({
    page,
    request,
  }) => {
    const peer = required(secondRequest, "second node");
    const issued = await peer.post("/api/peer/code", { data: {} });
    expect(issued.status()).toBe(201);
    const { code } = (await issued.json()) as { code: string };

    await page.goto("/settings/pairing");
    await page.getByLabel("Address of the other VVMDM").fill(nodeURL(secondNode));
    await page.getByLabel("Its pairing code").fill(code);
    await page.getByRole("button", { name: "Pair", exact: true }).click();
    await expect(page.getByText("Connected", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    const own = (await (await request.get("/api/peer")).json()) as {
      state: string;
      link: { name: string };
      grant?: unknown;
    };
    expect(own.state).toBe("connected");
    expect(own.grant).toBeUndefined();
    peerName = own.link.name;
    // The node that showed the code reads nothing: it holds the grant and no link, and never connects here.
    const other = (await (await peer.get("/api/peer")).json()) as {
      state: string;
      link?: unknown;
      grant: { name: string };
    };
    expect(other.state).toBe("unpaired");
    expect(other.link).toBeUndefined();
    expect(other.grant.name).toBe(
      ((await (await request.get("/api/peer")).json()) as { node: { name: string } }).node
        .name,
    );
  });

  test("ライブラリに両方の台の動画が出て、相手の動画にだけ目印が付く", async ({
    page,
  }) => {
    const local = required(localVideo, "local video");
    const remote = required(remoteVideo, "remote video");
    await page.goto(`/?q=${encodeURIComponent(query)}`);
    const results = page.getByRole("region", { name: "Search results", exact: true });

    const remoteLink = card(page, remote).getByRole("link").first();
    await expect(remoteLink).toBeVisible();
    await expect(remoteLink).toHaveAccessibleDescription(
      new RegExp(`On ${peerName.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`),
    );
    await expect(card(page, remote).locator("h3")).toHaveText(remoteTitle);

    const localLink = card(page, local).getByRole("link").first();
    await expect(localLink).toBeVisible();
    await expect(localLink).not.toHaveAccessibleDescription(/On /);
    await expect(results.locator("article[data-video-id]")).toHaveCount(2);
  });

  test("相手の動画を再生し、隣の字幕を出し、シークしても続けて再生する", async ({
    page,
  }) => {
    test.setTimeout(60_000);
    const remote = required(remoteVideo, "remote video");
    const ranges: number[] = [];
    page.on("response", (response) => {
      if (
        new URL(response.url()).pathname === `/api/videos/${String(remote.id)}/stream`
      ) {
        ranges.push(response.status());
      }
    });
    await page.setViewportSize({ width: 1280, height: 800 });
    await play(page, remote);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(remoteTitle);
    expect(ranges).toContain(206);

    await page.locator(".video-js").hover();
    await page.locator(".vjs-control-bar > .vjs-subs-caps-button").hover();
    await page.getByRole("menuitemradio", { name: /^Default/ }).click();
    const display = page.locator(".vjs-text-track-display");
    await expect(display).toContainText("Remote subtitle cue");

    await page.evaluate(() => {
      const element = document.querySelector("video");
      if (element === null) throw new Error("video is missing");
      element.currentTime = 20;
    });
    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return (
        element !== null &&
        !element.seeking &&
        !element.paused &&
        element.readyState >= 2 &&
        element.currentTime > 20.1
      );
    });
    await expect(display).toContainText("Remote subtitle cue");
  });

  test("ライブラリを視聴日で並べると、台に関係なく両方の台の動画が視聴の新しい順に並ぶ", async ({
    page,
    request,
  }) => {
    const local = required(localVideo, "local video");
    const remote = required(remoteVideo, "remote video");
    const saveProgress = async (video: Video) => {
      const response = await request.put(`/api/videos/${String(video.id)}/progress`, {
        headers: mutationHeaders,
        data: { positionMs: 1_000 },
      });
      expect(response.status()).toBe(200);
      return Date.parse(((await response.json()) as { updatedAt: string }).updatedAt);
    };
    // The played order is kept to the second; the remote video is played in a later second than the local one.
    const localPlayedAt = await saveProgress(local);
    await expect.poll(() => saveProgress(remote)).toBeGreaterThan(localPlayedAt);

    await page.goto(`/?q=${encodeURIComponent(query)}&sort=playedDesc`);
    const results = page.getByRole("region", { name: "Search results", exact: true });
    const cards = results.locator("article[data-video-id]");
    await expect
      .poll(() =>
        cards.evaluateAll((elements) =>
          elements.map((element) => element.getAttribute("data-video-id")),
        ),
      )
      .toEqual([String(remote.id), String(local.id)]);
  });

  test("相手の動画の people タブに相手の台が見つけた顔の出現位置が出て、そこから再生する", async ({
    page,
  }) => {
    const remote = required(remoteVideo, "remote video");
    await page.goto(`/videos/${String(remote.id)}`);
    await page
      .getByRole("tablist", { name: "About this video" })
      .getByRole("tab", { name: "People" })
      .click();
    const spans = page.getByRole("group", { name: `When ${faceName} is on screen` });
    await spans.getByRole("button", { name: "0:10–0:14" }).click();
    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return element !== null && !element.paused && element.currentTime >= 10;
    });
  });

  test("相手の人物の顔をこの台の人物へ移すと、相手の台でその名前の人物の顔になる", async ({
    page,
    request,
  }) => {
    const peer = required(secondRequest, "second node");
    const person = required(remoteFacePerson, "remote face person");
    // A retry finds the main node's target from the previous run.
    if ((await findPerson(request, targetName)) === undefined) {
      await createPerson(request, targetName, mutationHeaders);
    }

    await page.goto(`/people/${String(person)}/faces?tab=faces`);
    await page.getByRole("button", { name: "Select" }).click();
    await page.getByRole("checkbox", { name: `Select ${remoteTitle}` }).click();
    await page.getByRole("button", { name: "Move to person…" }).click();
    const dialog = page.getByRole("dialog", { name: "Move 1 face to…" });
    await dialog.getByRole("combobox", { name: "Search by name" }).fill(targetName);
    await dialog.getByRole("option", { name: new RegExp(targetName) }).click();
    await dialog.getByRole("button", { name: "Move", exact: true }).click();
    await expect(page.getByText(`Moved to ${targetName}`)).toBeVisible();

    // The move made the name on the second node, which now holds the face.
    const target = required(
      await findPerson(peer, targetName),
      "the second node's target",
    );
    const moved = await peer.get(`/api/people/${String(target)}/appearances`);
    const appearances = (await moved.json()) as { items: { title: string }[] };
    expect(appearances.items.map((item) => item.title)).toEqual([remoteTitle]);
  });

  test("この台で相手の動画を途中まで再生すると、相手の台に続きの位置と視聴履歴の項目が残る", async ({
    page,
  }) => {
    test.setTimeout(60_000);
    const peer = required(secondRequest, "second node");
    const remote = required(remoteVideo, "remote video");
    const before = await peerHistoryCount(peer);
    await play(page, remote);
    await page.evaluate(() => {
      const element = document.querySelector("video");
      if (element === null) throw new Error("video is missing");
      element.currentTime = 12;
    });
    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return element !== null && !element.seeking && element.currentTime > 12.5;
    });
    // A pause saves the position at once.
    await page.evaluate(() => document.querySelector("video")?.pause());

    await expect
      .poll(async () => {
        const response = await peer.get(`/api/videos/${peerId(remote)}`);
        const video = (await response.json()) as { progress?: { positionMs: number } };
        return video.progress?.positionMs ?? 0;
      })
      .toBeGreaterThanOrEqual(12_000);
    await expect.poll(() => peerHistoryCount(peer)).toBeGreaterThan(before);
  });

  test("相手の台で途中まで再生した相手の動画は、この台の視聴履歴に出て、その位置から再生が始まる", async ({
    page,
  }) => {
    test.setTimeout(60_000);
    const peer = required(secondRequest, "second node");
    const remote = required(remoteVideo, "remote video");
    // The second node's own playback: an identified save creates its history entry there. The 30-second
    // video counts as watched from 15 seconds on, which would start over instead of resuming.
    const saved = await peer.put(`/api/videos/${peerId(remote)}/progress`, {
      data: { positionMs: 10_000, playbackId: crypto.randomUUID() },
    });
    expect(saved.status()).toBe(200);

    await page.goto("/history");
    const resume = page
      .getByRole("list", { name: "Watch history" })
      .getByRole("link", { name: `Resume ${remoteTitle}` })
      .first();
    await expect(resume).toBeVisible();
    await recordFirstPlaying(page);
    await resume.click();
    await expect(page).toHaveURL(`/videos/${String(remote.id)}`);
    await expect.poll(() => firstPlayingAt(page), { timeout: 15_000 }).not.toBeNull();
    const startedAt = (await firstPlayingAt(page)) ?? 0;
    expect(startedAt).toBeGreaterThanOrEqual(10);
    expect(startedAt).toBeLessThan(12);
    await page.evaluate(() => document.querySelector("video")?.pause());
  });

  test("この台で相手の動画をお気に入りにすると、相手の台でもお気に入りになり、両方の台のお気に入りの絞り込みに出る", async ({
    page,
  }) => {
    const peer = required(secondRequest, "second node");
    const remote = required(remoteVideo, "remote video");
    await page.goto(`/videos/${String(remote.id)}`);
    const favorite = page.getByRole("button", { name: "Favorite", exact: true });
    await expect(favorite).toHaveAttribute("aria-pressed", "false");
    await favorite.click();
    await expect(favorite).toHaveAttribute("aria-pressed", "true");

    const own = (await (await peer.get(`/api/videos/${peerId(remote)}`)).json()) as {
      favorite?: boolean;
    };
    expect(own.favorite).toBe(true);
    const listed = await peer.get(
      `/api/videos?limit=50&favorite=true&query=${encodeURIComponent(query)}`,
    );
    const { items } = (await listed.json()) as { items: Video[] };
    expect(items.map((item) => item.title)).toEqual([remoteTitle]);

    await page.goto(`/?q=${encodeURIComponent(query)}&fav=1`);
    const results = page.getByRole("region", { name: "Search results", exact: true });
    await expect(card(page, remote)).toBeVisible();
    await expect(results.locator("article[data-video-id]")).toHaveCount(1);
  });

  test("相手の台が止まると通知が出て、相手の動画が一覧から外れる", async ({ page }) => {
    const local = required(localVideo, "local video");
    const remote = required(remoteVideo, "remote video");
    await page.goto(`/?q=${encodeURIComponent(query)}`);
    await expect(card(page, remote)).toBeVisible();

    await stopSecondNode();
    const notice = page.getByRole("alert").filter({ hasText: `Can't reach ${peerName}` });
    await expect(notice).toBeVisible({ timeout: 15_000 });
    await expect(notice.getByRole("link", { name: "Open Pairing" })).toBeVisible();

    await page.reload();
    await expect(card(page, local)).toBeVisible();
    await expect(card(page, remote)).toHaveCount(0);
    await expect(notice).toBeVisible();
  });
});
