import { TriangleAlert } from "lucide-react";

import type { AutoImportSettings } from "../api/client";
import { t, type UiText } from "../i18n";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";

import type { AutoImportSource } from "./useAutoImportSettings";

/**
 * autoImportStateLine は監視の状態の 1 行である（specs/042-folder-watch-import/ui-design.md
 * 「States」）。1 つの設定画面では、切り替えの行の説明に置く。
 */
export function autoImportStateLine(settings: AutoImportSettings): UiText {
  const text = t.settings.autoImport.state;
  if (!settings.enabled) return text.off;
  switch (settings.watch.state) {
    case "off":
      return text.noFolder;
    case "starting":
      return text.starting;
    case "active":
      return text.active;
    case "limited":
      return text.limited;
  }
}

function ProblemAlert({ watch }: { watch: AutoImportSettings["watch"] }) {
  if (watch.state !== "limited" || watch.problem === undefined) return null;
  const text = t.settings.autoImport;
  const problem = text.problem[watch.problem];
  // 道筋を持たない問題（events_lost）は、文がそれだけで完結している。
  const path = watch.problem === "events_lost" ? null : (watch.path ?? text.unknownPath);
  return (
    <div>
      <Alert variant="warning" role="note">
        <TriangleAlert aria-hidden="true" />
        <AlertTitle>{problem.title}</AlertTitle>
        <AlertDescription>
          <p>
            {problem.before}
            {path !== null &&
              (watch.path !== undefined ? (
                <code className="break-all">{path}</code>
              ) : (
                path
              ))}
            {problem.after}
          </p>
        </AlertDescription>
      </Alert>
    </div>
  );
}

/**
 * AutoImportState は自動の取り込みの監視の状態とその問題である。監視が `limited` のあいだ、
 * 問題を `warning` の Alert で出す。状態の 1 行は autoImportStateLine が作る。
 *
 * `source` には、監視の状態を追う契機（`followWatch`）で読んだものを渡す。
 */
export default function AutoImportState({ source }: { source: AutoImportSource }) {
  if (source.settings === null) return null;
  return <ProblemAlert watch={source.settings.watch} />;
}
