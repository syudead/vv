/**
 * The link a field's text names when it holds an http or https URL, else undefined. The browser's serialization is
 * sent because the server's URL parser rejects forms the browser repairs, such as `https:example.com`.
 */
export function typedLink(value: string): string | undefined {
  try {
    const url = new URL(value.trim());
    return url.protocol === "http:" || url.protocol === "https:" ? url.href : undefined;
  } catch {
    return undefined;
  }
}

/** Whether a field's text is a link to fetch by rather than words or an ID. */
export function isLink(value: string): boolean {
  return typedLink(value) !== undefined;
}
