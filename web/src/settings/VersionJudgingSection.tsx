import { useCallback, useEffect, useRef, useState } from "react";

import {
  getVersionJudgingSettings,
  isAborted,
  updateVersionJudgingSettings,
  type VersionJudgingSettings,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { FormRow } from "../ui/patterns/form-row";
import { PageSection } from "../ui/patterns/page-section";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Switch } from "../ui/shadcn/switch";

import { PageLinkText } from "./PageLink";
import { SaveStatus } from "./SaveStatus";
import { useOllamaConnection } from "./useOllamaConnection";
import { useSwitchSave } from "./useSwitchSave";

/**
 * VersionJudgingSection は設定の「Duplicate judging」区画である。「同じ動画かもしれない」候補の
 * 自動の判定を、所有者が 1 つのスイッチで入れたり切ったりする
 * （specs/054-auto-dup/ui-design.md「Settings: Duplicate judging section」）。
 *
 * スイッチを押すとすぐに保存し、保存ボタンも成功の通知も無い。判定の進み具合は出さない
 * （それは Duplicates の画面が見せる）。
 *
 * Ollama の接続先を読み、保存されていないと答えたあいだだけ、違いのある組の判定には接続が要る旨と
 * 「Connections」へのリンクをスイッチの下に添える。スイッチはどの状態でも押せ、読み込み中と
 * 読めなかったときは何も足さない（specs/058-settings-pages/research.md R-5、
 * ui-design.md「Library page」）。
 */
export default function VersionJudgingSection() {
  const text = t.settings.versionJudging;
  const [settings, setSettings] = useState<VersionJudgingSettings | null>(null);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const controller = useRef<AbortController | null>(null);
  const { connection } = useOllamaConnection();

  const load = useCallback(async () => {
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    setLoadError(null);
    try {
      setSettings(await getVersionJudgingSettings(abort.signal));
    } catch (failure) {
      if (isAborted(failure)) return;
      setLoadError(errorText(failure));
    }
  }, []);

  useEffect(() => {
    void load();
    return () => controller.current?.abort();
  }, [load]);

  // 失敗するとスイッチは保存した値に戻る（saving を解くと settings.enabled を映す）。
  const { saving, saveError, toggle } = useSwitchSave({
    settings,
    current: (saved) => saved.enabled,
    save: updateVersionJudgingSettings,
    saved: setSettings,
    saveFailed: text.saveFailed,
  });

  return (
    <PageSection
      title={text.heading}
      description={<PageLinkText sentence={text.description} page="connections" />}
    >
      {loadError !== null && settings === null && (
        <div>
          <ErrorState
            title={text.loadFailed(loadError)}
            retryLabel={t.common.retry}
            onRetry={() => void load()}
          />
        </div>
      )}
      {loadError === null && (
        <FormRow
          label={text.label}
          description={text.hint}
          htmlFor="version-judging-enabled"
        >
          {settings === null ? (
            // Skeleton は既定で読み上げから隠れる。読み込み中であることは読み上げに出す。
            <Skeleton
              aria-hidden={false}
              role="status"
              aria-label={text.loading}
              className="h-5 w-9 rounded-full"
            />
          ) : (
            <Switch
              id="version-judging-enabled"
              checked={saving ?? settings.enabled}
              aria-disabled={saving !== null || undefined}
              onCheckedChange={() => void toggle()}
            />
          )}
        </FormRow>
      )}
      {loadError === null && connection.status === "unsaved" && (
        <p className="text-sm text-muted-foreground">
          <PageLinkText sentence={text.connectionNote} page="connections" />
        </p>
      )}
      <SaveStatus saving={saving !== null} savingText={text.saving} error={saveError} />
    </PageSection>
  );
}
