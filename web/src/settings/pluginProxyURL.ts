/** Count Unicode code points, matching domain.WorkInfoPluginProxyURLMaxLen. */
const MAX_CHARACTERS = 2048;

const PROXY_URL = /^(?:https?|socks5):\/\/([^/?#\s]+)$/i;

const HOST_PORT = /^(?:\[[^\]]+\]|[^:[\]@]+)(?::(\d+))?$/;

/**
 * Empty disables the proxy; reject invalid drafts locally and let server validation handle
 * remaining differences.
 */
export function isPluginProxyURL(value: string): boolean {
  if (value === "") return true;
  if ([...value].length > MAX_CHARACTERS) return false;
  const authority = PROXY_URL.exec(value)?.[1];
  if (authority === undefined) return false;
  const hostPort = authority.slice(authority.lastIndexOf("@") + 1);
  const port = HOST_PORT.exec(hostPort);
  if (port === null) return false;
  if (port[1] === undefined) return true;
  const n = Number(port[1]);
  return n >= 1 && n <= 65535;
}
