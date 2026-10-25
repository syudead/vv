import { Bot, CircleAlert, Eraser, X } from "lucide-react";
import { type ReactNode, useEffect, useRef } from "react";

import { useAudience } from "../auth/audience";
import { errorText, t } from "../i18n";
import { SidePanel } from "../ui/patterns/side-panel";
import { Alert, AlertAction, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { type AssistantStore, type Readiness, useAssistant } from "./AssistantProvider";
import Composer from "./Composer";
import ReplyView, { ConnectionsLink, WaitingRow } from "./ReplyView";

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
  const saved = readiness.reason === "generation_model_unset";
  return (
    <Alert>
      <Bot aria-hidden="true" />
      <AlertTitle>{saved ? words.modelUnsetTitle : words.ollamaNotSavedTitle}</AlertTitle>
      <AlertDescription>
        {saved ? words.modelUnset : words.ollamaNotSaved}
      </AlertDescription>
      <ConnectionsLink />
    </Alert>
  );
}

function EmptyThread({ onExample }: { onExample: (text: string) => void }) {
  const examples = t.assistant.examples;
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
      <p className="text-sm text-muted-foreground">{t.assistant.empty}</p>
      <div className="flex flex-col items-center gap-2">
        {[examples.filter, examples.count, examples.tidy].map((text) => (
          <Button key={text} variant="outline" size="sm" onClick={() => onExample(text)}>
            {text}
          </Button>
        ))}
      </div>
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

function Thread({ store }: { store: AssistantStore }) {
  const { exchanges, waiting, choose } = store;
  return (
    <div role="log" aria-live="polite" className="flex flex-col gap-4">
      {exchanges.map((exchange) => (
        <div key={exchange.id} className="flex flex-col gap-2">
          <p className="max-w-5/6 self-end rounded-lg bg-secondary px-3 py-2 text-sm whitespace-pre-wrap text-secondary-foreground wrap-anywhere">
            {exchange.shown}
          </p>
          {exchange.reply === undefined ? (
            <WaitingRow startedAt={exchange.startedAt} />
          ) : (
            <ReplyView
              exchange={exchange}
              reply={exchange.reply}
              onChoose={waiting ? undefined : (candidate) => choose(exchange, candidate)}
            />
          )}
        </div>
      ))}
    </div>
  );
}

function PanelBody({ store }: { store: AssistantStore }) {
  const { exchanges, readiness } = store;
  if (exchanges.length > 0) return <Thread store={store} />;
  if (readiness.state === "checking") return <CheckingThread />;
  if (readiness.state !== "ready") return null;
  return <EmptyThread onExample={store.setDraft} />;
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
          <Composer store={store} checking={readiness.state === "checking"} />
        ) : (
          <ReadinessAlert readiness={readiness} retry={store.checkReadiness} />
        )
      }
    >
      <PanelBody store={store} />
    </SidePanel>
  );
}
