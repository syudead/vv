import { request } from "./client";
import type { components } from "./gen/openapi";

export type OnboardingStep = components["schemas"]["Onboarding"]["step"];
export type Interest = components["schemas"]["Interest"];
export type InterestPickResult = components["schemas"]["InterestPickResult"];

export const ONBOARDING_STEPS = ["folders", "scan", "interests"] as const;

export function nextOnboardingStep(step: OnboardingStep): OnboardingStep {
  const index = ONBOARDING_STEPS.indexOf(step as (typeof ONBOARDING_STEPS)[number]);
  if (index < 0) return "done";
  return ONBOARDING_STEPS[index + 1] ?? "done";
}

/**
 * toOnboardingStep は応答の段階を、知っている値に揃える。知らない値と欠けた値は `done` と
 * 扱い、どの画面も止めない。
 */
export function toOnboardingStep(value: unknown): OnboardingStep {
  return value === "folders" || value === "scan" || value === "interests"
    ? value
    : "done";
}

type Listener = (step: OnboardingStep) => void;

const listeners = new Set<Listener>();

/**
 * subscribeOnboarding は、この画面から書いた段階を受け取る。戻り値で購読をやめる。
 * 書いた端末では `onboarding` の知らせを待たずに、書いた直後の遷移から新しい段階で振り分ける。
 */
export function subscribeOnboarding(listener: Listener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export async function readOnboarding(signal?: AbortSignal): Promise<OnboardingStep> {
  const body = await request<Partial<components["schemas"]["Onboarding"]>>(
    "/api/onboarding",
    { signal },
  );
  return toOnboardingStep(body.step);
}

/**
 * writeOnboarding は段階を書き（PUT /api/onboarding）、書けた段階を購読者へ知らせる。
 * 2 つの端末から書けば、後に書いた方が残る。
 */
export async function writeOnboarding(
  step: OnboardingStep,
  signal?: AbortSignal,
): Promise<OnboardingStep> {
  const body = await request<Partial<components["schemas"]["Onboarding"]>>(
    "/api/onboarding",
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ step }),
      signal,
    },
  );
  const written = toOnboardingStep(body.step ?? step);
  for (const listener of [...listeners]) listener(written);
  return written;
}

export async function listInterests(signal?: AbortSignal): Promise<Interest[]> {
  const body = await request<components["schemas"]["InterestList"]>(
    "/api/onboarding/interests",
    { signal },
  );
  return body.interests;
}

/**
 * pickInterests は選んだ分野をタグと棚にする（POST /api/onboarding/interests）。
 * 棚がもうある分野は棚を足さない（research.md R-6）。`ids` は 1〜24 件。
 */
export function pickInterests(
  ids: readonly string[],
  signal?: AbortSignal,
): Promise<InterestPickResult> {
  return request<InterestPickResult>("/api/onboarding/interests", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ids }),
    signal,
  });
}
