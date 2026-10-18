import type { OnboardingStep } from "../api/onboarding";

export const ONBOARDING_PATH = "/welcome";

/**
 * onboardingRedirect は、見る人が `pathname` を開いたときに置き換える先を返す。置き換えなければ
 * null（docs/design-docs/home.md）。
 *
 * - 所有者で段階が `done` でなければ、どの経路も `/welcome` へ置き換える。
 * - 所有者で段階が `done` なら、`/welcome` は `/` へ置き換える。
 * - 閲覧者は送らない（`/welcome` は所有者だけの画面で、ゲートがログインへ送る）。
 *
 * `pathname` は AuthGate の routePath で揃えたものである。
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
