import { useId, useState } from "react";
import { useNavigate } from "react-router";

import { writeOnboarding } from "../api/onboarding";
import { errorText, t } from "../i18n";
import { ONBOARDING_PATH } from "../onboarding/onboardingRoute";
import { FormRow } from "../ui/patterns/form-row";
import { PageSection } from "../ui/patterns/page-section";
import { Button } from "../ui/shadcn/button";
import { Spinner } from "../ui/shadcn/spinner";
import { useToast } from "../ui/Toast";

export default function SetupSection() {
  const text = t.settings.setup;
  const toast = useToast();
  const navigate = useNavigate();
  const setupId = useId();
  const [startingSetup, setStartingSetup] = useState(false);

  const startSetup = async () => {
    setStartingSetup(true);
    try {
      await writeOnboarding("folders");
      void navigate(ONBOARDING_PATH);
    } catch (error) {
      toast(text.setupFailed(errorText(error)));
      setStartingSetup(false);
    }
  };

  return (
    <PageSection title={text.heading}>
      <FormRow label={text.setup} htmlFor={setupId} description={text.setupDescription}>
        <Button
          id={setupId}
          aria-label={text.startSetup}
          variant="outline"
          size="sm"
          disabled={startingSetup}
          onClick={() => void startSetup()}
        >
          {startingSetup && <Spinner aria-hidden="true" role="presentation" />}
          {text.startSetup}
        </Button>
      </FormRow>
    </PageSection>
  );
}
