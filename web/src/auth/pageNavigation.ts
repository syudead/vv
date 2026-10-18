// Audience changes require a full navigation to discard caches loaded for the previous audience.
// Isolate navigation so tests can replace it.

export function assignPage(url: string): void {
  window.location.assign(url);
}

export function reloadPage(): void {
  window.location.reload();
}

export function currentPath(location: {
  pathname: string;
  search: string;
  hash: string;
}): string {
  return `${location.pathname}${location.search}${location.hash}`;
}

export function loginPath(next: string): string {
  return `/login?${new URLSearchParams({ next })}`;
}
