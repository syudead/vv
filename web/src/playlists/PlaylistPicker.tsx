import { ListVideo, Plus } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";

import { isAborted, RequestFailed } from "../api/client";
import {
  addToPlaylist,
  createPlaylist,
  listAllPlaylists,
  type PlaylistSummary,
} from "../api/playlists";
import { maxVideoTagsSelection } from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { overLimitMessage } from "../library/selectionErrors";
import { Button } from "../ui/shadcn/button";
import { Command, CommandInput, CommandItem, CommandList } from "../ui/shadcn/command";
import { PopoverContent } from "../ui/shadcn/popover";
import { Spinner } from "../ui/shadcn/spinner";
import { useToast } from "../ui/Toast";
import { matchPlaylists, nameTooLong, resultText } from "./pickerRules";

const createValue = "__create__";

type Load =
  | { status: "loading" }
  | { status: "failed" }
  | { status: "ready"; playlists: PlaylistSummary[] };

/** Pending は返事を待っている行（選んだプレイリストか作成の行）である。 */
type Pending = { kind: "playlist"; id: number } | { kind: "create" } | null;

/**
 * PlaylistPicker は「Add to playlist」のポップオーバーの中身である
 * （specs/044-playlists/ui-design.md「Add to playlist」）。カード・行・動画ページ・選択バーの
 * 4 か所が、それぞれの `Popover` の中に置く。
 *
 * - 開くたびに `GET /api/playlists` を `nextCursor` が尽きるまで読み（listAllPlaylists）、打った文字を
 *   名前に含むものを API の順に出す。
 *   打った名前と同じ名前のプレイリストが無ければ、先頭に「Create "{name}"」の行を出す。
 * - 行を選ぶと `videoIds` と `groupIds` を `POST /api/playlists/{id}/items` で送る。作成の行は
 *   `POST /api/playlists` の後に、その新しいプレイリストへ足す。足すのに失敗した後に同じ名前で
 *   作成の行をもう一度選ぶと、開いている間に作ったその名前のプレイリストへ足し直す（別の名前を
 *   挟んでも同じ）。そのプレイリストが消えていた（404）なら覚えを捨て、次は作り直す。
 * - 結果はトースト 1 つで伝えて閉じる。失敗は開いたまま、入力の下に理由を出す。
 * - Esc はポップオーバーだけを閉じる。一覧の Esc（選択の解除）には「使った」と伝える。
 */
