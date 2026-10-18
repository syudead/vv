import { useState } from "react";

import { startScan } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { useScan } from "../shell/ScanProvider";
import { StepsPage } from "../ui/patterns/steps-page";
import { Button } from "../ui/shadcn/button";
import { SkipButton, StepNotice, type StepProps } from "./stepFrame";

const NO_FOLDER_ID = "onboarding-scan-no-folder";

export default function ScanStep({ frame, write }: StepProps) {
  const text = t.onboarding.scan;
  const scan = useScan();
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState<UiText | null>(null);
  const noFolder = scan.noMediaFolders && !scan.running;

  const startable = scan.running || scan.canStart;

  const start = async () => {
    setStarting(true);
    setStartError(null);
    try {
      await startScan();
    } catch (error) {
      setStartError(t.shell.scan.startFailed(errorText(error)));
      return;
    } finally {
      setStarting(false);

      scan.refresh();
    }
    await write.go("interests");
  };

  const busy = starting || write.pending;
  return (
    <StepsPage
      {...frame}
      title={text.title}
      description={text.description}
      notice={<StepNotice message={startError ?? write.error} />}
      skip={<SkipButton disabled={busy} onClick={() => void write.go("interests")} />}
      action={
        <Button
          size="sm"
          disabled={busy || !startable}
          aria-describedby={noFolder ? NO_FOLDER_ID : undefined}
          onClick={() => void start()}
        >
          {text.start}
        </Button>
      }
    >
      {noFolder && (
        <p id={NO_FOLDER_ID} className="text-xs text-muted-foreground">
          {text.noFolder}
        </p>
      )}
    </StepsPage>
  );
}
