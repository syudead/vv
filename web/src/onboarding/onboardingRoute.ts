import type { OnboardingStep } from "../api/onboarding";

export const ONBOARDING_PATH = "/welcome";

/**
 * Where to replace `pathname`, already normalized by AuthGate's routePath, or null to stay
 * (docs/design-docs/home.md).
 *
 * - An owner whose step is not `done` goes from every route to `/welcome`.
 * - An owner whose step is `done` goes from `/welcome` to `/`.
 * - A viewer is never redirected; `/welcome` is owner-only and the gate sends a viewer to sign in.
 */
export function onboardingRedirect(
  owner: boolean,
  step: OnboardingStep,
  pathname: string,
): string | null {
  if (!owner) return null;
  const onWelcome = pathname === ONBOARDING_PATH;
  if (step === "done") return onWelcome ? "/" : null;
  return onWelcome ? null : ONBOARDING_PATH;
}
