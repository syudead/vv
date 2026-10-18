import { AlertCircle } from "lucide-react";

import { ONBOARDING_STEPS, type OnboardingStep } from "../api/onboarding";
import { t, type UiText } from "../i18n";
import type { StepsPageStep } from "../ui/patterns/steps-page";
import { Alert, AlertDescription } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";

/** StepFrame は StepsPage の、手順の帯に渡す値である。 */
export interface StepFrame {
  steps: readonly StepsPageStep[];
  current: number;
  stepsLabel: string;
  progress: UiText;
}

/** stepFrame は今の手順 `step` の帯の値を作る。 */
export function stepFrame(step: OnboardingStep): StepFrame {
  const text = t.onboarding;
  const found = ONBOARDING_STEPS.indexOf(step as (typeof ONBOARDING_STEPS)[number]);
  const current = Math.max(found, 0);
  return {
    steps: ONBOARDING_STEPS.map((key, index) => {
      const name = text.stepNames[key];
      let label: string = name;
      if (index < current) label = text.stepDone(name);
      else if (index === current) label = text.stepCurrent(name);
      return { key, name, label };
    }),
    current,
    stepsLabel: text.steps,
    progress: text.stepOf(current + 1, ONBOARDING_STEPS.length),
  };
}

/**
 * StepWrite は段階の書き込みである。`pending` の間は下端のボタンを押せなくする。`go` は書けたら
 * true を返し、書けなければ `error` に理由を置いて false を返す。
 */
export interface StepWrite {
  pending: boolean;
  error: UiText | null;
  go: (next: OnboardingStep) => Promise<boolean>;
}

/** StepProps は手順 1 つの部品が受け取る値である。 */
export interface StepProps {
  frame: StepFrame;
  write: StepWrite;
}

/** SkipButton は下端の左の「Skip」である。 */
export function SkipButton({
  disabled,
  onClick,
}: {
  disabled: boolean;
  onClick: () => void;
}) {
  return (
    <Button variant="ghost" size="sm" disabled={disabled} onClick={onClick}>
      {t.onboarding.skip}
    </Button>
  );
}

/** StepNotice は下端の行の上の失敗の Alert である。理由が無ければ何も描かない。 */
export function StepNotice({ message }: { message: UiText | null }) {
  if (message === null) return null;
  return (
    <Alert variant="destructive">
      <AlertCircle aria-hidden="true" />
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  );
}
