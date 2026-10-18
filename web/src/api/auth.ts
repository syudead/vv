import { RequestFailed, sendRequest, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";

// Bypass client audience reload handling so authentication screens can handle their own errors.

export type AuthSession = components["schemas"]["AuthSession"];
export type AuthState = AuthSession["state"];
export type AuthRedirect = components["schemas"]["AuthRedirect"];

export class LoginThrottled extends RequestFailed {
  /** Retry-After seconds, or null if unavailable. */
  readonly retryAfterSeconds: number | null;

  constructor(message: string, retryAfterSeconds: number | null) {
    super(429, "login_throttled", message);
    this.name = "LoginThrottled";
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

function postJSON(path: string, body: unknown): Promise<Response> {
  return sendRequest(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

async function redirectFrom(response: Response): Promise<AuthRedirect> {
  if (!response.ok) throw await toRequestFailed(response);
  return (await response.json()) as AuthRedirect;
}

/** Owners receive a server-validated redirectTo for next; clients must not validate redirects independently. */
export async function getAuthSession(
  next?: string,
  signal?: AbortSignal,
): Promise<AuthSession> {
  const query = next === undefined ? "" : `?${new URLSearchParams({ next })}`;
  const response = await sendRequest(`/api/auth/session${query}`, { signal });
  if (!response.ok) throw await toRequestFailed(response);
  return (await response.json()) as AuthSession;
}

export async function setupAccount(
  username: string,
  password: string,
): Promise<AuthRedirect> {
  return redirectFrom(await postJSON("/api/auth/setup", { username, password }));
}

/** Throw LoginThrottled for 429 and RequestFailed for other failures. */
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

export async function logout(): Promise<void> {
  const response = await sendRequest("/api/auth/logout", { method: "POST" });
  if (response.status !== 204) throw await toRequestFailed(response);
}
