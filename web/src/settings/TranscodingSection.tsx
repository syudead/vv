import { CircleAlert, ExternalLink, LoaderCircle, TriangleAlert } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import {
  getTranscodingSettings,
  RequestFailed,
  updateTranscodingSettings,
  type EncoderAvailability,
  type TranscodingSettings,
  type VideoEncoderChoice,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { FactList } from "../ui/patterns/fact-list";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Label } from "../ui/shadcn/label";
import { RadioGroup, RadioGroupItem } from "../ui/shadcn/radio-group";
import { Skeleton } from "../ui/shadcn/skeleton";
import { HARDWARE_ENCODING_GUIDE_URL } from "./docsLinks";

/** 確認中の間に `GET` し直す間隔である（research.md R-5）。 */
export const CHECKING_POLL_INTERVAL_MS = 3000;

/** choiceOrder は選択肢の並びである（親 Issue 要件 1）。 */
const choiceOrder: readonly VideoEncoderChoice[] = [
  "software",
  "nvenc",
  "qsv",
  "vaapi",
  "videotoolbox",
  "auto",
];

interface ChoiceState {
  enabled: boolean;
  status: UiText;
}

function hardwareState(availability: EncoderAvailability | undefined): ChoiceState {
  const text = t.settings.transcoding;
  if (availability?.state === "available") {
    return { enabled: true, status: text.state.available };
  }
  if (availability?.state === "checking") {
    return { enabled: false, status: text.state.checking };
  }
  return {
    enabled: false,
    status: text.reason[availability?.reason ?? "encoder_missing"],
  };
}

function choiceState(
  choice: VideoEncoderChoice,
  settings: TranscodingSettings,
): ChoiceState {
  const text = t.settings.transcoding;
  if (choice === "software") return { enabled: true, status: text.state.alwaysAvailable };
  if (choice === "auto") return { enabled: true, status: text.state.auto };
  return hardwareState(settings.encoders.find((item) => item.encoder === choice));
}

/**
 * TranscodingSection は設定画面の「動画の変換」区画である。所有者がライブ変換の映像
 * エンコード方式を選び、どれが使えて今どれが効いているかを見る。選んだ時点で保存し、
 * 表示は表示時の `GET` と `PUT` の応答で置き換える。確認中の間だけ数秒ごとに `GET` し直す
 * （specs/025-hardware-encoding/research.md R-5）。
 */
