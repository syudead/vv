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

/** Approving は開いている承認の確認である。changed は差し替えか使えなくなったことの知らせ。 */
interface Approving {
  plugin: WorkInfoPlugin;
  pending: boolean;
  changed: boolean;
  error: UiText | null;
}

/** Editing は開いている設定のダイアログか削除の確認である。 */
interface Editing {
  plugin: WorkInfoPlugin;
  pending: boolean;
  error: UiText | null;
}

/**
 * FocusTarget は描いたあとにフォーカスを移す先である。行の id（その行の最初のボタン）、
 * `{ menu: id }`（その行の「More actions」）か「Add plugin」。
 */
type FocusTarget = number | { menu: number } | "add";

/** addedToast は追加の応答の状態と状態コードから、結果のトーストの文言を選ぶ。 */
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

/**
 * WorkInfoPluginsSection は設定の「Work-info plugins」節である。所有者が `.wasm` のプラグインを
 * 足し、状態を見て、承認・取り消し・削除し、設定の値を入れる。
 * 設定画面は所有者だけが開けるので、この節も所有者にだけ出る（受け入れ条件 7）。
 *
 * 節は `/settings/plugins#work-info-plugins` の行き先で、動画ページの取得ダイアログの「Open Settings」が
 * ここへ移る。
 */
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
  // loads は始めた読み込みの数、changes は操作の応答で行を変えた数である。
  const loads = useRef(0);
  const changes = useRef(0);

  /**
   * load は一覧を読み直す。後から始めた読み込みがあれば、その結果に任せて何もしない。読む
   * あいだに操作の応答が行を変えたなら、その一覧は操作の前の行を持ちうるので読み直す
   * （遅れて届いた一覧が取り消しや削除の結果を巻き戻さないように）。quiet は失敗を節の
   * 誤りにせず、今の一覧を残す。
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

  // ダイアログが閉じたときの Radix のフォーカスの戻しより後に移すため、描いたあと次の
  // タスクで移す。
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

  /** changePlugins は操作の応答で一覧を変え、読みかけの一覧を古いものにする。 */
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
      // 足した行は応答から先に一覧へ入れる。読み直しが失敗しても、足した行は見えたまま残る。
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
      // 削除を待つあいだに追加が終わり、一覧が読み直されていることがある。最新の一覧から
      // 対象だけを除き、次の行（無ければ「Add plugin」）へフォーカスを移す。
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
            // 開いた項目はメニューと一緒に消えたので、FormDialog の「前のフォーカスへ戻す」には
            // 頼らず、その行の「More actions」へ戻す（web/registry/rules/patterns.md）。
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
