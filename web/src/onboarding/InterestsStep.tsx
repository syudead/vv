import { Info } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import { getAutoTaggingSettings, type Scan } from "../api/client";
import { type Interest, listInterests, pickInterests } from "../api/onboarding";
import { errorText, t, type UiText } from "../i18n";
import { inProgress, useScan } from "../shell/ScanProvider";
import { ErrorState } from "../ui/patterns/error-state";
import { StepsPage } from "../ui/patterns/steps-page";
import { Alert, AlertDescription } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Progress } from "../ui/shadcn/progress";
import { Skeleton } from "../ui/shadcn/skeleton";
import { ToggleGroup, ToggleGroupItem } from "../ui/shadcn/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { useToast } from "../ui/Toast";
import { SkipButton, StepNotice, type StepProps } from "./stepFrame";

// オンボードの「Interests」の手順（docs/design-docs/home.md「Onboarding」）。上に取り込みの行、
// 自動タグ付けが切なら注意、その下に分野の ToggleGroup。ホームにもうある分野は押したまま押せ
// なくし、tooltip で理由を言う。「Finish」は選んだ分野を足してから段階を `done` にしてホームを
// 開く。足せなかったら選んだ分野を残す。Skip は何も足さずに `done` にする。

type InterestList =
  | { status: "loading" }
  | { status: "failed"; reason: UiText }
  | { status: "ready"; interests: Interest[] };

/** ScanLine は今の取り込みの行である。取り込みが無ければ何も描かない。 */
function ScanLine({ scan }: { scan: Scan | null }) {
  const text = t.onboarding.interests;
  if (scan === null) return null;
  if (inProgress(scan)) {
    const total = scan.videos?.total;
    const settled = scan.videos?.settled ?? 0;
    return (
      <div className="flex flex-col gap-2">
        <p role="status" className="text-sm tabular-nums">
          {text.scanning(settled, total ?? 0)}
        </p>
        <Progress
          aria-label={text.scanProgress}
          value={total === undefined ? null : settled}
          max={total === undefined || total === 0 ? undefined : total}
        />
      </div>
    );
  }
  if (scan.status === "failed") {
    return (
      <p role="status" className="text-sm text-destructive">
        {text.scanFailed}
      </p>
    );
  }
  return (
    <p role="status" className="text-sm text-muted-foreground">
      {text.scanFinished}
    </p>
  );
}

/** InterestChip は分野 1 つの切り替えである。ホームにもうある分野は押したまま押せない。 */
function InterestChip({ interest }: { interest: Interest }) {
  const item = (
    <ToggleGroupItem
      value={interest.id}
      disabled={interest.onHome}
      aria-label={
        interest.onHome ? t.onboarding.interests.onHomeLabel(interest.name) : undefined
      }
      title={interest.synonym}
      className="flex-none rounded-md px-3 data-[variant=outline]:border-l"
    >
      {interest.name}
    </ToggleGroupItem>
  );
  if (!interest.onHome) return item;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">{item}</span>
      </TooltipTrigger>
      <TooltipContent>{t.onboarding.interests.onHome}</TooltipContent>
    </Tooltip>
  );
}

