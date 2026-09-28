import { AlertTriangle, ExternalLink, LoaderCircle } from "lucide-react";
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
import Button from "../ui/Button";
import Skeleton from "../ui/Skeleton";
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
      // 確認の結果が画面の表示より新しいときは、使える方式の表示を合わせ直す。
      if (failure instanceof RequestFailed && failure.reason === "encoder_unavailable") {
        getTranscodingSettings().then(setSettings, () => undefined);
      }
    } finally {
      setSaving(null);
    }
  };

  const selected = saving ?? settings?.videoEncoder;

  return (
    <section
      aria-labelledby="transcoding-heading"
      className="mt-8 rounded-lg border border-border bg-surface p-4 sm:p-5"
    >
      <div className="border-b border-border pb-4">
        <h2 id="transcoding-heading" className="text-base font-semibold">
          {text.heading}
        </h2>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-fg-muted">
          {text.description}{" "}
          <a
            href={HARDWARE_ENCODING_GUIDE_URL}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-link underline underline-offset-4"
          >
            {text.guide}
            <ExternalLink aria-hidden="true" className="size-3.5" />
            <span className="sr-only">{text.opensInNewTab}</span>
          </a>
        </p>
      </div>

      {loading && settings === null && (
        <div role="status" aria-label={text.loading} className="space-y-3 py-5">
          <Skeleton className="h-6 w-48" />
          <Skeleton className="h-12" />
          <Skeleton className="h-12" />
        </div>
      )}
      {!loading && loadError !== null && settings === null && (
        <div className="flex flex-col items-start gap-3 py-6">
          <p role="alert" className="text-sm text-danger">
            {text.loadFailed(loadError)}
          </p>
          <Button onClick={() => void load()}>{t.common.retry}</Button>
        </div>
      )}
      {settings !== null && (
        <div className="pt-4">
          <p className="flex flex-wrap items-baseline gap-x-2">
            <span className="text-sm text-fg-muted">{text.inUse}</span>
            <span className="font-medium text-fg">
              {text.encoder[settings.effectiveEncoder]}
            </span>
          </p>
          {settings.checking && (
            <p
              role="status"
              className="mt-2 flex items-center gap-2 text-sm text-fg-muted"
            >
              <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />
              {text.checking}
            </p>
          )}
          {settings.fallbackReason === "selected_unavailable" && (
            <p className="mt-2 flex items-start gap-2 text-sm text-warning">
              <AlertTriangle aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
              {text.selectedUnavailable}
            </p>
          )}

          <div className="mt-4 flex flex-wrap items-center gap-3">
            <span id="transcoding-choices-label" className="text-sm text-fg-muted">
              {text.choices}
            </span>
            {saving !== null && (
              <span role="status" className="text-xs text-fg-muted">
                {text.saving}
              </span>
            )}
          </div>
          <div
            role="radiogroup"
            aria-labelledby="transcoding-choices-label"
            className="mt-1 divide-y divide-border"
          >
            {choiceOrder.map((choice) => {
              const state = choiceState(choice, settings);
              const disabled = !state.enabled || saving !== null;
              const nameId = `transcoding-name-${choice}`;
              const statusId = `transcoding-status-${choice}`;
              return (
                <label
                  key={choice}
                  className={
                    disabled
                      ? "flex min-w-0 items-start gap-3 py-3"
                      : "flex min-w-0 cursor-pointer items-start gap-3 py-3"
                  }
                >
                  <input
                    type="radio"
                    name="video-encoder"
                    value={choice}
                    checked={selected === choice}
                    disabled={disabled}
                    aria-labelledby={nameId}
                    aria-describedby={statusId}
                    onChange={() => void select(choice)}
                    className="mt-1 size-4 shrink-0 accent-accent"
                  />
                  <span className="flex min-w-0 flex-1 flex-col gap-0.5 sm:flex-row sm:items-baseline sm:justify-between sm:gap-4">
                    <span
                      id={nameId}
                      className={state.enabled ? "text-fg" : "text-fg-muted"}
                    >
                      {text.encoder[choice]}
                    </span>
                    <span id={statusId} className="text-sm text-fg-muted sm:text-right">
                      {state.status}
                    </span>
                  </span>
                </label>
              );
            })}
          </div>
          {saveError !== null && (
            <p role="alert" className="mt-3 text-sm text-danger">
              {saveError}
            </p>
          )}
        </div>
      )}
    </section>
  );
}
