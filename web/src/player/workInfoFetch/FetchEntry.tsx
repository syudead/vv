import { CloudDownload } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import type { Video } from "../../api/client";
import type { DetailMark } from "../../api/useVideoDetail";
import { listWorkInfoPlugins, type WorkInfoPlugin } from "../../api/workInfoPlugins";
import { t } from "../../i18n";
import ActionButton from "../ActionButton";
import type { ThumbnailCapture } from "../VideoFacts";
import { approvedPlugins } from "./fetchRules";
import FetchDialog from "./FetchDialog";

/**
 * useApprovedPlugins は作品情報プラグインの一覧を読み、承認済みのものを返す（R-11）。読んで
 * いる間と読めなかったときは空で、入口を置かない。読み直しは前の読みかけを捨てる。
 */
function useApprovedPlugins(): {
  plugins: WorkInfoPlugin[];
  reload: () => Promise<void>;
} {
  const [plugins, setPlugins] = useState<WorkInfoPlugin[]>([]);
  const latest = useRef<AbortController | null>(null);

  const reload = useCallback(async () => {
    latest.current?.abort();
    const controller = new AbortController();
    latest.current = controller;
    try {
      const list = await listWorkInfoPlugins(controller.signal);
      if (!controller.signal.aborted) setPlugins(approvedPlugins(list));
    } catch {
      // 一覧を読めなければ、取得の入口についてページは何も言わない（ui-design「Video page:
      // fetch entry」）。前に読めた一覧はそのまま使う。
    }
  }, []);

  useEffect(() => {
    void reload();
    return () => latest.current?.abort();
  }, [reload]);
  return { plugins, reload };
}

/**
 * FetchEntry は情報の行の操作群の「Fetch work info from a plugin」と、それが開く取得の
 * ダイアログである。
 * 所有者にだけ置かれ、承認済みのプラグインが一覧にあるときだけ入口を出す。押すと再生を止めて
 * ダイアログを開く。ダイアログを開いている間に入口の条件が外れても、閉じるまではダイアログを残す。
 */
export default function FetchEntry({
  video,
  capture,
  onApplied,
  onDialogChange,
}: {
  video: Video;
  /** プレイヤーが出ているときだけ渡す。開くときに再生を止める。 */
  capture?: ThumbnailCapture;
  onApplied: (video: Video, mark: DetailMark) => void;
  /** ダイアログを開いた・閉じた。開いている間、動画ページのキーを止める。 */
  onDialogChange?: (open: boolean) => void;
}) {
  const { plugins, reload } = useApprovedPlugins();
  const [open, setOpen] = useState(false);
  const dialogChange = useRef(onDialogChange);
  dialogChange.current = onDialogChange;

  useEffect(() => {
    dialogChange.current?.(open);
  }, [open]);
  useEffect(() => () => dialogChange.current?.(false), []);

  return (
    <>
      {plugins.length > 0 && (
        <ActionButton
          label={t.player.workInfoFetch.entry}
          onClick={() => {
            capture?.pause();
            setOpen(true);
          }}
        >
          <CloudDownload aria-hidden="true" />
        </ActionButton>
      )}
      {open && (
        <FetchDialog
          video={video}
          plugins={plugins}
          onReload={reload}
          onApplied={onApplied}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}
