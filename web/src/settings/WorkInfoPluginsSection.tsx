import { CircleAlert, ExternalLink, LoaderCircle, Puzzle, Upload } from "lucide-react";
import {
  type ChangeEvent,
  type ReactNode,
  type RefObject,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";

import { RequestFailed } from "../api/client";
import {
  approveWorkInfoPlugin,
  deleteWorkInfoPlugin,
  listWorkInfoPlugins,
  revokeWorkInfoPluginApproval,
  setWorkInfoPluginSettings,
  uploadWorkInfoPlugin,
  type WorkInfoPlugin,
} from "../api/workInfoPlugins";
import { errorText, t, type UiText } from "../i18n";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Input } from "../ui/shadcn/input";
import { Skeleton } from "../ui/shadcn/skeleton";
import { useToast } from "../ui/Toast";
import { WORK_INFO_PLUGINS_GUIDE_URL } from "./docsLinks";
import { useSectionAnchor } from "./useSectionAnchor";
import WorkInfoPluginApprovalDialog from "./WorkInfoPluginApprovalDialog";
import WorkInfoPluginRow, { type RowWork } from "./WorkInfoPluginRow";
import WorkInfoPluginRunDialog from "./WorkInfoPluginRunDialog";
import { MAX_PLUGIN_BYTES, pluginName } from "./workInfoPluginRows";
import WorkInfoPluginSettingsDialog from "./WorkInfoPluginSettingsDialog";

const text = () => t.settings.workInfoPlugins;

/** changed means the reviewed plugin was replaced or became unavailable. */
interface Approving {
  plugin: WorkInfoPlugin;
  pending: boolean;
  changed: boolean;
  error: UiText | null;
}

interface Editing {
  plugin: WorkInfoPlugin;
  pending: boolean;
  error: UiText | null;
}

/** Focus the row's first button, its menu trigger, or Add plugin after rendering. */
type FocusTarget = number | { menu: number } | "add";

function addedToast(plugin: WorkInfoPlugin, replaced: boolean): UiText {
  const name = pluginName(plugin);
  if (plugin.state === "invalid") return text().addedUnusable(name);
  return replaced ? text().replaced(name) : text().added(name);
}

function isChangedMeanwhile(failure: unknown): boolean {
  return (
    failure instanceof RequestFailed &&
    (failure.code === "plugin_changed" || failure.code === "plugin_unusable")
  );
}

