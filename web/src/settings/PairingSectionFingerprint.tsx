import { ShieldAlert } from "lucide-react";

import { t, type UiText } from "../i18n";
import { Alert, AlertAction, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";

/**
 * A certificate waiting for the owner's trust. The action flows under the fingerprint rather than
 * in the corner, where it would cover the description at every width.
 */
export function FingerprintAlert({
  title,
  address,
  fingerprint,
  action,
  pending,
  destructive = false,
  body,
  onTrust,
}: {
  title: UiText;
  /** Defaults to asking for a comparison with the certificate at address on the other side. */
  body?: UiText;
  address: string;
  fingerprint: string;
  action: UiText;
  pending: boolean;
  destructive?: boolean;
  onTrust: () => void;
}) {
  return (
    <Alert
      variant={destructive ? "destructive" : "warning"}
      className="has-data-[slot=alert-action]:pr-3"
    >
      <ShieldAlert aria-hidden="true" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription className="flex flex-col gap-2">
        <p>{body ?? t.settings.pairing.certificateBody(address)}</p>
        <code className="block rounded-sm bg-muted px-2 py-1 font-mono text-xs break-all select-all">
          {fingerprint}
        </code>
      </AlertDescription>
      <AlertAction className="static col-start-2 mt-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={pending}
          onClick={onTrust}
        >
          {action}
        </Button>
      </AlertAction>
    </Alert>
  );
}
