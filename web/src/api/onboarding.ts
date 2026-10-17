import { request } from "./client";
import type { components } from "./gen/openapi";

// オンボードの画面 API（specs/057-home/contracts/screen-api.md）。型は api/openapi.yaml からの
// 生成物を使う。段階は設定に 1 つで持ち、どの端末からも同じものを読む（research.md R-3）。
// どれも所有者だけ。

export type OnboardingStep = components["schemas"]["Onboarding"]["step"];
export type Interest = components["schemas"]["Interest"];
export type InterestPickResult = components["schemas"]["InterestPickResult"];

/** ONBOARDING_STEPS は手順の段階を進む順に並べたものである。`done` は含めない。 */
export const ONBOARDING_STEPS = ["folders", "scan", "interests"] as const;

/** nextOnboardingStep は `step` の次の段階を返す。最後の手順の次は `done` である。 */
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
 * 書いた端末では `home` の知らせを待たずに、書いた直後の遷移から新しい段階で振り分ける。
 */
export function subscribeOnboarding(listener: Listener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** readOnboarding は今の段階を読む（GET /api/onboarding）。 */
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

/** listInterests は分野の一覧を一覧の順に読む（GET /api/onboarding/interests）。 */
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
