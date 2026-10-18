/** Preserve the return list URL and opt into playback when opening a video directly. */
export function cardLinkState(
  backTo: string,
  autoplay: boolean | undefined,
): { from: string; autoplay?: true } {
  return autoplay === true ? { from: backTo, autoplay: true } : { from: backTo };
}
