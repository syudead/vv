import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router";

import { type OnboardingStep, writeOnboarding } from "../api/onboarding";
import { errorText, t, type UiText } from "../i18n";
import { useDocumentTitle } from "../videoList/pageHooks";
import FoldersStep from "./FoldersStep";
import InterestsStep from "./InterestsStep";
import { useOnboardingStep } from "./OnboardingGate";
import ScanStep from "./ScanStep";
import { stepFrame, type StepWrite } from "./stepFrame";

/** useStepWrite は段階の書き込みと、その途中か、失敗の理由を持つ。 */
function useStepWrite(step: OnboardingStep): StepWrite {
  const navigate = useNavigate();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);

  useEffect(() => setError(null), [step]);

  const go = useCallback(
    async (next: OnboardingStep) => {
      setPending(true);
      setError(null);
      try {
        await writeOnboarding(next);
      } catch (failure) {
        setError(t.onboarding.saveFailed(errorText(failure)));
        return false;
      } finally {
        setPending(false);
      }
      if (next === "done") void navigate("/", { replace: true });
      return true;
    },
    [navigate],
  );

  return { pending, error, go };
}

export default function OnboardingPage() {
  const step = useOnboardingStep();
  const write = useStepWrite(step);
  const frame = stepFrame(step);
  useDocumentTitle(t.onboarding.documentTitle);

  if (step === "scan") return <ScanStep frame={frame} write={write} />;
  if (step === "interests") return <InterestsStep frame={frame} write={write} />;
  return <FoldersStep frame={frame} write={write} />;
}
