// Share this route contract between deferred loading and the owner-only gate.

export function videoEditPath(videoId: number): string {
  return `/videos/${String(videoId)}/edit`;
}

export function isVideoEditPath(pathname: string): boolean {
  return /^\/videos\/[^/]+\/edit\/?$/i.test(pathname);
}