export default function TranscodingSection({
  pollIntervalMs = CHECKING_POLL_INTERVAL_MS,
}: {
  pollIntervalMs?: number;
}) {
  const text = t.settings.transcoding;
  const [settings, setSettings] = useState<TranscodingSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [saving, setSaving] = useState<VideoEncoderChoice | null>(null);
  const [saveError, setSaveError] = useState<UiText | null>(null);
  const [pollTick, setPollTick] = useState(0);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    setLoadError(null);
    try {
      setSettings(await getTranscodingSettings(signal));
    } catch (failure) {
      if (signal?.aborted) return;
      setLoadError(errorText(failure));
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  // 確認中の間は、終わるまで読み直す。新しい応答（読み直しでも保存でも）で settings が
  // 変わると、前の読み直しは取り消す。読み直しの失敗は表示を変えずに次の回へ回す。
  const checking = settings?.checking === true;
  useEffect(() => {
    if (!checking) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      getTranscodingSettings(controller.signal).then(
        (next) => setSettings(next),
        () => {
          if (!controller.signal.aborted) setPollTick((tick) => tick + 1);
        },
      );
    }, pollIntervalMs);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [checking, settings, pollTick, pollIntervalMs]);

  const select = async (choice: VideoEncoderChoice) => {
    if (settings === null || saving !== null || choice === settings.videoEncoder) return;
    setSaveError(null);
    setSaving(choice);
    try {
      setSettings(await updateTranscodingSettings(choice));
    } catch (failure) {
      setSaveError(text.saveFailed(errorText(failure)));
      // 確認の結果が画面の表示より新しいときは、使える方式の表示を合わせ直す。読み直しが
      // 終わるまで選択肢は操作できないままにし、あとの保存の応答を古い応答で上書きしない。
      if (failure instanceof RequestFailed && failure.reason === "encoder_unavailable") {
        try {
          setSettings(await getTranscodingSettings());
        } catch {
          // 読み直しの失敗は表示を変えない。保存の失敗はすでに出している。
        }
      }
    } finally {
      setSaving(null);
    }
  };

  const selected = saving ?? settings?.videoEncoder;

  return (
    <PageSection
      title={text.heading}
      description={
        <>
          {text.description}{" "}
          <a
            href={HARDWARE_ENCODING_GUIDE_URL}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-primary underline underline-offset-4"
          >
            {text.guide}
            <ExternalLink aria-hidden="true" className="size-3" />
            <span className="sr-only">{text.opensInNewTab}</span>
          </a>
        </>
      }
    >
      {loading && settings === null && (
        <div role="status" aria-label={text.loading} className="flex flex-col gap-3">
          <Skeleton className="h-5 w-1/3" />
          <Skeleton className="h-8" />
          <Skeleton className="h-8" />
        </div>
      )}
      {!loading && loadError !== null && settings === null && (
        <div>
          <ErrorState
            title={text.loadFailed(loadError)}
            retryLabel={t.common.retry}
            onRetry={() => void load()}
          />
        </div>
      )}
      {settings !== null && (
        <div className="flex flex-col gap-2">
          <FactList
            facts={[
              {
                id: "in-use",
                term: text.inUse,
                value: (
                  <span className="font-medium">
                    {text.encoder[settings.effectiveEncoder]}
                  </span>
                ),
              },
            ]}
          />
          {settings.checking && (
            <p role="status" className="flex items-center gap-2 text-muted-foreground">
              <LoaderCircle
                aria-hidden="true"
                className="size-4 animate-spin motion-reduce:animate-none"
              />
              {text.checking}
            </p>
          )}
          {settings.fallbackReason === "selected_unavailable" && (
            <Alert variant="warning" role="note">
              <TriangleAlert aria-hidden="true" />
              <AlertTitle className="font-normal">{text.selectedUnavailable}</AlertTitle>
            </Alert>
          )}
        </div>
      )}
      {settings !== null && (
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-3">
            <span id="transcoding-choices-label" className="font-medium">
              {text.choices}
            </span>
            {saving !== null && (
              <span role="status" className="text-xs text-muted-foreground">
                {text.saving}
              </span>
            )}
          </div>
          <RadioGroup
            name="video-encoder"
            value={selected}
            onValueChange={(value) => void select(value as VideoEncoderChoice)}
            aria-labelledby="transcoding-choices-label"
          >
            {choiceOrder.map((choice) => {
              const state = choiceState(choice, settings);
              const disabled = !state.enabled || saving !== null;
              const itemId = `transcoding-choice-${choice}`;
              const nameId = `transcoding-name-${choice}`;
              const statusId = `transcoding-status-${choice}`;
              return (
                <div key={choice} className="flex min-w-0 items-start gap-3">
                  <RadioGroupItem
                    id={itemId}
                    value={choice}
                    disabled={disabled}
                    aria-labelledby={nameId}
                    aria-describedby={statusId}
                    className="mt-0.5"
                  />
                  <Label
                    htmlFor={itemId}
                    className="flex min-w-0 flex-1 flex-col items-start gap-0.5 font-normal sm:flex-row sm:items-baseline sm:justify-between sm:gap-4"
                  >
                    <span
                      id={nameId}
                      className={
                        state.enabled ? "text-foreground" : "text-muted-foreground"
                      }
                    >
                      {text.encoder[choice]}
                    </span>
                    <span id={statusId} className="text-muted-foreground sm:text-right">
                      {state.status}
                    </span>
                  </Label>
                </div>
              );
            })}
          </RadioGroup>
        </div>
      )}
      {saveError !== null && (
        <div>
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{saveError}</AlertTitle>
          </Alert>
        </div>
      )}
    </PageSection>
  );
}
