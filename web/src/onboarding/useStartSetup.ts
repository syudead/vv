import { useCallback, useState } from "react";
import { useNavigate } from "react-router";

import { writeOnboarding } from "../api/onboarding";
import { errorText, t } from "../i18n";
import { useToast } from "../ui/Toast";
import { ONBOARDING_PATH } from "./onboardingRoute";

/**
 * Restart onboarding: write the step `folders`, then open `/welcome` (docs/design-docs/home.md, Sidebar
 * and library). Without the write, the gate would send an owner whose step is `done` back home; a
 * failed write shows its reason in a notice and stays.
 */
export function useStartSetup() {
  const navigate = useNavigate();
  const toast = useToast();
  const [pending, setPending] = useState(false);
  const start = useCallback(async () => {
    setPending(true);
    try {
      await writeOnboarding("folders");
      void navigate(ONBOARDING_PATH);
    } catch (error) {
      toast(t.onboarding.saveFailed(errorText(error)));
    } finally {
      setPending(false);
    }
  }, [navigate, toast]);
  return { start, pending };
}
