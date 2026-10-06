import {
  CircleAlert,
  Copy,
  ExternalLink,
  LoaderCircle,
  ShieldAlert,
  Trash2,
} from "lucide-react";
import { type FormEvent, useCallback, useEffect, useId, useRef, useState } from "react";

import {
  createAPIToken,
  listAPITokens,
  revokeAPIToken,
  type APIToken,
} from "../api/client";
import { errorText, formatDateTime, formatRelative, t, type UiText } from "../i18n";
import { copyText } from "../lib/clipboard";
import { ErrorState } from "../ui/patterns/error-state";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "../ui/shadcn/alert-dialog";
import { Button } from "../ui/shadcn/button";
import { Field, FieldDescription, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Skeleton } from "../ui/shadcn/skeleton";
import { useToast } from "../ui/Toast";
import { EXTERNAL_API_GUIDE_URL } from "./docsLinks";

/** Revealed は発行直後の平文の表示である。ページの読み直しや「Done」で消え、二度と出ない。 */
interface Revealed {
  token: APIToken;
  secret: string;
}

function RevokeDialog({
  token,
  pending,
  error,
  onClose,
  onRevoke,
}: {
  token: APIToken;
  pending: boolean;
  error: UiText | null;
  onClose: () => void;
  onRevoke: () => void;
}) {
  const text = t.settings.revokeTokenDialog;
  // 確認の窓の型（ConfirmDialog）と同じ組み方で、失効が終わるまで窓を開いたままにし、
  // 失敗を窓の中に出す。
  return (
    <AlertDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{text.title}</AlertDialogTitle>
          <AlertDialogDescription>{text.warning}</AlertDialogDescription>
        </AlertDialogHeader>
        <div className="min-w-0 rounded-md bg-muted p-3">
          <p className="text-xs text-muted-foreground">{text.target}</p>
          <p className="text-sm break-words">{token.name}</p>
          <p className="text-xs text-muted-foreground tabular-nums">
            {t.settings.apiTokens.created(formatDateTime(token.createdAt))}
          </p>
        </div>
        {error !== null && (
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{error}</AlertTitle>
          </Alert>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>{t.common.cancel}</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={pending}
            onClick={(event) => {
              event.preventDefault();
              onRevoke();
            }}
          >
            {pending && (
              <LoaderCircle className="animate-spin motion-reduce:animate-none" />
            )}
            {pending ? text.revoking : text.submit}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function TokenReveal({ revealed, onDone }: { revealed: Revealed; onDone: () => void }) {
  const text = t.settings.apiTokens;
  const toast = useToast();
  const copyButton = useRef<HTMLButtonElement>(null);

  // 発行の間は入力と button が無効で、focus の行き場が無い。結果の主操作へ移す。
  useEffect(() => copyButton.current?.focus(), []);

  const copy = () =>
    copyText(revealed.secret, {
      onCopied: () => toast(text.copied),
      onFailed: () => toast(text.copyFailed),
    });

  return (
    <div className="flex min-w-0 flex-col gap-3">
      <p className="text-xs break-words text-muted-foreground">
        {text.revealTitle(revealed.token.name)}
      </p>
      <code className="block rounded-md bg-muted p-3 font-mono text-sm break-all select-all">
        {revealed.secret}
      </code>
      <div className="flex flex-wrap gap-2">
        <Button ref={copyButton} size="sm" onClick={copy}>
          <Copy />
          {text.copy}
        </Button>
        <Button variant="outline" size="sm" onClick={onDone}>
          {text.done}
        </Button>
      </div>
      <Alert variant="warning" role="note">
        <ShieldAlert aria-hidden="true" />
        <AlertTitle className="font-normal">{text.revealWarning}</AlertTitle>
      </Alert>
    </div>
  );
}

/**
 * APITokensSection は設定画面の「API tokens」節である。所有者が外部連携 API と MCP の
 * トークンを発行し、発行直後に一度だけ平文を見てコピーし、一覧で名前と日時を確かめ、
 * 確認の後に失効する（specs/026-external-api/ui-design.md）。
 */
export default function APITokensSection() {
  const text = t.settings.apiTokens;
  const toast = useToast();
  const nameId = useId();
  const hintId = useId();
  const errorId = useId();
  const blockedId = useId();
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<UiText | null>(null);
  const [revealed, setRevealed] = useState<Revealed | null>(null);
  const [revoking, setRevoking] = useState<APIToken | null>(null);
  const [revokePending, setRevokePending] = useState(false);
  const [revokeError, setRevokeError] = useState<UiText | null>(null);
  const nameInput = useRef<HTMLInputElement>(null);
  const rowRefs = useRef(new Map<number, HTMLDivElement>());
  const focusAfterRender = useRef<number | "name" | null>(null);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    setLoadError(null);
    try {
      setTokens(await listAPITokens(signal));
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

  // 失効や「Done」の後の focus は、行や入力が描かれてから移す。
  useEffect(() => {
    const target = focusAfterRender.current;
    if (target === null) return;
    focusAfterRender.current = null;
    if (target === "name") nameInput.current?.focus();
    else rowRefs.current.get(target)?.querySelector<HTMLButtonElement>("button")?.focus();
  });

  const createBlocked = loading || loadError !== null;
  const trimmed = name.trim();

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (trimmed === "" || creating || createBlocked) return;
    setCreating(true);
    setCreateError(null);
    try {
      const created = await createAPIToken(name);
      setTokens((current) => [
        created.token,
        ...current.filter((token) => token.id !== created.token.id),
      ]);
      setRevealed({ token: created.token, secret: created.secret });
      setName("");
    } catch (failure) {
      setCreateError(errorText(failure));
    } finally {
      setCreating(false);
    }
  };

  const closeReveal = () => {
    setRevealed(null);
    focusAfterRender.current = "name";
  };

  const revoke = async () => {
    if (revoking === null) return;
    const target = revoking;
    setRevokePending(true);
    setRevokeError(null);
    try {
      await revokeAPIToken(target.id);
      // 失効を待つあいだに発行が終わり、一覧の先頭に足されていることがある。
      // 失効を始めたときの一覧ではなく最新の一覧から、対象の 1 本だけを除く。
      setTokens((current) => {
        const index = current.findIndex((token) => token.id === target.id);
        const remaining = current.filter((token) => token.id !== target.id);
        const next = remaining[Math.min(Math.max(index, 0), remaining.length - 1)];
        focusAfterRender.current = next === undefined ? "name" : next.id;
        return remaining;
      });
      // 失効したトークンの平文は残さない。
      setRevealed((current) => (current?.token.id === target.id ? null : current));
      setRevoking(null);
      toast(text.revoked);
    } catch (failure) {
      setRevokeError(errorText(failure));
    } finally {
      setRevokePending(false);
    }
  };

  const describedBy = [
    createError !== null ? errorId : hintId,
    loadError !== null ? blockedId : null,
  ]
    .filter((id) => id !== null)
    .join(" ");

  const showList = loading || loadError !== null || tokens.length > 0;

  return (
    <PageSection
      title={text.heading}
      description={
        <>
          {text.description}{" "}
          <a
            href={EXTERNAL_API_GUIDE_URL}
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
      {revealed !== null ? (
        <TokenReveal revealed={revealed} onDone={closeReveal} />
      ) : (
        <form onSubmit={(event) => void submit(event)} noValidate>
          <Field data-invalid={createError !== null || undefined}>
            <FieldLabel htmlFor={nameId}>{text.name}</FieldLabel>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-start">
              <Input
                ref={nameInput}
                id={nameId}
                type="text"
                autoComplete="off"
                value={name}
                onChange={(event) => setName(event.target.value)}
                disabled={creating || createBlocked}
                aria-invalid={createError !== null}
                aria-describedby={describedBy}
                className="h-8 sm:max-w-sm"
              />
              <Button
                type="submit"
                size="sm"
                disabled={trimmed === "" || creating || createBlocked}
              >
                {creating && (
                  <LoaderCircle className="animate-spin motion-reduce:animate-none" />
                )}
                {creating ? text.creating : text.create}
              </Button>
            </div>
            {createError !== null ? (
              <FieldError id={errorId}>{createError}</FieldError>
            ) : (
              <FieldDescription id={hintId}>{text.nameHint}</FieldDescription>
            )}
            {loadError !== null && (
              <FieldDescription id={blockedId}>{text.createBlocked}</FieldDescription>
            )}
          </Field>
        </form>
      )}

      {showList && (
        <div aria-label={text.list} className="flex flex-col divide-y divide-border">
          {loading && (
            <div role="status" aria-label={text.loading} className="flex flex-col gap-3">
              <Skeleton className="h-10" />
              <Skeleton className="h-10" />
            </div>
          )}
          {!loading && loadError !== null && (
            <ErrorState
              title={text.loadFailed(loadError)}
              retryLabel={t.common.retry}
              onRetry={() => void load()}
            />
          )}
          {!loading &&
            loadError === null &&
            tokens.map((token) => (
              <div
                key={token.id}
                ref={(element) => {
                  if (element === null) rowRefs.current.delete(token.id);
                  else rowRefs.current.set(token.id, element);
                }}
                className="flex min-w-0 items-start gap-3 py-3 first:pt-0 last:pb-0"
              >
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <p className="font-medium break-words">{token.name}</p>
                  <p className="flex flex-wrap gap-x-3 text-xs text-muted-foreground tabular-nums">
                    <span>{text.created(formatDateTime(token.createdAt))}</span>
                    {token.lastUsedAt === null ? (
                      <span>{text.neverUsed}</span>
                    ) : (
                      <span title={formatDateTime(token.lastUsedAt)}>
                        {text.lastUsed(formatRelative(token.lastUsedAt))}
                      </span>
                    )}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={text.revokeNamed(token.name)}
                  onClick={() => {
                    setRevokeError(null);
                    setRevoking(token);
                  }}
                  disabled={revoking?.id === token.id && revokePending}
                  className="text-destructive"
                >
                  <Trash2 />
                  {text.revoke}
                </Button>
              </div>
            ))}
        </div>
      )}

      {revoking !== null && (
        <RevokeDialog
          token={revoking}
          pending={revokePending}
          error={revokeError}
          onClose={() => {
            if (!revokePending) setRevoking(null);
          }}
          onRevoke={() => void revoke()}
        />
      )}
    </PageSection>
  );
}
