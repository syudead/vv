import { useCallback, useEffect, useId, useRef, useState } from "react";
import { useNavigate } from "react-router";

import { isAborted } from "../api/client";
import { type HomeOrganizer, organizeHome, readHome } from "../api/home";
import { ONBOARDING_PATH, writeOnboardingStep } from "../api/onboarding";
import { subscribeServerEvents } from "../api/serverEvents";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { FormRow } from "../ui/patterns/form-row";
import { PageSection } from "../ui/patterns/page-section";
import { Button } from "../ui/shadcn/button";
import { Spinner } from "../ui/shadcn/spinner";
import { useToast } from "../ui/Toast";
import { lastRunLine, organizerBusy } from "./homeSectionRules";

type Load =
  | { status: "loading" }
  | { status: "failed"; reason: UiText }
  | { status: "ready"; organizer: HomeOrganizer };

/**
 * useOrganizer はホームの整理役の状態を GET /api/home で読み、`home` の知らせとつなぎ直しで
 * 読み直す。`set` は POST /api/home/organize の応答を当てて読み直す。
 */
function useOrganizer() {
  const [load, setLoad] = useState<Load>({ status: "loading" });
  const loaded = useRef(false);
  const read = useCallback(async (signal: AbortSignal) => {
    try {
      const home = await readHome(signal);
      loaded.current = true;
      // 所有者の応答には必ず入る。無ければ（ゲスト）走っていないものとして扱う。
      setLoad({ status: "ready", organizer: home.organizer ?? { running: false } });
    } catch (error) {
      // 読めている状態は残す。読み直しの失敗で、節を壊さない。
      if (isAborted(error) || loaded.current) return;
      setLoad({ status: "failed", reason: errorText(error) });
    }
  }, []);
  const controller = useRef(new AbortController());
  const reload = useCallback(() => {
    controller.current.abort();
    controller.current = new AbortController();
    void read(controller.current.signal);
  }, [read]);
  useEffect(() => {
    const unsubscribe = subscribeServerEvents({
      home: reload,
      open: (reconnected) => {
        if (reconnected) reload();
      },
    });
    reload();
    return () => {
      unsubscribe();
      controller.current.abort();
    };
  }, [reload]);
  // 応答を当てた後、読み直す。応答より先に `home` の知らせ（実行の終わり）が届いていても、
  // 応答の後に読んだ今の状態で待ちを消す。
  const set = useCallback(
    (organizer: HomeOrganizer) => {
      setLoad({ status: "ready", organizer });
      reload();
    },
    [reload],
  );
  const retry = useCallback(() => {
    setLoad({ status: "loading" });
    reload();
  }, [reload]);
  return { load, set, retry };
}

/**
 * HomeSection は設定の「Home」の節である（specs/057-home/ui-design.md「Smart folder page and
 * Settings」）。「Reorganize」は整理役の組み直しを待ちに入れ（research.md R-9）、待ちか実行中は
 * `Spinner` を出して押せない。説明の下に最後の実行を出す。「Start setup」はオンボードの段階を
 * `folders` に書いてオンボードの画面を開く。
 */
export default function HomeSection() {
  const text = t.settings.home;
  const { load, set, retry } = useOrganizer();
  const toast = useToast();
  const navigate = useNavigate();
  const reorganizeId = useId();
  const setupId = useId();
  const [requesting, setRequesting] = useState(false);
  const [startingSetup, setStartingSetup] = useState(false);

  const reorganize = async () => {
    setRequesting(true);
    try {
      set(await organizeHome());
    } catch (error) {
      toast(text.reorganizeFailed(errorText(error)));
    } finally {
      setRequesting(false);
    }
  };

  const startSetup = async () => {
    setStartingSetup(true);
    try {
      await writeOnboardingStep("folders");
      void navigate(ONBOARDING_PATH);
    } catch (error) {
      toast(text.setupFailed(errorText(error)));
      setStartingSetup(false);
    }
  };

  const busy =
    requesting || (load.status === "ready" ? organizerBusy(load.organizer) : true);

  return (
    <PageSection title={text.heading}>
      {load.status === "failed" ? (
        <ErrorState
          title={text.loadFailed(load.reason)}
          retryLabel={t.common.retry}
          onRetry={retry}
        />
      ) : (
        <FormRow
          label={text.shelves}
          htmlFor={reorganizeId}
          description={
            <>
              {text.shelvesDescription}
              {load.status === "ready" && (
                <span className="mt-1 block text-xs text-muted-foreground">
                  {lastRunLine(load.organizer.last)}
                </span>
              )}
            </>
          }
        >
          <Button
            id={reorganizeId}
            // 行の名札（Shelves）ではなく、見えている文言を名前にする。
            aria-label={text.reorganize}
            variant="outline"
            size="sm"
            disabled={busy}
            onClick={() => void reorganize()}
          >
            {busy && <Spinner aria-hidden="true" role="presentation" />}
            {text.reorganize}
          </Button>
        </FormRow>
      )}
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
