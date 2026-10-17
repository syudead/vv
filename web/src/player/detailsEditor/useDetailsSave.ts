import { type ReactNode, type RefObject, useEffect, useRef, useState } from "react";

import { isAborted, RequestFailed, type Video } from "../../api/client";
import { createCredit } from "../../api/credits";
import { setVideoDetails } from "../../api/workInfo";
import { errorText, t } from "../../i18n";
import { type Draft, newEntries, toDetailsInput, withCreated } from "./draft";
import type { FieldProblem } from "./fields";

/** SaveFailure は帯の下の失敗の行の中身である。`field` は入力に写せなかった経路。 */
export interface SaveFailure {
  text: ReactNode;
  field?: string;
}

/**
 * useDetailsSave は編集画面の `Save` である。
 *
 * 1. 下書きのまだ作っていない名前を、人物の役割の順、団体の役割の順に 1 件ずつ
 *    `POST /api/people`（`/api/organizations`）で作る。作れたものはその場で下書きの id にし、
 *    次の `Save` で作り直さない。失敗したらそこで止め、失敗の行を出す。
 * 2. 1 回の `PUT /api/videos/{id}/details` で表示名と作品情報の全体を送る。`200` なら
 *    `onSaved` に応答の動画を渡す。`field` のある `400`・`404` はその入力に理由を出し、
 *    入力が無い経路は失敗の行に経路と一緒に出す。`video_not_found` は `onGone`。
 *    ほかは失敗の行で、先に作ったものがあればその数を添える。
 *
 * 送信中は次の `Save` を受けない。画面を離れたら送信中の要求を打ち切る。
 */
export function useDetailsSave({
  videoId,
  draftRef,
  setDraft,
  onSaved,
  onGone,
}: {
  videoId: number;
  /** 最新の下書き。 */
  draftRef: RefObject<Draft>;
  setDraft: (draft: Draft) => void;
  onSaved: (video: Video) => void;
  onGone: () => void;
}): {
  saving: boolean;
  failure: SaveFailure | null;
  problem: FieldProblem | null;
  save: () => void;
} {
  const [saving, setSaving] = useState(false);
  const [failure, setFailure] = useState<SaveFailure | null>(null);
  const [problem, setProblem] = useState<FieldProblem | null>(null);
  const busy = useRef(false);
  const controller = useRef<AbortController | null>(null);

  useEffect(() => () => controller.current?.abort(), []);

  async function run(signal: AbortSignal): Promise<void> {
    const e = t.player.detailsEditor;
    let draft = draftRef.current;
    let created = 0;
    for (const entry of newEntries(draft)) {
      try {
        const credit = await createCredit(entry.kind, { name: entry.name }, signal);
        draft = withCreated(draft, entry.kind, entry.newKey, credit.id);
        created += 1;
        setDraft(draft);
      } catch (error) {
        if (signal.aborted || isAborted(error)) return;
        setFailure({ text: e.saveFailed(errorText(error)) });
        return;
      }
    }
    const { body, targets } = toDetailsInput(draft);
    let video: Video;
    try {
      video = await setVideoDetails(videoId, body, signal);
    } catch (error) {
      if (signal.aborted || isAborted(error)) return;
      if (error instanceof RequestFailed && error.reason === "video_not_found") {
        onGone();
        return;
      }
      const reason = errorText(error);
      const field =
        error instanceof RequestFailed && (error.status === 400 || error.status === 404)
          ? error.field
          : undefined;
      if (field !== undefined) {
        const control = targets.get(field);
        if (control !== undefined) setProblem({ control, text: reason });
        else setFailure({ text: e.saveFailed(reason), field });
        return;
      }
      setFailure({
        text: created > 0 ? e.createdNotSaved(created, reason) : e.saveFailed(reason),
      });
      return;
    }
    onSaved(video);
  }

  const save = () => {
    if (busy.current) return;
    busy.current = true;
    const mine = new AbortController();
    controller.current = mine;
    setSaving(true);
    setFailure(null);
    setProblem(null);
    void run(mine.signal).finally(() => {
      busy.current = false;
      if (!mine.signal.aborted) setSaving(false);
    });
  };

  return { saving, failure, problem, save };
}
