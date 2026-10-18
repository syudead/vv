/** Restore the previously focused element after the temporary input captures selection. */
function copyWithSelection(text: string): boolean {
  const previous = document.activeElement;
  const field = document.createElement("textarea");
  field.value = text;
  field.setAttribute("readonly", "");
  field.style.position = "fixed";
  field.style.opacity = "0";
  document.body.appendChild(field);
  field.select();
  try {
    // This deprecated API is the only fallback on insecure connections.
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    field.remove();
    if (previous instanceof HTMLElement && previous.isConnected) previous.focus();
  }
}

/**
 * Use Clipboard API on secure origins, falling back to synchronous selection copying for HTTP LAN
 * addresses.
 */
export function copyText(
  text: string,
  { onCopied, onFailed }: { onCopied: () => void; onFailed: () => void },
): void {
  const fallback = () => (copyWithSelection(text) ? onCopied() : onFailed());
  if (navigator.clipboard === undefined) {
    fallback();
    return;
  }
  void navigator.clipboard.writeText(text).then(onCopied, fallback);
}
