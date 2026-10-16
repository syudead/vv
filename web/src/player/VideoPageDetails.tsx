import type { Video } from "../api/client";
import type { useRelatedVideos, useVideoDetail } from "../api/useVideoDetail";
import { Skeleton } from "../ui/shadcn/skeleton";
import AutoTagButton from "./AutoTagButton";
import type { usePageHandoff } from "./detailsEditor/handoff";
import GroupLine from "./GroupLine";
import PlaylistLine from "./PlaylistLine";
import SmartFolderLine from "./SmartFolderLine";
import { CreatingLine } from "./StatusOverlays";
import type { VideoPlaylist } from "./useVideoPlaylist";
import VideoFacts, { type ThumbnailCapture } from "./VideoFacts";
import VideoTags from "./VideoTags";
import VideoTitle from "./VideoTitle";
import VisibilitySwitch from "./VisibilitySwitch";

type Detail = ReturnType<typeof useVideoDetail>;
type Related = ReturnType<typeof useRelatedVideos>;

/**
 * VideoPageDetails は動画ページの左の列のプレイヤーの下（題名・タグ・公開・情報）である。
 * 動画を読む前は骨組みを出す。題名とタグは 1 つのまとまりで、その上にプレイリスト・
 * スマートフォルダ・グループの行を置く（ui-design.md「Video page tags」Placement）。
 */
export default function VideoPageDetails({
  loading,
  video,
  owner,
  playlist,
  backTo,
  trail,
  autoplay,
  capture,
  detail,
  related,
  handoff,
  onGroupChanged,
  onEditingThumbnail,
}: {
  loading: boolean;
  video: Video | undefined;
  owner: boolean;
  playlist: VideoPlaylist;
  backTo: string;
  trail: number[];
  /** 版の行で移った先で再生を始めるか。 */
  autoplay: boolean;
  capture: ThumbnailCapture | undefined;
  detail: Pick<Detail, "refresh" | "replace" | "setFavorite">;
  related: Pick<Related, "rename" | "rethumb">;
  handoff: ReturnType<typeof usePageHandoff>;
  /** グループの操作（外す・タグ化・解散）の後。 */
  onGroupChanged: () => void;
  onEditingThumbnail: (editing: boolean) => void;
}) {
  if (loading)
    return (
      <div aria-hidden="true" className="flex flex-col gap-3">
        <Skeleton className="h-8 w-2/3" />
        <Skeleton className="h-5 w-1/2" />
      </div>
    );
  if (video === undefined) return null;
  const { refresh, replace, setFavorite } = detail;
  // 取り直しの間と失敗したときは前の値のまま（specs/033-video-dates/ui-design.md
  // 「Refresh after edits」）。
  const reread = () => void refresh();
  const { playback } = playlist;
  return (
    <>
      <CreatingLine video={video} />
      <div className="flex flex-col gap-2">
        {playlist.context.status === "ready" && playback !== undefined && (
          <PlaylistLine
            playlistId={playlist.playlistId}
            name={playback.playlist.name}
            position={playback.position}
            count={playback.count}
            backTo={backTo}
          />
        )}
        {video.smartFolder !== undefined && (
          <SmartFolderLine folder={video.smartFolder} backTo={backTo} />
        )}
        {video.group !== undefined && (
          <GroupLine
            group={video.group}
            videoId={video.id}
            owner={owner}
            backTo={backTo}
            // 行・メンバーの並び・前後が、残ったグループ（id のいちばん小さいもの）に
            // 従うか、ふつうの動画の形に戻る。
            onChanged={onGroupChanged}
          />
        )}
        <VideoTitle
          // 別の動画へ移ったら、編集中の入力と失敗の行を持ち越さない。
          key={`title:${String(video.id)}`}
          video={video}
          owner={owner}
          onSaved={(saved, mark) => {
            replace(saved, mark, "keep");
            // グループの並びにあるこの動画の題名も、画面の題名とそろえる。
            related.rename(saved);
          }}
          onStale={reread}
        />
        {owner && <OwnerControls video={video} reread={reread} />}
      </div>
      {/* ゲストの応答には location が無いので、開く・コピーの操作は出ない。 */}
      <VideoFacts
        // 別の動画へ移ったら、送信中の指定と失敗の行を持ち越さない。
        key={`facts:${String(video.id)}`}
        video={video}
        owner={owner}
        capture={capture}
        onChanged={(saved, mark) => {
          replace(saved, mark, "keep");
          // グループの並びにあるこの動画のサムネイルと題名も、画面のものとそろえる
          // （作品情報の取得の反映は題名を変える）。
          related.rethumb(saved);
          related.rename(saved);
        }}
        onStale={reread}
        onEditingChange={onEditingThumbnail}
        // 作品情報の編集画面の入口。戻り先（from・trail）を載せて開き、編集画面から
        // 戻ったときはそこへフォーカスを戻す（specs/047-video-work-info/ui-design.md
        // 「Video page: entry」）。
        details={{
          linkState: { from: backTo, trail },
          focusEntry: handoff.focusEntry(video.id),
        }}
        // 付け外しの後は動画を取り直し、取り直した `favorite` で塗りを確かめる
        // （specs/035-favorites/ui-design.md「Video page」）。
        onFavorite={setFavorite}
        versions={{
          navigation: { backTo, autoplay },
          // 外した動画は固有の作品情報に戻るので、束の作品情報を残さず取り直す。
          onReplace: (removed, mark) => replace(removed, mark, "refetch"),
          onRefresh: reread,
        }}
      />
    </>
  );
}

/** OwnerControls は所有者だけの題名の下の操作である。題名 → タグ → 公開の順。 */
function OwnerControls({
  video,
  reread,
}: {
  video: Video;
  /** 動画を取り直す。 */
  reread: () => void;
}) {
  return (
    <>
      <VideoTags
        // VideoPage 自身が動画ごとに作り直されず（同じ /videos/:id
        // ルートのまま次の動画へ移ることがある）使い回されるため、
        // VideoTags を videoId で作り直し、前の動画の重ねた
        // 付け外し（appliedRef）を持ち越さない（Devin の指摘1）。
        key={`tags:${String(video.id)}`}
        videoId={video.id}
        tags={video.tags}
        onStaleVideo={reread}
        // 更新日時が進むので取り直す。
        onChanged={reread}
      />
      {/* タグの並びのすぐ下で、この動画のタグを判定モデルに聞く
          （docs/design-docs/auto-tagging.md）。 */}
      <AutoTagButton key={`auto-tag:${String(video.id)}`} videoId={video.id} />
      {/* 題名 → タグ → 公開の順（ui-design.md「Visibility toggle」）。
          別の動画へ移ったら失敗の行を持ち越さないよう、id で作り直す。 */}
      <VisibilitySwitch
        key={`visibility:${String(video.id)}`}
        videoId={video.id}
        isPublic={video.public}
        onChanged={reread}
      />
    </>
  );
}
