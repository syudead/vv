import { useEffect, useState } from "react";

import type { Video } from "../../api/client";

//
//
// Keep Video handoffs in memory, not history state, to avoid stale data on reload or later navigation.
// Read during render without consuming for StrictMode; clear in the receiving effect.

let toEditor: Video | null = null;
let toPage: PageHandoff | null = null;

export interface PageHandoff {
  id: number;
  video?: Video;
}

export function handVideoToEditor(video: Video): void {
  toEditor = video;
}

export function peekVideoForEditor(id: number): Video | undefined {
  return toEditor !== null && toEditor.id === id ? toEditor : undefined;
}

export function clearVideoForEditor(): void {
  toEditor = null;
}

/** Return saved data when present and restore entry focus after either save or cancel. */
export function handBackToPage(id: number, video?: Video): void {
  toPage = { id, video };
}

export function peekPageHandoff(id: number): PageHandoff | null {
  return toPage !== null && toPage.id === id ? toPage : null;
}

export function clearPageHandoff(): void {
  toPage = null;
}

export interface ReceivedHandoff {
  video?: Video;
  focusEntry: (videoId: number) => boolean;
}

export function usePageHandoff(id: number): ReceivedHandoff {
  const [handoff] = useState(() => peekPageHandoff(id));
  useEffect(() => clearPageHandoff(), []);
  return {
    video: handoff?.video,
    focusEntry: (videoId) => handoff !== null && handoff.id === videoId,
  };
}