function GuideLink() {
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

function AddButton({
  buttonRef,
  adding,
  blocked,
  onFile,
  onTooLarge,
}: {
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
        data-testid="work-info-plugin-input"
        onChange={choose}
      />
    </>
  );
}

function PluginList({
  loading,
  loadError,
  plugins,
  onRetry,
  renderRow,
}: {
  loading: boolean;
  loadError: UiText | null;
  plugins: WorkInfoPlugin[];
  onRetry: () => void;
  renderRow: (plugin: WorkInfoPlugin) => ReactNode;
}) {
  if (loading)
    return (
      <div role="status" aria-label={text().loading} className="flex flex-col gap-3">
        <Skeleton className="h-10" />
        <Skeleton className="h-10" />
      </div>
    );
  if (loadError !== null)
    return (
      <ErrorState
        title={text().loadFailed(loadError)}
        retryLabel={t.common.retry}
        onRetry={onRetry}
      />
    );
  if (plugins.length === 0)
    return (
      <EmptyState icon={<Puzzle />} title={text().empty} description={text().emptyHint} />
    );
  return (
    <div aria-label={text().list} className="flex flex-col divide-y divide-border">
      {plugins.map(renderRow)}
    </div>
  );
}

export default function WorkInfoPluginsSection() {
  const anchor = useSectionAnchor("work-info-plugins");
  return (
    <div ref={anchor} id="work-info-plugins">
      <WorkInfoPluginsSectionBody />
    </div>
  );
}

function WorkInfoPluginsSectionBody() {
  const toast = useToast();
  const [plugins, setPlugins] = useState<WorkInfoPlugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState<UiText | null>(null);
  const [work, setWork] = useState<{ id: number; kind: RowWork } | null>(null);
  const [rowError, setRowError] = useState<{ id: number; message: UiText } | null>(null);
  const [approving, setApproving] = useState<Approving | null>(null);
  const [configuring, setConfiguring] = useState<Editing | null>(null);
  const [deleting, setDeleting] = useState<Editing | null>(null);
  const [reading, setReading] = useState<WorkInfoPlugin | null>(null);
  const addButton = useRef<HTMLButtonElement>(null);
  const rowRefs = useRef(new Map<number, HTMLDivElement>());
  const focusAfterRender = useRef<FocusTarget | null>(null);
  // Track started loads separately from mutations applied to rows.
  const loads = useRef(0);
  const changes = useRef(0);

  /**
   * Discard superseded loads; reread if a mutation changed rows during the request to avoid
   * undoing it. Quiet failures retain the current list without section errors.
   */
  const load = useCallback(
    async ({ signal, quiet = false }: { signal?: AbortSignal; quiet?: boolean } = {}) => {
      const id = ++loads.current;
      if (!quiet) setLoadError(null);
      for (;;) {
        const changesAtStart = changes.current;
        try {
          const list = await listWorkInfoPlugins(signal);
          if (id !== loads.current) return;
          if (changes.current !== changesAtStart) continue;
          setPlugins(list);
          setLoadError(null);
          setLoading(false);
          return;
        } catch (failure) {
          if (signal?.aborted || id !== loads.current) return;
          if (!quiet) setLoadError(errorText(failure));
          setLoading(false);
          return;
        }
      }
    },
    [],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load({ signal: controller.signal });
    return () => controller.abort();
  }, [load]);

  // Defer until after rendering and Radix's close-time focus restoration.
  useEffect(() => {
    const target = focusAfterRender.current;
    if (target === null) return;
    focusAfterRender.current = null;
    const timer = window.setTimeout(() => {
      if (target === "add") addButton.current?.focus();
      else if (typeof target === "number")
        rowRefs.current.get(target)?.querySelector<HTMLButtonElement>("button")?.focus();
      else
        rowRefs.current
          .get(target.menu)
          ?.querySelector<HTMLButtonElement>('button[aria-haspopup="menu"]')
          ?.focus();
    }, 0);
    return () => window.clearTimeout(timer);
  });

  const changePlugins = (change: (current: WorkInfoPlugin[]) => WorkInfoPlugin[]) => {
    changes.current++;
    setPlugins(change);
  };

  const replacePlugin = (next: WorkInfoPlugin) =>
    changePlugins((current) =>
      current.map((plugin) => (plugin.id === next.id ? next : plugin)),
    );

  const clearRowError = (id: number) =>
    setRowError((current) => (current?.id === id ? null : current));

  const add = async (file: File) => {
    setAdding(true);
    setAddError(null);
    try {
      const { plugin: added, replaced } = await uploadWorkInfoPlugin(file);
      // Insert the uploaded row optimistically so it survives a failed refresh.
      changePlugins((current) =>
        current.some((plugin) => plugin.id === added.id)
          ? current.map((plugin) => (plugin.id === added.id ? added : plugin))
          : [...current, added],
      );
      clearRowError(added.id);
      focusAfterRender.current = added.id;
      toast(addedToast(added, replaced));
      await load({ quiet: true });
    } catch (failure) {
      setAddError(text().addFailed(errorText(failure)));
    } finally {
      setAdding(false);
    }
  };

  const approve = async (digest: string) => {
    if (approving === null) return;
    const target = approving.plugin;
    setApproving({ ...approving, pending: true, error: null });
    try {
      const approved = await approveWorkInfoPlugin(target.id, digest);
      replacePlugin(approved);
      clearRowError(target.id);
      setApproving(null);
      focusAfterRender.current = target.id;
    } catch (failure) {
      if (isChangedMeanwhile(failure)) {
        setApproving((current) =>
          current === null ? null : { ...current, pending: false, changed: true },
        );
        void load();
        return;
      }
      setApproving((current) =>
        current === null
          ? null
          : {
              ...current,
              pending: false,
              error: text().approval.failed(errorText(failure)),
            },
      );
    }
  };

  const saveSettings = async (values: Record<string, string | null>) => {
    if (configuring === null) return;
    const target = configuring.plugin;
    setConfiguring({ ...configuring, pending: true, error: null });
    try {
      const saved = await setWorkInfoPluginSettings(target.id, values);
      replacePlugin(saved);
      setConfiguring(null);
      toast(text().settingsDialog.saved(pluginName(saved)));
    } catch (failure) {
      setConfiguring((current) =>
        current === null
          ? null
          : {
              ...current,
              pending: false,
              error: text().settingsDialog.saveFailed(errorText(failure)),
            },
      );
    }
  };

  const revoke = async (target: WorkInfoPlugin) => {
    setRowError(null);
    setWork({ id: target.id, kind: "revoke" });
    try {
      replacePlugin(await revokeWorkInfoPluginApproval(target.id));
      focusAfterRender.current = target.id;
    } catch (failure) {
      setRowError({
        id: target.id,
        message: text().rowFailed(pluginName(target), errorText(failure)),
      });
    } finally {
      setWork(null);
    }
  };

  const remove = async () => {
    if (deleting === null) return;
    const target = deleting.plugin;
    setDeleting({ ...deleting, pending: true, error: null });
    setWork({ id: target.id, kind: "delete" });
    try {
      await deleteWorkInfoPlugin(target.id);
      // Remove from the latest list because concurrent uploads may have refreshed it; focus the next row or Add plugin.
      changePlugins((current) => {
        const index = current.findIndex((plugin) => plugin.id === target.id);
        const remaining = current.filter((plugin) => plugin.id !== target.id);
        const next = remaining[Math.min(Math.max(index, 0), remaining.length - 1)];
        focusAfterRender.current = next === undefined ? "add" : next.id;
        return remaining;
      });
      setDeleting(null);
      clearRowError(target.id);
      toast(text().deleteDialog.deleted(pluginName(target)));
    } catch (failure) {
      setDeleting((current) =>
        current === null
          ? null
          : {
              ...current,
              pending: false,
              error: text().deleteDialog.failed(errorText(failure)),
            },
      );
    } finally {
      setWork(null);
    }
  };

  const renderRow = (plugin: WorkInfoPlugin) => (
    <WorkInfoPluginRow
      key={plugin.id}
      plugin={plugin}
      work={work?.id === plugin.id ? work.kind : null}
      error={rowError?.id === plugin.id ? rowError.message : null}
      rowRef={(element) => {
        if (element === null) rowRefs.current.delete(plugin.id);
        else rowRefs.current.set(plugin.id, element);
      }}
      onApprove={() =>
        setApproving({ plugin, pending: false, changed: false, error: null })
      }
      onSettings={() => setConfiguring({ plugin, pending: false, error: null })}
      onLastRun={() => setReading(plugin)}
      onRevoke={() => void revoke(plugin)}
      onDelete={() => setDeleting({ plugin, pending: false, error: null })}
    />
  );

  return (
    <PageSection
      title={text().heading}
      description={
        <>
          {text().description} <GuideLink />
        </>
      }
      actions={
        <AddButton
          buttonRef={addButton}
          adding={adding}
          blocked={loading || loadError !== null}
          onFile={(file) => void add(file)}
          onTooLarge={() => setAddError(text().tooLarge)}
        />
      }
    >
      {addError !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{addError}</AlertTitle>
        </Alert>
      )}
      <PluginList
        loading={loading}
        loadError={loadError}
        plugins={plugins}
        onRetry={() => {
          setLoading(true);
          void load();
        }}
        renderRow={renderRow}
      />
      {approving !== null && (
        <WorkInfoPluginApprovalDialog
          plugin={approving.plugin}
          pending={approving.pending}
          changed={approving.changed}
          error={approving.error}
          onClose={() => {
            if (!approving.pending) setApproving(null);
          }}
          onApprove={(digest) => void approve(digest)}
        />
      )}
      {configuring !== null && (
        <WorkInfoPluginSettingsDialog
          plugin={configuring.plugin}
          pending={configuring.pending}
          error={configuring.error}
          onClose={() => {
            if (!configuring.pending) setConfiguring(null);
          }}
          onSave={(values) => void saveSettings(values)}
        />
      )}
      {reading !== null && (
        <WorkInfoPluginRunDialog
          plugin={reading}
          onClose={() => {
            // Restore focus explicitly to More actions because the opening menu item has unmounted.
            focusAfterRender.current = { menu: reading.id };
            setReading(null);
          }}
        />
      )}
      {deleting !== null && (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open && !deleting.pending) setDeleting(null);
          }}
          title={text().deleteDialog.title(pluginName(deleting.plugin))}
          description={text().deleteDialog.description}
          actionLabel={
            deleting.pending ? text().deleteDialog.deleting : text().deleteDialog.submit
          }
          cancelLabel={t.common.cancel}
          pending={deleting.pending}
          onConfirm={() => void remove()}
        >
          {deleting.error !== null && (
            <Alert variant="destructive">
              <CircleAlert aria-hidden="true" />
              <AlertTitle>{deleting.error}</AlertTitle>
            </Alert>
          )}
        </ConfirmDialog>
      )}
    </PageSection>
  );
}
