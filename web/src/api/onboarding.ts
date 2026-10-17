import { request } from "./client";
import type { components } from "./gen/openapi";

// オンボードの画面 API（specs/057-home/contracts/screen-api.md）。段階は設定 1 つで持ち、
// 書くと画面の知らせ `home` が出る（research.md R-3、R-16）。所有者だけ。

export type Onboarding = components["schemas"]["Onboarding"];
export type OnboardingStep = Onboarding["step"];

/** ONBOARDING_PATH はオンボードの画面の場所である（specs/057-home/ui-design.md「Onboarding」）。 */
export const ONBOARDING_PATH = "/welcome";

/** writeOnboardingStep はオンボードの段階を書き、書いた段階を返す（PUT /api/onboarding）。 */
export function writeOnboardingStep(
  step: OnboardingStep,
  signal?: AbortSignal,
): Promise<Onboarding> {
  return request<Onboarding>("/api/onboarding", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ step }),
    signal,
  });
}
