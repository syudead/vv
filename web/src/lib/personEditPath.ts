// Share this route contract between deferred loading and the person page's entry links.

export function personEditPath(id: number): string {
  return `/people/${String(id)}/edit`;
}

export function isPersonEditPath(pathname: string): boolean {
  return /^\/people\/[^/]+\/edit\/?$/i.test(pathname);
}
