import { Copy } from "lucide-react";
import { useEffect, useId, useState } from "react";

import { issuePairingCode, type PairingCode } from "../api/peer";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { copyText } from "../lib/clipboard";
import { FormRow } from "../ui/patterns/form-row";
import { Button } from "../ui/shadcn/button";
import { FieldError } from "../ui/shadcn/field";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { useToast } from "../ui/Toast";

import { formatCountdown, groupPairingCode } from "./pairing";

/** Milliseconds left on the code, updated each second until it expires. */
function useRemaining(expiresAt: string | null): number {
  const deadline = expiresAt === null ? 0 : Date.parse(expiresAt);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (expiresAt === null) return;
    setNow(Date.now());
    const timer = setInterval(() => {
      const current = Date.now();
      setNow(current);
      if (current >= deadline) clearInterval(timer);
    }, 1000);
    return () => clearInterval(timer);
  }, [expiresAt, deadline]);
  return deadline - now;
}

function ShownCode({
  code,
  issuing,
  disabled,
  onRenew,
}: {
  code: PairingCode;
  issuing: boolean;
  disabled: boolean;
  onRenew: () => void;
}) {
  const text = t.settings.pairing.code;
  const toast = useToast();
  const remaining = useRemaining(code.expiresAt);
  const expired = remaining <= 0;
  const copy = () =>
    copyText(code.code, {
      onCopied: () => toast(text.copied),
      onFailed: () => toast(text.copyFailed),
    });
  return (
    <div className="flex min-w-0 flex-col gap-1 sm:w-2xs sm:items-end lg:w-sm">
      <div className="flex items-center gap-1">
        <code
          className={cn(
            "font-mono text-xl font-semibold whitespace-nowrap tabular-nums select-all",
            expired && "text-muted-foreground",
          )}
        >
          {groupPairingCode(code.code)}
        </code>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label={text.copy} onClick={copy}>
              <Copy />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{text.copy}</TooltipContent>
        </Tooltip>
      </div>
      <div className="flex items-center gap-2">
        <p role="status" className="text-xs text-muted-foreground tabular-nums">
          {expired ? text.expired : text.expiresIn(formatCountdown(remaining))}
        </p>
        <Button
          variant="ghost"
          size="sm"
          disabled={disabled || issuing}
          onClick={onRenew}
        >
          {text.renew}
        </Button>
      </div>
      <p className="text-sm text-muted-foreground sm:text-right">
        {text.hint} <code>{window.location.origin}</code>.
      </p>
    </div>
  );
}

/** Showing a code never touches the pairing form; a new code replaces the one shown. */
export function PairingCodeRow({ disabled }: { disabled: boolean }) {
  const text = t.settings.pairing.code;
  const codeId = useId();
  const errorId = useId();
  const [code, setCode] = useState<PairingCode | null>(null);
  const [issuing, setIssuing] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);

  const issue = async () => {
    setIssuing(true);
    setFailure(null);
    try {
      setCode(await issuePairingCode());
    } catch (error) {
      setFailure(text.failed(errorText(error)));
    } finally {
      setIssuing(false);
    }
  };

  return (
    <FormRow label={text.label} description={text.description} htmlFor={codeId}>
      <div id={codeId} className="flex min-w-0 flex-col gap-2 sm:items-end">
        {code === null ? (
          <Button
            variant="outline"
            size="sm"
            disabled={disabled || issuing}
            aria-describedby={failure === null ? undefined : errorId}
            onClick={() => void issue()}
          >
            {text.show}
          </Button>
        ) : (
          <ShownCode
            code={code}
            issuing={issuing}
            disabled={disabled}
            onRenew={() => void issue()}
          />
        )}
        {failure !== null && <FieldError id={errorId}>{failure}</FieldError>}
      </div>
    </FormRow>
  );
}
