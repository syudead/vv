import { useState } from "react";

import { startScan } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { useScan } from "../shell/ScanProvider";
import { StepsPage } from "../ui/patterns/steps-page";
import { Button } from "../ui/shadcn/button";
import { SkipButton, StepNotice, type StepProps } from "./stepFrame";

// オンボードの「Scan」の手順（docs/design-docs/home.md「Onboarding」）。「Scan library」は
// /api/scans で取り込みを始め、段階を `interests` にする。もう走っている取り込みは始めたものと
// 数える（開始の要求は走っているものをそのまま返す）。メディアフォルダが無ければ押せない。

const NO_FOLDER_ID = "onboarding-scan-no-folder";

export default function ScanStep({ frame, write }: StepProps) {
  const text = t.onboarding.scan;
  const scan = useScan();
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState<UiText | null>(null);
  const noFolder = scan.noMediaFolders && !scan.running;
  // フォルダの件数が分かるまで（読み込み中・読めなかった）も押せない。走っている取り込みは
  // 始めたものと数えるので押せる。
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
      // 進みの表示（右下の印と次の手順の行）は知らせを待たずに今の取り込みを読む。
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
