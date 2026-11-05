import { type ChildProcess, spawn } from "node:child_process";
import { once } from "node:events";
import { rm } from "node:fs/promises";

import {
  type APIRequestContext,
  expect,
  request as playwrightRequest,
} from "@playwright/test";

import { type Node, nodeEnv, nodeURL, serverBinary } from "./nodes";
import { ownerAccount } from "./owner-account";

// Starting, filling and stopping the extra real servers the pairing specs read through the main node.

export interface Video {
  id: number;
  title: string;
  probeState: string;
}

export interface MediaFolder {
  id: number;
  version: number;
  path: string;
}

export interface PeerLinkBody {
  slot: number;
  nodeId: string;
  name: string;
  state: string;
  address?: string;
}

const origin = "http://127.0.0.1:15173";
export const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

/** The offset of the first slot; a remote id is its own id plus slot × remoteOffset. */
export const remoteOffset = 2 ** 40;

export function required<T>(value: T | undefined, name: string): T {
  if (value === undefined) throw new Error(`${name} is not ready`);
  return value;
}

/** A server started from an empty data directory, with a request context on it. */
export interface RunningNode {
  node: Node;
  request: APIRequestContext;
  /** Stops the server and waits for it to exit; a second call does nothing. */
  stop: () => Promise<void>;
}

export async function startNode(node: Node): Promise<RunningNode> {
  await rm(node.dataDir, { recursive: true, force: true });
  let child: ChildProcess | undefined = spawn(serverBinary, [], {
    env: { ...process.env, ...nodeEnv(node) },
    stdio: ["ignore", "ignore", "inherit"],
  });
  const stop = async () => {
    const running = child;
    child = undefined;
    if (running === undefined || running.exitCode !== null) return;
    const exited = once(running, "exit");
    running.kill("SIGTERM");
    await exited;
  };
  const request = await playwrightRequest.newContext({ baseURL: nodeURL(node) });
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
  return { node, request, stop };
}

/** Creates the owner on a fresh node and finishes its onboarding, so its library answers. */
export async function setUpOwner(request: APIRequestContext): Promise<void> {
  const setup = await request.post("/api/auth/setup", { data: ownerAccount });
  expect(setup.status()).toBe(200);
  const onboarding = await request.put("/api/onboarding", { data: { step: "done" } });
  expect(onboarding.status()).toBe(200);
}

/** Adds the folder on the node behind request, scans it and returns its video titled title once probed. */
export async function indexFolder(
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

export async function peerLinks(request: APIRequestContext): Promise<PeerLinkBody[]> {
  const response = await request.get("/api/peer");
  expect(response.status()).toBe(200);
  return ((await response.json()) as { links: PeerLinkBody[] }).links;
}

/** Removes every link the node of request reads. */
export async function unpairAll(request: APIRequestContext): Promise<void> {
  for (const link of await peerLinks(request)) {
    const removed = await request.delete(`/api/peer/links/${link.nodeId}`, {
      headers: mutationHeaders,
    });
    expect(removed.status()).toBe(204);
  }
}

/** Deletes local media folders through the main node, at the version it lists now. */
export async function removeFolders(
  request: APIRequestContext,
  folders: readonly MediaFolder[],
): Promise<void> {
  const listed = (await (
    await request.get("/api/media-folders")
  ).json()) as MediaFolder[];
  for (const folder of listed.filter((item) =>
    folders.some((mine) => mine.id === item.id),
  )) {
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  }
}
