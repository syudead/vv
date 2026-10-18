import { Unplug } from "lucide-react";

import { t } from "../i18n";
import { DialogError } from "../tags/DialogError";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";

import { PageLinkText } from "./PageLink";
import type { OllamaConnection } from "./useOllamaConnection";

export function ollamaLockNotice(connection: OllamaConnection, onRetry: () => void) {
  const text = t.settings.autoTagging;
  if (connection.status === "unsaved")
    return (
      <Alert role="note">
        <Unplug aria-hidden="true" />
        <AlertTitle>{text.lockTitle}</AlertTitle>
        <AlertDescription>
          <p>
            <PageLinkText sentence={text.lockBody} page="connections" />
          </p>
        </AlertDescription>
      </Alert>
    );
  if (connection.status === "failed")
    return (
      <DialogError
        message={text.connectionReadFailed(connection.reason)}
        onRetry={onRetry}
      />
    );
  return undefined;
}