/** useInterestsData は分野の一覧と、自動タグ付けが入かを読む。 */
function useInterestsData() {
  const [list, setList] = useState<InterestList>({ status: "loading" });
  // autoTagging は自動タグ付けが入か。読めなければ null で、注意を出さない。
  const [autoTagging, setAutoTagging] = useState<boolean | null>(null);

  const load = useCallback(async (signal?: AbortSignal) => {
    setList({ status: "loading" });
    try {
      setList({ status: "ready", interests: await listInterests(signal) });
    } catch (error) {
      if (signal?.aborted) return;
      setList({ status: "failed", reason: errorText(error) });
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    getAutoTaggingSettings(controller.signal).then(
      (settings) => setAutoTagging(settings.enabled),
      () => undefined,
    );
    return () => controller.abort();
  }, [load]);

  const replace = useCallback((interests: Interest[]) => {
    setList({ status: "ready", interests });
  }, []);

  return { list, autoTagging, reload: () => void load(), replace };
}

/**
 * madeAfterFailure は分野の追加が失敗したあと一覧を読み直し、選んだ分野のうちホームに
 * 載ったものを返す。棚を確定してから自動タグ付けへ入れるので、入れるのだけが失敗すれば
 * 棚は残っている（research.md R-6）。読み直せなければ null で、何も足せなかったと扱う。
 */
async function madeAfterFailure(
  picked: readonly string[],
): Promise<{ interests: Interest[]; made: string[] } | null> {
  try {
    const interests = await listInterests();
    const onHome = new Set(interests.filter((i) => i.onHome).map(({ id }) => id));
    return { interests, made: picked.filter((id) => onHome.has(id)) };
  } catch {
    return null;
  }
}

export default function InterestsStep({ frame, write }: StepProps) {
  const text = t.onboarding.interests;
  const { scan } = useScan();
  const toast = useToast();
  const { list, autoTagging, reload, replace } = useInterestsData();
  const [picked, setPicked] = useState<string[]>([]);
  const [picking, setPicking] = useState(false);
  const [pickError, setPickError] = useState<UiText | null>(null);

  const interests = list.status === "ready" ? list.interests : [];
  const onHome = interests.filter((interest) => interest.onHome).map(({ id }) => id);

  // 失敗のあと、棚ができていればそれを知らせ、自動タグ付けへ入れられなかったことだけを言う。
  // 手順には残り、ホームに載った分野は押したまま押せなくなる。
  const settleFailedPick = async (error: unknown) => {
    const reason = errorText(error);
    const after = await madeAfterFailure(picked);
    if (after === null || after.made.length === 0) {
      setPickError(text.pickFailed(reason));
      return;
    }
    replace(after.interests);
    toast(text.added(after.made.length));
    const left = picked.filter((id) => !after.made.includes(id));
    setPicked(left);
    setPickError(left.length > 0 ? text.pickFailed(reason) : text.queueFailed(reason));
  };

  const finish = async () => {
    setPickError(null);
    if (picked.length > 0) {
      setPicking(true);
      try {
        const result = await pickInterests(picked);
        if (result.shelves.length > 0) toast(text.added(result.shelves.length));
        setPicked([]);
      } catch (error) {
        await settleFailedPick(error);
        return;
      } finally {
        setPicking(false);
      }
    }
    await write.go("done");
  };

  const busy = picking || write.pending;
  return (
    <StepsPage
      {...frame}
      title={text.title}
      description={text.description}
      notice={<StepNotice message={pickError ?? write.error} />}
      skip={<SkipButton disabled={busy} onClick={() => void write.go("done")} />}
      action={
        <Button
          size="sm"
          disabled={busy || list.status !== "ready"}
          onClick={() => void finish()}
        >
          {text.finish}
        </Button>
      }
    >
      <ScanLine scan={scan} />
      {autoTagging === false && (
        <Alert>
          <Info aria-hidden="true" />
          <AlertDescription>{text.autoTaggingOff}</AlertDescription>
        </Alert>
      )}
      {list.status === "loading" && (
        <div role="status" aria-label={text.loading} className="flex flex-wrap gap-2">
          <Skeleton className="h-8 w-16" />
          <Skeleton className="h-8 w-16" />
          <Skeleton className="h-8 w-16" />
        </div>
      )}
      {list.status === "failed" && (
        <ErrorState
          title={text.loadFailed(list.reason)}
          retryLabel={t.common.retry}
          onRetry={reload}
        />
      )}
      {list.status === "ready" && (
        <ToggleGroup
          type="multiple"
          variant="outline"
          size="sm"
          aria-label={text.label}
          value={[...onHome, ...picked]}
          onValueChange={(values) =>
            setPicked(values.filter((id) => !onHome.includes(id)))
          }
          className="w-full flex-wrap gap-2"
        >
          {interests.map((interest) => (
            <InterestChip key={interest.id} interest={interest} />
          ))}
        </ToggleGroup>
      )}
    </StepsPage>
  );
}
