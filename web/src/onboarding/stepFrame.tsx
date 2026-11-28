import { AlertCircle } from "lucide-react";

import { ONBOARDING_STEPS, type OnboardingStep } from "../api/onboarding";
import { t, type UiText } from "../i18n";
import type { StepsPageStep } from "../ui/patterns/steps-page";
import { Alert, AlertDescription } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";

/** The steps strip's values for StepsPage. */
export interface StepFrame {
  steps: readonly StepsPageStep[];
  current: number;
  stepsLabel: string;
  progress: UiText;
}

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
 * The step write: the footer buttons are disabled while `pending`; `go` returns true once written, or
 * sets `error` to the reason and returns false.
 */
export interface StepWrite {
  pending: boolean;
  error: UiText | null;
  go: (next: OnboardingStep) => Promise<boolean>;
}

export interface StepProps {
  frame: StepFrame;
  write: StepWrite;
}

/** The footer's `Skip`, at its left. */
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

/** The failure Alert above the footer; nothing without a reason. */
export function StepNotice({ message }: { message: UiText | null }) {
  if (message === null) return null;
  return (
    <Alert variant="destructive">
      <AlertCircle aria-hidden="true" />
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  );
}
