import { Unplug } from "lucide-react";

import { t } from "../i18n";
import { DialogError } from "../tags/DialogError";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";

import { PageLinkText } from "./PageLink";
import type { OllamaConnection } from "./useOllamaConnection";

/**
 * ollamaLockNotice は「Auto-tagging」のページの notice の 1 行である。接続先が保存されて
 * いなければ、設定を押せない理由と「Connections」へのリンクを、読めなければ読めなかった旨と
 * `Retry` を出す。読み込み中と保存済みは何も出さない（undefined）。
 */
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
