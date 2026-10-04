import { Copy, ExternalLink, LoaderCircle, ShieldAlert, Trash2 } from "lucide-react";
import { type FormEvent, useCallback, useEffect, useId, useRef, useState } from "react";

import {
  createAPIToken,
  listAPITokens,
  revokeAPIToken,
  type APIToken,
} from "../api/client";
import { errorText, formatDateTime, formatRelative, t, type UiText } from "../i18n";
import { copyText } from "../lib/clipboard";
import Button from "../ui/legacy/Button";
import { ModalFrame } from "../ui/ModalFrame";
import Skeleton from "../ui/Skeleton";
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
  const cancel = useRef<HTMLButtonElement>(null);
  return (
    <ModalFrame title={text.title} onClose={onClose} initialFocus={cancel}>
      <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-4 sm:p-5">
        <div className="min-w-0 rounded-md border border-control-border bg-field p-3">
          <p className="mb-1 text-xs text-fg-muted">{text.target}</p>
          <p className="break-words text-sm text-fg">{token.name}</p>
          <p className="mt-1 text-xs text-fg-muted tabular-nums">
            {t.settings.apiTokens.created(formatDateTime(token.createdAt))}
          </p>
        </div>
        <p className="border-l-2 border-danger-strong pl-3 text-sm leading-6 text-fg-muted">
          {text.warning}
        </p>
        {error !== null && (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        )}
      </div>
      <div className="flex shrink-0 flex-wrap justify-end gap-2 border-t border-border p-4">
        <Button ref={cancel} onClick={onClose} disabled={pending}>
          {t.common.cancel}
        </Button>
        <Button variant="danger" onClick={onRevoke} disabled={pending}>
          {pending && <LoaderCircle className="animate-spin" />}
          {pending ? text.revoking : text.submit}
        </Button>
      </div>
    </ModalFrame>
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
    <div className="mt-4 min-w-0 rounded-md border border-control-border bg-field p-3 sm:p-4">
      <p className="break-words text-xs text-fg-muted">
        {text.revealTitle(revealed.token.name)}
      </p>
      <code className="mt-2 block select-all break-all font-mono text-sm text-fg">
        {revealed.secret}
      </code>
      <div className="mt-3 flex flex-wrap gap-2">
        <Button ref={copyButton} variant="primary" onClick={copy}>
          <Copy />
          {text.copy}
        </Button>
        <Button onClick={onDone}>{text.done}</Button>
      </div>
      <p className="mt-3 flex gap-2 border-l-2 border-warning-strong pl-3 text-sm leading-6 text-warning">
        <ShieldAlert className="mt-1 size-4 shrink-0" aria-hidden="true" />
        {text.revealWarning}
      </p>
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
    <section
      aria-labelledby="api-tokens-heading"
      className="mt-8 rounded-lg border border-border bg-surface p-4 sm:p-5"
    >
      <div className="border-b border-border pb-4">
        <h2 id="api-tokens-heading" className="text-base font-semibold">
          {text.heading}
        </h2>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-fg-muted">
          {text.description}{" "}
          <a
            href={EXTERNAL_API_GUIDE_URL}
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

      {revealed !== null ? (
        <TokenReveal revealed={revealed} onDone={closeReveal} />
      ) : (
        <form className="pt-4" onSubmit={(event) => void submit(event)} noValidate>
          <label htmlFor={nameId} className="text-xs font-medium text-fg-muted">
            {text.name}
          </label>
          <div className="mt-1 flex flex-col gap-2 sm:flex-row sm:items-start">
            <input
              ref={nameInput}
              id={nameId}
              type="text"
              autoComplete="off"
              value={name}
              onChange={(event) => setName(event.target.value)}
              disabled={creating || createBlocked}
              aria-invalid={createError !== null}
              aria-describedby={describedBy}
              className="h-9 w-full rounded-sm border border-control-border bg-field px-3 text-sm text-fg focus:border-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-link disabled:cursor-not-allowed disabled:opacity-50 sm:max-w-sm"
            />
            <Button
              type="submit"
              variant="primary"
              className="w-full sm:w-auto"
              disabled={trimmed === "" || creating || createBlocked}
            >
              {creating && <LoaderCircle className="animate-spin" />}
              {creating ? text.creating : text.create}
            </Button>
          </div>
          {createError !== null ? (
            <p id={errorId} role="alert" className="mt-2 text-sm text-danger">
              {createError}
            </p>
          ) : (
            <p id={hintId} className="mt-2 text-xs text-fg-muted">
              {text.nameHint}
            </p>
          )}
          {loadError !== null && (
            <p id={blockedId} className="mt-2 text-xs text-fg-muted">
              {text.createBlocked}
            </p>
          )}
        </form>
      )}

      {showList && (
        <div aria-label={text.list} className="mt-5 divide-y divide-border">
          {loading && (
            <div role="status" aria-label={text.loading} className="space-y-3 py-5">
              <Skeleton className="h-12" />
              <Skeleton className="h-12" />
            </div>
          )}
          {!loading && loadError !== null && (
            <div className="flex flex-col items-start gap-3 py-6">
              <p role="alert" className="text-sm text-danger">
                {text.loadFailed(loadError)}
              </p>
              <Button onClick={() => void load()}>{t.common.retry}</Button>
            </div>
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
                className="flex min-w-0 items-start gap-3 py-4"
              >
                <div className="min-w-0 flex-1">
                  <p className="break-words text-sm font-medium text-fg">{token.name}</p>
                  <p className="mt-1 flex flex-wrap gap-x-3 text-xs text-fg-muted tabular-nums">
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
                  className="text-danger"
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
    </section>
  );
}
