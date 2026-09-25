import { RequestFailed, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";

// 認証の経路（specs/016-single-account-auth/contracts/auth-api.md §2〜§4）。
// 画面は fetch せず、ここを通す。所有者として描いている間の 401 で読み直す
// client.ts の仕組みには乗せない。ここの誤りは画面がそのまま扱う。

export type AuthSession = components["schemas"]["AuthSession"];
export type AuthState = AuthSession["state"];
export type AuthRedirect = components["schemas"]["AuthRedirect"];

/** LoginThrottled はログインの試行が多すぎた（429 login_throttled）ことを表す。 */
export class LoginThrottled extends RequestFailed {
  /** 再び試せるまでの秒数（Retry-After）。読めなければ null。 */
  readonly retryAfterSeconds: number | null;

  constructor(message: string, retryAfterSeconds: number | null) {
    super(429, "login_throttled", message);
    this.name = "LoginThrottled";
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

function postJSON(path: string, body: unknown): Promise<Response> {
  return fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

async function redirectFrom(response: Response): Promise<AuthRedirect> {
  if (!response.ok) throw await toRequestFailed(response);
  return (await response.json()) as AuthRedirect;
}

/**
 * getAuthSession は見る人の状態を返す。next を渡すと、所有者のときだけサーバーが
 * 確かめた戻り先（redirectTo）も返る。戻り先を画面で判定しないためである。
 */
export async function getAuthSession(
  next?: string,
  signal?: AbortSignal,
): Promise<AuthSession> {
  const query = next === undefined ? "" : `?${new URLSearchParams({ next })}`;
  const response = await fetch(`/api/auth/session${query}`, { signal });
  if (!response.ok) throw await toRequestFailed(response);
  return (await response.json()) as AuthSession;
}

/** setupAccount は最初のアカウントを作り、そのままログインする。 */
export async function setupAccount(
  username: string,
  password: string,
): Promise<AuthRedirect> {
  return redirectFrom(await postJSON("/api/auth/setup", { username, password }));
}

/** login はログインする。429 は LoginThrottled、それ以外の誤りは RequestFailed で投げる。 */
export async function login(
  username: string,
  password: string,
  next?: string,
): Promise<AuthRedirect> {
  const body = next === undefined ? { username, password } : { username, password, next };
  const response = await postJSON("/api/auth/login", body);
  if (response.status === 429) {
    const failed = await toRequestFailed(response);
    const retryAfter = Number.parseInt(response.headers.get("Retry-After") ?? "", 10);
    throw new LoginThrottled(
      failed.message,
      Number.isFinite(retryAfter) && retryAfter > 0 ? retryAfter : null,
    );
  }
  return redirectFrom(response);
}

/** logout はログアウトする。204 以外は RequestFailed で投げる。 */
export async function logout(): Promise<void> {
  const response = await fetch("/api/auth/logout", { method: "POST" });
  if (response.status !== 204) throw await toRequestFailed(response);
}
