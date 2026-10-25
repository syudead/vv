import { type ChildProcess, spawn } from "node:child_process";
import { once } from "node:events";
import { rm } from "node:fs/promises";
import path from "node:path";

import {
  type APIRequestContext,
  expect,
  type Page,
  request as playwrightRequest,
  test,
} from "@playwright/test";

import { mainNode, nodeEnv, nodeURL, secondNode, serverBinary } from "./nodes";
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

let second: ChildProcess | undefined;
let secondRequest: APIRequestContext | undefined;
let localFolder: MediaFolder | undefined;
let peerName = "";
let localVideo: Video | undefined;
let remoteVideo: Video | undefined;

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

function card(page: Page, video: Video) {
  return page.locator(`article[data-video-id="${String(video.id)}"]`);
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
      (await request.delete("/api/peer", { headers: mutationHeaders })).status(),
    ).toBe(204);
    secondRequest = await startSecondNode();
    remoteVideo = { ...(await setUpSecondNode(secondRequest)) };
    remoteVideo.id += remoteOffset;

    const local = await indexFolder(request, path.join(mediaRoot(), "local"), localTitle);
    localFolder = local.folder;
    localVideo = local.video;
  });

  test.afterAll(async ({ request }) => {
    await stopSecondNode();
    await secondRequest?.dispose();
    const unpaired = await request.delete("/api/peer", { headers: mutationHeaders });
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

  test("設定の Pairing で相手のコードを入れると、両方の台が connected になる", async ({
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
    // The form's default is the browser's origin, which is the Vite server; the peer calls the node itself.
    await page.getByLabel("Address of this VVMDM").fill(nodeURL(mainNode));
    await page.getByRole("button", { name: "Pair", exact: true }).click();
    await expect(page.getByText("Connected", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    const own = (await (await request.get("/api/peer")).json()) as {
      state: string;
      peer: { name: string };
    };
    expect(own.state).toBe("connected");
    peerName = own.peer.name;
    // The node that showed the code connects back on its own; when its first attempt comes before the
    // main node stored the pairing, the main node's handshake makes it retry at once.
    await expect
      .poll(
        async () =>
          ((await (await peer.get("/api/peer")).json()) as { state: string }).state,
        { timeout: 20_000 },
      )
      .toBe("connected");
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
