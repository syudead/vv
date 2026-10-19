import { CircleAlert } from "lucide-react";
import { type RefObject, useCallback, useEffect, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import {
  approveWorkInfoPlugin,
  deleteWorkInfoPlugin,
  listWorkInfoPlugins,
  revokeWorkInfoPluginApproval,
  setWorkInfoPluginAutoLookup,
  setWorkInfoPluginSettings,
  uploadWorkInfoPlugin,
  type WorkInfoPlugin,
  type WorkInfoPluginKind,
} from "../api/workInfoPlugins";
import { errorText, t, type UiText } from "../i18n";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { useToast } from "../ui/Toast";
import { useSectionAnchor } from "./useSectionAnchor";
import WorkInfoPluginApprovalDialog from "./WorkInfoPluginApprovalDialog";
import WorkInfoPluginRow, { type RowWork } from "./WorkInfoPluginRow";
import WorkInfoPluginRunDialog from "./WorkInfoPluginRunDialog";
import { pluginName } from "./workInfoPluginRows";
import {
  AddButton,
  GuideLink,
  PluginList,
  sectionText,
} from "./WorkInfoPluginSectionParts";
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

/** Focus the row's first button, its menu trigger, or a section's Add plugin after rendering. */
type FocusTarget = number | { menu: number } | { add: WorkInfoPluginKind };

const KINDS = ["work", "person"] as const satisfies readonly WorkInfoPluginKind[];

const ANCHORS = {
  work: "work-info-plugins",
  person: "person-plugins",
} as const satisfies Record<WorkInfoPluginKind, string>;

/** The manifest decides the kind, so name the other section when the row lands there. */
function addedToast(
  plugin: WorkInfoPlugin,
  replaced: boolean,
  section: WorkInfoPluginKind,
): UiText {
  const name = pluginName(plugin);
  if (plugin.state === "invalid") return text().addedUnusable(name);
  if (plugin.kind !== section)
    return replaced
      ? text().replacedUnder[plugin.kind](name)
      : text().addedUnder[plugin.kind](name);
  return replaced ? text().replaced(name) : text().added(name);
}

function withoutKey(record: Record<number, boolean>, id: number) {
  const { [id]: _removed, ...rest } = record;
  return rest;
}

function isChangedMeanwhile(failure: unknown): boolean {
  return (
    failure instanceof RequestFailed &&
    (failure.code === "plugin_changed" || failure.code === "plugin_unusable")
  );
}

/** The work-info and person sections read one list and share the row, prompt and menu. */
export default function WorkInfoPluginsSection() {
  const toast = useToast();
  const anchors = {
    work: useSectionAnchor(ANCHORS.work),
    person: useSectionAnchor(ANCHORS.person),
  };
  const [plugins, setPlugins] = useState<WorkInfoPlugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [adding, setAdding] = useState<WorkInfoPluginKind | null>(null);
  const [addError, setAddError] = useState<{
    kind: WorkInfoPluginKind;
    message: UiText;
  } | null>(null);
  const [switching, setSwitching] = useState<Record<number, boolean>>({});
  const [work, setWork] = useState<{ id: number; kind: RowWork } | null>(null);
  const [rowError, setRowError] = useState<{ id: number; message: UiText } | null>(null);
  const [approving, setApproving] = useState<Approving | null>(null);
  const [configuring, setConfiguring] = useState<Editing | null>(null);
  const [deleting, setDeleting] = useState<Editing | null>(null);
  const [reading, setReading] = useState<WorkInfoPlugin | null>(null);
  const addButtons: Record<WorkInfoPluginKind, RefObject<HTMLButtonElement | null>> = {
    work: useRef<HTMLButtonElement>(null),
    person: useRef<HTMLButtonElement>(null),
  };
  const switchesSent = useRef(new Set<number>());
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
      if (typeof target === "number")
        rowRefs.current.get(target)?.querySelector<HTMLButtonElement>("button")?.focus();
      else if ("add" in target) addButtons[target.add].current?.focus();
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

  const add = async (file: File, section: WorkInfoPluginKind) => {
    setAdding(section);
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
      toast(addedToast(added, replaced, section));
      await load({ quiet: true });
    } catch (failure) {
      setAddError({ kind: section, message: text().addFailed(errorText(failure)) });
    } finally {
      setAdding(null);
    }
  };

  const setAutoLookup = async (target: WorkInfoPlugin, enabled: boolean) => {
    if (switchesSent.current.has(target.id)) return;
    switchesSent.current.add(target.id);
    clearRowError(target.id);
    setSwitching((current) => ({ ...current, [target.id]: enabled }));
    try {
      replacePlugin(await setWorkInfoPluginAutoLookup(target.id, enabled));
    } catch (failure) {
      setRowError({
        id: target.id,
        message: text().rowFailed(pluginName(target), errorText(failure)),
      });
    } finally {
      switchesSent.current.delete(target.id);
      setSwitching((current) => withoutKey(current, target.id));
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
      // Remove from the latest list because concurrent uploads may have refreshed it; focus the
      // next row of the same section or that section's Add plugin.
      changePlugins((current) => {
        const section = current.filter((plugin) => plugin.kind === target.kind);
        const index = section.findIndex((plugin) => plugin.id === target.id);
        const remaining = section.filter((plugin) => plugin.id !== target.id);
        const next = remaining[Math.min(Math.max(index, 0), remaining.length - 1)];
        focusAfterRender.current = next === undefined ? { add: target.kind } : next.id;
        return current.filter((plugin) => plugin.id !== target.id);
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
      lookup={
        plugin.kind === "person" && plugin.state === "approved"
          ? {
              saving: switching[plugin.id] ?? null,
              onChange: (enabled) => void setAutoLookup(plugin, enabled),
            }
          : null
      }
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

  const renderSection = (kind: WorkInfoPluginKind) => {
    const own = sectionText(kind);
    return (
      <div key={kind} ref={anchors[kind]} id={ANCHORS[kind]}>
        <PageSection
          title={own.heading}
          description={
            <>
              {own.description} <GuideLink />
            </>
          }
          actions={
            <AddButton
              kind={kind}
              buttonRef={addButtons[kind]}
              adding={adding === kind}
              blocked={loading || loadError !== null || adding !== null}
              onFile={(file) => void add(file, kind)}
              onTooLarge={() => setAddError({ kind, message: text().tooLarge })}
            />
          }
        >
          {addError?.kind === kind && (
            <Alert variant="destructive">
              <CircleAlert aria-hidden="true" />
              <AlertTitle>{addError.message}</AlertTitle>
            </Alert>
          )}
          <PluginList
            kind={kind}
            loading={loading}
            loadError={loadError}
            plugins={plugins.filter((plugin) => plugin.kind === kind)}
            onRetry={() => {
              setLoading(true);
              void load();
            }}
            renderRow={renderRow}
          />
        </PageSection>
      </div>
    );
  };

  return (
    <>
      {KINDS.map(renderSection)}
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
    </>
  );
}
