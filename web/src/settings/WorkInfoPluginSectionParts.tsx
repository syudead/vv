import {
  ExternalLink,
  LoaderCircle,
  Puzzle,
  Upload,
  UserRoundSearch,
} from "lucide-react";
import { type ChangeEvent, type ReactNode, type RefObject, useRef } from "react";

import type { WorkInfoPlugin, WorkInfoPluginKind } from "../api/workInfoPlugins";
import { t, type UiText } from "../i18n";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { Button } from "../ui/shadcn/button";
import { Input } from "../ui/shadcn/input";
import { Skeleton } from "../ui/shadcn/skeleton";
import { WORK_INFO_PLUGINS_GUIDE_URL } from "./docsLinks";
import { MAX_PLUGIN_BYTES } from "./workInfoPluginRows";

const text = () => t.settings.workInfoPlugins;

/** The texts that differ between the work-info and person sections. */
export function sectionText(kind: WorkInfoPluginKind) {
  const all = text();
  const own = kind === "person" ? all.person : all;
  return {
    heading: own.heading,
    description: own.description,
    list: own.list,
    loading: own.loading,
    loadFailed: own.loadFailed,
    empty: own.empty,
    emptyHint: own.emptyHint,
  };
}

export function GuideLink() {
  return (
    <a
      href={WORK_INFO_PLUGINS_GUIDE_URL}
      target="_blank"
      rel="noreferrer"
      className="inline-flex items-center gap-1 text-primary underline underline-offset-4"
    >
      {text().guide}
      <ExternalLink aria-hidden="true" className="size-3" />
      <span className="sr-only">{text().opensInNewTab}</span>
    </a>
  );
}

export function AddButton({
  kind,
  buttonRef,
  adding,
  blocked,
  onFile,
  onTooLarge,
}: {
  kind: WorkInfoPluginKind;
  buttonRef: RefObject<HTMLButtonElement | null>;
  adding: boolean;
  blocked: boolean;
  onFile: (file: File) => void;
  onTooLarge: () => void;
}) {
  const fileInput = useRef<HTMLInputElement>(null);
  const unavailable = adding || blocked;
  const choose = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (file === undefined) return;
    if (file.size > MAX_PLUGIN_BYTES) onTooLarge();
    else onFile(file);
  };
  return (
    <>
      <Button
        ref={buttonRef}
        variant="outline"
        size="sm"
        aria-disabled={unavailable || undefined}
        className="aria-disabled:cursor-default aria-disabled:opacity-50"
        onClick={() => {
          if (!unavailable) fileInput.current?.click();
        }}
      >
        {adding ? (
          <LoaderCircle
            aria-hidden="true"
            className="animate-spin motion-reduce:animate-none"
          />
        ) : (
          <Upload aria-hidden="true" />
        )}
        {adding ? text().adding : text().add}
      </Button>
      <Input
        ref={fileInput}
        type="file"
        accept=".wasm"
        tabIndex={-1}
        aria-hidden="true"
        aria-label={text().fileInput}
        className="hidden"
        data-testid={kind === "person" ? "person-plugin-input" : "work-info-plugin-input"}
        onChange={choose}
      />
    </>
  );
}

export function PluginList({
  kind,
  loading,
  loadError,
  plugins,
  onRetry,
  renderRow,
}: {
  kind: WorkInfoPluginKind;
  loading: boolean;
  loadError: UiText | null;
  plugins: WorkInfoPlugin[];
  onRetry: () => void;
  renderRow: (plugin: WorkInfoPlugin) => ReactNode;
}) {
  const own = sectionText(kind);
  if (loading)
    return (
      <div role="status" aria-label={own.loading} className="flex flex-col gap-3">
        <Skeleton className="h-10" />
        <Skeleton className="h-10" />
      </div>
    );
  if (loadError !== null)
    return (
      <ErrorState
        title={own.loadFailed(loadError)}
        retryLabel={t.common.retry}
        onRetry={onRetry}
      />
    );
  if (plugins.length === 0)
    return (
      <EmptyState
        icon={kind === "person" ? <UserRoundSearch /> : <Puzzle />}
        title={own.empty}
        description={own.emptyHint}
      />
    );
  return (
    <div aria-label={own.list} className="flex flex-col divide-y divide-border">
      {plugins.map(renderRow)}
    </div>
  );
}
