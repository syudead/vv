import { Bot, CircleAlert, Eraser, X } from "lucide-react";
import { type ReactNode, useEffect, useRef } from "react";

import type { AssistantReadinessReason } from "../api/assistant";
import { useAudience } from "../auth/audience";
import { errorText, t } from "../i18n";
import { SidePanel } from "../ui/patterns/side-panel";
import { Alert, AlertAction, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { type AssistantStore, type Readiness, useAssistant } from "./AssistantProvider";
import Composer from "./Composer";
import ProposalCard, { RunRow } from "./ProposalCard";
import ReplyView, { ConnectionsLink, useGenerationModel } from "./ReplyView";
import type { Exchange } from "./thread";

export const assistantPanelId = "assistant-panel";

/** Closing returns the focus to the trigger that opened the panel. */
function focusTrigger() {
  document.querySelector<HTMLElement>("[data-assistant-trigger]")?.focus();
}

function HeaderButton({
  label,
  onClick,
  children,
}: {
  label: string;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={label} onClick={onClick}>
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

function ReadinessAlert({
  readiness,
  retry,
}: {
  readiness: Readiness;
  retry: () => void;
}) {
  const words = t.assistant.readiness;
  if (readiness.state === "failed") {
    return (
      <Alert variant="destructive">
        <CircleAlert aria-hidden="true" />
        <AlertTitle className="font-normal">
          {words.failed(errorText(readiness.error))}
        </AlertTitle>
        <AlertAction>
          <Button variant="outline" size="sm" onClick={retry}>
            {t.common.retry}
          </Button>
        </AlertAction>
      </Alert>
    );
  }
  if (readiness.state !== "notReady") return null;
  return <NotReadyAlert reason={readiness.reason} />;
}

function NotReadyAlert({ reason }: { reason: AssistantReadinessReason }) {
  const words = t.assistant.readiness;
  const model = useGenerationModel(reason === "generation_model_without_tools");
  const [title, description] =
    reason === "generation_model_without_tools"
      ? [words.withoutToolsTitle, words.withoutTools(model)]
      : reason === "generation_model_unset"
        ? [words.modelUnsetTitle, words.modelUnset]
        : [words.ollamaNotSavedTitle, words.ollamaNotSaved];
  return (
    <Alert>
      <Bot aria-hidden="true" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{description}</AlertDescription>
      <ConnectionsLink />
    </Alert>
  );
}

function EmptyThread({
  onExample,
  vision,
}: {
  onExample: (text: string) => void;
  vision: boolean;
}) {
  const examples = t.assistant.examples;
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
      <p className="text-sm text-muted-foreground">{t.assistant.empty}</p>
      <div className="flex flex-col items-center gap-2">
        {[examples.library, examples.help, examples.tidy].map((text) => (
          <Button key={text} variant="outline" size="sm" onClick={() => onExample(text)}>
            {text}
          </Button>
        ))}
      </div>
      {!vision && (
        <p className="text-xs text-muted-foreground">{t.assistant.withoutVision}</p>
      )}
    </div>
  );
}

function CheckingThread() {
  return (
    <div className="flex flex-col gap-4" aria-hidden="true">
      {[0, 1].map((index) => (
        <div key={index} className="flex flex-col gap-2">
          <Skeleton className="h-8 w-2/3 self-end" />
          <Skeleton className="h-4 w-1/2" />
        </div>
      ))}
    </div>
  );
}

function ExchangeReply({
  store,
  exchange,
}: {
  store: AssistantStore;
  exchange: Exchange;
}) {
  const card = store.cards.get(exchange.id);
  const { waiting, choose } = store;
  return (
    <>
      <ReplyView
        exchange={exchange}
        onChoose={waiting ? undefined : (candidate) => choose(exchange, candidate)}
      />
      {card !== undefined && exchange.reply?.kind !== "expired" && (
        <ProposalCard
          card={card}
          onExecute={(steps) => store.execute(exchange.id, steps)}
          onDismiss={() => store.dismiss(exchange.id)}
        />
      )}
    </>
  );
}

function RecentRuns({ store }: { store: AssistantStore }) {
  if (store.runs.length === 0) return null;
  const headingId = "assistant-recent-runs";
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-2">
      <h3 id={headingId} className="text-xs font-medium text-muted-foreground">
        {t.assistant.recentRuns}
      </h3>
      <ul className="flex flex-col gap-3">
        {store.runs.map((proposal) => (
          <RunRow key={proposal.id} proposal={proposal} />
        ))}
      </ul>
    </section>
  );
}

function Thread({ store }: { store: AssistantStore }) {
  const { exchanges } = store;
  return (
    <div role="log" aria-live="polite" className="flex flex-col gap-4">
      {exchanges.map((exchange) => (
        <div key={exchange.id} className="flex flex-col gap-2">
          <p className="max-w-5/6 self-end rounded-lg bg-secondary px-3 py-2 text-sm whitespace-pre-wrap text-secondary-foreground wrap-anywhere">
            {exchange.shown}
          </p>
          <ExchangeReply store={store} exchange={exchange} />
        </div>
      ))}
    </div>
  );
}

function ThreadBody({ store }: { store: AssistantStore }) {
  const { exchanges, readiness } = store;
  if (exchanges.length > 0) return <Thread store={store} />;
  if (readiness.state === "checking" || store.loading) return <CheckingThread />;
  if (readiness.state !== "ready") return null;
  return <EmptyThread onExample={store.setDraft} vision={readiness.vision} />;
}

function PanelBody({ store }: { store: AssistantStore }) {
  if (store.runs.length === 0) return <ThreadBody store={store} />;
  return (
    <div className="flex min-h-full flex-col gap-4">
      <RecentRuns store={store} />
      <div className="flex-1">
        <ThreadBody store={store} />
      </div>
    </div>
  );
}

/** The assistant's side column, owner-only; it keeps no state of its own beyond the root store. */
export default function AssistantPanel() {
  const store = useAssistant();
  const owner = useAudience() === "owner";
  const body = useRef<HTMLDivElement>(null);
  const exchanges = store?.exchanges;
  const open = store?.open === true;
  // Show the newest row when a row arrives and when the panel opens again.
  useEffect(() => {
    const node = body.current;
    if (open && node !== null) node.scrollTop = node.scrollHeight;
  }, [exchanges, open]);

  if (store === null || !owner || !store.open) return null;
  const close = () => {
    store.close();
    focusTrigger();
  };
  const { readiness } = store;
  const composer = readiness.state === "ready" || readiness.state === "checking";
  return (
    <SidePanel
      id={assistantPanelId}
      label={t.assistant.title}
      title={t.assistant.title}
      onClose={close}
      bodyRef={body}
      actions={
        <>
          {store.exchanges.length > 0 && (
            <HeaderButton label={t.assistant.clearThread} onClick={store.clear}>
              <Eraser aria-hidden="true" />
            </HeaderButton>
          )}
          <HeaderButton label={t.common.close} onClick={close}>
            <X aria-hidden="true" />
          </HeaderButton>
        </>
      }
      footer={
        composer ? (
          <Composer
            store={store}
            checking={readiness.state === "checking" || store.loading}
          />
        ) : (
          <ReadinessAlert readiness={readiness} retry={store.checkReadiness} />
        )
      }
    >
      <PanelBody store={store} />
    </SidePanel>
  );
}
