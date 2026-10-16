/**
 * プロキシの URL の最大の長さ（文字数。サーバーと同じく Unicode のコードポイントで数える）。
 * サーバーの domain.WorkInfoPluginProxyURLMaxLen と同じ。
 */
const MAX_CHARACTERS = 2048;

/** scheme と、パスもクエリもフラグメントも無い残り（ユーザー情報とホストとポート）。 */
const PROXY_URL = /^(?:https?|socks5):\/\/([^/?#\s]+)$/i;

/** ホスト（[IPv6] か、コロンを含まない名前）と任意のポート。 */
const HOST_PORT = /^(?:\[[^\]]+\]|[^:[\]@]+)(?::(\d+))?$/;

/**
 * isPluginProxyURL は、値がサーバーの受け付けるプロキシの URL かを返す
 * （specs/056-plugin-tooling/contracts/screen-api.md、サーバーの規則は
 * domain.ParseWorkInfoPluginProxyURL）。`""` はプロキシなしで受け付ける。画面はこれに外れた値を
 * 送らず、サーバーの `400` と同じ誤りを出す。細部で食い違えばサーバーの `400` が同じ誤りを出す。
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
