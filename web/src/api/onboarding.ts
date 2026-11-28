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

/** Map the response's step to a known value; an unknown or missing one is `done`, so no screen is blocked. */
export function toOnboardingStep(value: unknown): OnboardingStep {
  return value === "folders" || value === "scan" || value === "interests"
    ? value
    : "done";
}

type Listener = (step: OnboardingStep) => void;

const listeners = new Set<Listener>();

/**
 * Receive the steps this tab writes; the returned function unsubscribes. The writing device routes by
 * the new step on the navigation right after the write, without waiting for the `onboarding` event.
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

/** Write the step (PUT /api/onboarding) and tell subscribers the written step; the later of two devices' writes wins. */
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
 * Turn the picked interests into tags and home shelves (POST /api/onboarding/interests); an interest
 * that already has a shelf gets no second one. `ids` holds 1 to 24 entries.
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
