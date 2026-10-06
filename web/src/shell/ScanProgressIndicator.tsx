import { X } from "lucide-react";
import { useEffect, useRef, useState, type MouseEvent } from "react";
import { useNavigate } from "react-router";

import { t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/shadcn/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { useScanNotice } from "./ScanNoticeProvider";
import ScanProgressBar from "./ScanProgressBar";
import { useScan } from "./ScanProvider";
import { ScanDetail, ScanIssueCounts, ScanStatusIcon } from "./ScanSummaryParts";
import {
  inProgressState,
  issueCountTexts,
  presentScan,
  statusAnnouncement,
  type ScanPresentation,
} from "./scanPresentation";

const POINTER_CLOSE_DELAY_MS = 100;

/** indicatorName は右下の本体の名前（「Open the scan status」の前）である。 */
function indicatorName(presentation: ScanPresentation): UiText {
  const text = t.shell.scan;
  const issues = issueCountTexts(presentation);
  return text.indicatorName(
    presentation.statusText ?? text.status.starting,
    presentation.videos === null
      ? null
      : text.videosDone(presentation.videos.settled, presentation.videos.total),
    issues.length === 0 ? null : text.issueCounts(issues),
  );
}

/**
 * ScanProgressIndicator は右下の本体と概要である（specs/024-import-progress/ui-design.md
 * 「Floating Indicator」「Summary Popover」）。置き場所・開き方・押したときの移動先は
 * 012 の形のまま。
 *
 * 概要はホバーかフォーカスで開き、ポインタが離れる・Esc・外を押す・設定へ移る・フォーカスが
 * 外れる、のいずれでも閉じる。開いているかは、開く操作と閉じる操作の出来事だけで決める。
 * 「ホバーかフォーカスの間は開く」と状態から決めると、閉じたときにフォーカスが本体へ戻って
 * すぐ開き直る（issue 830）。閉じたあとに本体へ残ったフォーカスでは開き直らない。
 *
 * 本体はどの状態でも閉じる button で閉じられ、閉じた取り込みは終わるまでも、結果の通知と
 * しても出さない。次の取り込みでは、また出す。
 */
export default function ScanProgressIndicator() {
  const scan = useScan();
  const notice = useScanNotice();
  const { setCompletionNoticePaused } = notice;
  const navigate = useNavigate();
  const presentation = presentScan(scan);
  const [open, setOpen] = useState(false);
  const pointerCloseTimer = useRef<number | null>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const pointerInside = useRef(false);
  // 本体にフォーカスがあるまま閉じたら、フォーカスが外れるまではフォーカスで開き直らない。
  const focusOpenSuppressed = useRef(false);

  const terminalVisible =
    presentation.scan !== null &&
    notice.completionNotice?.scanId === presentation.scan.id;
  // 開始の途中は、まだ前の取り込みを指している。閉じたのが前の取り込みでも出す。
  const dismissed =
    notice.dismissPending ||
    (presentation.state !== "starting" &&
      presentation.scan !== null &&
      presentation.scan.id === notice.dismissedScanId);
  const visible = !dismissed && (inProgressState(presentation.state) || terminalVisible);
  // 一部失敗と失敗は、閉じるか設定へ移るまで残す。
  const persistent = presentation.state === "partial" || presentation.state === "failed";

  const clearPointerCloseTimer = () => {
    if (pointerCloseTimer.current !== null) {
      window.clearTimeout(pointerCloseTimer.current);
      pointerCloseTimer.current = null;
    }
  };

  useEffect(() => {
    if (visible) return;
    if (pointerCloseTimer.current !== null) {
      window.clearTimeout(pointerCloseTimer.current);
      pointerCloseTimer.current = null;
    }
    pointerInside.current = false;
    focusOpenSuppressed.current = false;
    setOpen(false);
    if (scan.loaded) setCompletionNoticePaused(false);
  }, [scan.loaded, setCompletionNoticePaused, visible]);

  useEffect(
    () => () => {
      if (pointerCloseTimer.current !== null) {
        window.clearTimeout(pointerCloseTimer.current);
      }
    },
    [],
  );

  // 完了の通知は、概要が開いている間だけ止める。閉じれば通常どおり時間がたてば消える。
  useEffect(() => {
    if (!scan.loaded) return;
    setCompletionNoticePaused(terminalVisible && open);
  }, [open, scan.loaded, setCompletionNoticePaused, terminalVisible]);

  if (!visible) return null;

  const close = () => {
    clearPointerCloseTimer();
    focusOpenSuppressed.current = document.activeElement === trigger.current;
    setOpen(false);
  };
  const goToDetails = (event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    close();
    if (persistent) notice.acknowledgeTerminalScan();
    navigate("/settings#scan-status");
  };
  const enterPointerArea = () => {
    clearPointerCloseTimer();
    pointerInside.current = true;
    setOpen(true);
  };
  const leavePointerArea = () => {
    clearPointerCloseTimer();
    pointerCloseTimer.current = window.setTimeout(() => {
      pointerInside.current = false;
      close();
    }, POINTER_CLOSE_DELAY_MS);
  };
  const focusTrigger = () => {
    if (!focusOpenSuppressed.current) setOpen(true);
  };
  const blurTrigger = () => {
    focusOpenSuppressed.current = false;
    // ポインタが本体か概要の上にある間は、ポインタの方で閉じる。
    if (!pointerInside.current) setOpen(false);
  };
  const dismissIndicator = () => {
    close();
    notice.dismissIndicator();
  };

  return (
    // 再生画面でも右下に置く。右上には見出しの帯の閉じる × があり、そこを覆ってしまう。
    // 狭い幅でも左右に 0.75rem を残す（max-w-viewport-inset）。
    <div
      className="fixed right-3 bottom-4 z-30 max-w-viewport-inset sm:right-5 sm:bottom-5"
      onPointerEnter={enterPointerArea}
      onPointerLeave={leavePointerArea}
    >
      <span role="status" aria-atomic="true" className="sr-only">
        {statusAnnouncement(presentation)}
      </span>
      <Popover open={open} onOpenChange={(next) => (next ? setOpen(true) : close())}>
        <div className="flex items-center gap-1">
          <PopoverTrigger asChild>
            <Button
              ref={trigger}
              variant="outline"
              onClick={goToDetails}
              onFocus={focusTrigger}
              onBlur={blurTrigger}
              aria-label={t.shell.scan.openStatus(indicatorName(presentation))}
              className="max-w-full bg-popover font-normal shadow-elevated"
            >
              <ScanStatusIcon state={presentation.state} />
              <span className="font-medium">{presentation.statusText}</span>
              {presentation.videos !== null && (
                <span className="tabular-nums">
                  {t.shell.scan.videosShort(
                    presentation.videos.settled,
                    presentation.videos.total,
                  )}
                </span>
              )}
              <ScanIssueCounts
                failed={presentation.issues.failed}
                substituted={presentation.issues.substituted}
                mostSevereOnly
                compact
              />
            </Button>
          </PopoverTrigger>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="icon-sm"
                aria-label={t.shell.scan.dismiss}
                className="bg-popover shadow-elevated"
                onClick={dismissIndicator}
              >
                <X aria-hidden="true" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t.shell.scan.dismiss}</TooltipContent>
          </Tooltip>
        </div>
        <PopoverContent
          align="end"
          onOpenAutoFocus={(event) => event.preventDefault()}
          // 閉じたときにフォーカスを本体へ戻さない。戻すと、ホバーで開いていただけの概要が
          // 本体のフォーカスを残し、閉じたことが分かりにくい（issue 830）。
          onCloseAutoFocus={(event) => event.preventDefault()}
          onPointerEnter={enterPointerArea}
          onPointerLeave={leavePointerArea}
        >
          <div className="space-y-2">
            <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
              <span className="inline-flex items-center gap-2 whitespace-nowrap font-semibold">
                <ScanStatusIcon state={presentation.state} />
                {presentation.statusText}
              </span>
              <ScanIssueCounts
                failed={presentation.issues.failed}
                substituted={presentation.issues.substituted}
                className="text-sm"
              />
            </div>
            <ScanProgressBar presentation={presentation} />
            {presentation.progressText !== null && (
              <p className="text-sm tabular-nums">{presentation.progressText}</p>
            )}
            <ScanDetail presentation={presentation} />
            {(presentation.state === "partial" ||
              (presentation.state === "done" && presentation.issues.substituted > 0)) && (
              <p className="text-xs text-muted-foreground">
                {t.shell.scan.seeSettingsForList}
              </p>
            )}
          </div>
        </PopoverContent>
      </Popover>
    </div>
  );
}
