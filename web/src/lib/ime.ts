import type { KeyboardEvent } from "react";

/**
 * keyCode 229 supports older browsers without isComposing; accept values to support both React and
 * native events.
 */
function isComposingKey(isComposing: boolean, keyCode: number): boolean {
  return isComposing || keyCode === 229;
}

/** Do not handle IME confirmation or navigation keys as input actions; match player/keyboard.ts. */
export function isComposingKeyEvent(event: KeyboardEvent<HTMLInputElement>): boolean {
  return isComposingKey(event.nativeEvent.isComposing, event.nativeEvent.keyCode);
}

/** Guard native Escape handling so IME cancellation does not close the dialog. */
export function isComposingNativeKeyEvent(event: globalThis.KeyboardEvent): boolean {
  return isComposingKey(event.isComposing, event.keyCode);
}
