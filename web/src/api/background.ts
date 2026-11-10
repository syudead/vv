import { request } from "./client";
import type { components } from "./gen/openapi";

export type BackgroundWork = components["schemas"]["BackgroundWork"];
export type BackgroundActivity = components["schemas"]["BackgroundActivity"];
export type BackgroundGroup = components["schemas"]["BackgroundGroup"];
export type BackgroundGroupName = components["schemas"]["BackgroundGroupName"];

/** Owner-only: the pause switch, the work running now and the work left per group. */
export function getBackgroundWork(signal?: AbortSignal): Promise<BackgroundWork> {
  return request<BackgroundWork>("/api/background", { signal });
}

/** Owner-only: saves the switch and answers the status after the change. */
export function setBackgroundPause(
  paused: boolean,
  signal?: AbortSignal,
): Promise<BackgroundWork> {
  return request<BackgroundWork>("/api/background/pause", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ paused }),
    signal,
  });
}