export default function PlaylistPicker({
  open,
  onOpenChange,
  videoIds,
  groupIds,
  side = "bottom",
  align = "end",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  videoIds: readonly number[];
  groupIds: readonly number[];
  side?: "top" | "bottom";
  align?: "start" | "end";
}) {
  const toast = useToast();
  const w = t.playlists.add;
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [value, setValue] = useState("");
  const [load, setLoad] = useState<Load>({ status: "loading" });
  const [pending, setPending] = useState<Pending>(null);
  const [failure, setFailure] = useState<UiText | null>(null);
  const loadAbort = useRef<AbortController | null>(null);
  // 開くたび・閉じるたびに進める。返事が来たときに数が変わっていれば、その送信は閉じた
  // ピッカーのもので、開き直したピッカーを閉じたり理由を書き換えたりしてはいけない。
  const session = useRef(0);
  // 作成の行で作ったプレイリストを名前ごとに覚える。同じ名前でもう一度押したら、作り直さずに
  // これへ足す。名前は重ねてよいので、作り直すと空のプレイリストが残る。開くたびに新しい Map に
  // 替え、送信は始めたときの Map にだけ書くので、閉じたピッカーの返事が開き直した後の覚えを
  // 書き換えることはない。
  const created = useRef(new Map<string, PlaylistSummary>());

  const read = useCallback((quiet: boolean) => {
    loadAbort.current?.abort();
    const controller = new AbortController();
    loadAbort.current = controller;
    if (!quiet) setLoad({ status: "loading" });
    listAllPlaylists(controller.signal).then(
      (playlists) => {
        if (!controller.signal.aborted) setLoad({ status: "ready", playlists });
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted || quiet) return;
        setLoad({ status: "failed" });
      },
    );
  }, []);

  // 開くたびに読み直す（別の画面で作ったプレイリストもそこにある）。
  useEffect(() => {
    if (!open) return;
    session.current += 1;
    setValue("");
    setFailure(null);
    setPending(null);
    created.current = new Map();
    read(false);
    return () => {
      session.current += 1;
      loadAbort.current?.abort();
    };
  }, [open, read]);

  const trimmed = value.trim();
  const tooLong = nameTooLong(trimmed);
  const playlists = load.status === "ready" ? load.playlists : [];
  const matches = matchPlaylists(playlists, trimmed);
  const exact = playlists.some((playlist) => playlist.name === trimmed);
  const showCreate = load.status !== "loading" && trimmed !== "" && !exact;
  const blocked = pending !== null || tooLong;

  /**
   * createOnce は名前 name のプレイリストを作り、memory に覚える。memory に既にあればそれを返す。
   */
  async function createOnce(
    memory: Map<string, PlaylistSummary>,
    name: string,
  ): Promise<PlaylistSummary> {
    const earlier = memory.get(name);
    if (earlier !== undefined) return earlier;
    const playlist = await createPlaylist(name);
    memory.set(name, playlist);
    return playlist;
  }

  // 送るものは、この描画の props（videoIds・groupIds）で決まる。送っている間に選択が
  // 変わっても、この呼び出しの値は変わらない。
  async function send(target: { id: number; name: string } | { create: string }) {
    const mine = session.current;
    const current = () => session.current === mine;
    const memory = created.current;
    setFailure(null);
    // 開いている間に選択が増えて上限を超えたら、送らずに理由を出す（「Favorite」と同じ）。
    if (videoIds.length + groupIds.length > maxVideoTagsSelection) {
      setFailure(overLimitMessage());
      return;
    }
    setPending("id" in target ? { kind: "playlist", id: target.id } : { kind: "create" });
    let playlist: { id: number; name: string } | null = null;
    try {
      playlist = "id" in target ? target : await createOnce(memory, target.create);
      const result = await addToPlaylist(playlist.id, videoIds, groupIds);
      // 足したことは、ピッカーを閉じた後でもトーストで伝える。
      toast(resultText(result, playlist.name));
      if (current()) onOpenChange(false);
    } catch (error: unknown) {
      // 作ったプレイリストが別のタブで消されていれば、次の作成の行で作り直す。
      if (error instanceof RequestFailed && error.status === 404 && playlist !== null) {
        const gone = playlist.id;
        for (const [name, kept] of memory) {
          if (kept.id === gone) memory.delete(name);
        }
      }
      if (!current()) return;
      setFailure(w.failed(errorText(error)));
      // 途中で消されたプレイリストを除き、作った分を足すため、一覧を静かに読み直す。
      read(true);
    } finally {
      if (current()) setPending(null);
    }
  }

  const rowIcon = (busy: boolean, icon: ReactNode) =>
    busy ? <Spinner className="text-muted-foreground" /> : icon;

  let body: ReactNode = null;
  if (load.status === "loading") {
    body = <p className="px-2 py-1.5 text-sm text-muted-foreground">{w.loading}</p>;
  } else if (load.status === "failed") {
    body = (
      <div className="flex items-center justify-between gap-2 px-2 py-1">
        <p className="text-sm text-muted-foreground">{w.loadFailed}</p>
        <Button variant="ghost" size="sm" onClick={() => read(false)}>
          {t.common.retry}
        </Button>
      </div>
    );
  } else if (playlists.length === 0 && trimmed === "") {
    body = <p className="px-2 py-1.5 text-sm text-muted-foreground">{w.none}</p>;
  }

  const hasRows = showCreate || (load.status === "ready" && matches.length > 0);

  return (
    <PopoverContent
      side={side}
      align={align}
      aria-label={w.label}
      onOpenAutoFocus={(event) => {
        event.preventDefault();
        inputRef.current?.focus();
      }}
      onEscapeKeyDown={(event) => {
        event.preventDefault();
        onOpenChange(false);
      }}
    >
      <Command label={w.search} shouldFilter={false} className="bg-transparent">
        <CommandInput
          ref={inputRef}
          icon={<ListVideo aria-hidden="true" />}
          value={value}
          onValueChange={setValue}
          placeholder={w.search}
          aria-label={w.search}
          aria-busy={pending !== null || undefined}
        />
        {/* 理由は入力のすぐ下に出す（ui-design.md「Add to playlist」）。 */}
        {tooLong && (
          <p role="alert" className="py-1 text-xs text-destructive">
            {w.nameTooLong}
          </p>
        )}
        {failure !== null && (
          <p role="alert" className="py-1 text-xs text-destructive">
            {failure}
          </p>
        )}
        {hasRows && (
          <CommandList label={w.list}>
            {showCreate && (
              <CommandItem
                value={createValue}
                disabled={blocked}
                onSelect={() => void send({ create: trimmed })}
              >
                {rowIcon(
                  pending?.kind === "create",
                  <Plus className="text-muted-foreground" aria-hidden="true" />,
                )}
                <span className="truncate">{w.create(trimmed)}</span>
              </CommandItem>
            )}
            {load.status === "ready" &&
              matches.map((playlist) => (
                <CommandItem
                  key={playlist.id}
                  value={String(playlist.id)}
                  disabled={blocked}
                  onSelect={() => void send(playlist)}
                >
                  {rowIcon(
                    pending?.kind === "playlist" && pending.id === playlist.id,
                    <ListVideo className="text-muted-foreground" aria-hidden="true" />,
                  )}
                  <span className="truncate">{playlist.name}</span>
                </CommandItem>
              ))}
          </CommandList>
        )}
        {body}
      </Command>
    </PopoverContent>
  );
}
