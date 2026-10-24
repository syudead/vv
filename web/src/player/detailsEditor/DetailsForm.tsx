import { type Ref, useRef, useState } from "react";

import type { Video } from "../../api/client";
import { t } from "../../i18n";
import { FormPage } from "../../ui/patterns/form-page";
import BasicsPanel from "./BasicsPanel";
import CreditsPanel from "./CreditsPanel";
import { type Draft, draftChanged, draftFromVideo } from "./draft";
import EditorAside from "./EditorAside";
import { EditorBand, FailureLine, failureText } from "./EditorBand";
import { useEditorForm } from "./fields";
import { useDetailsSave } from "./useDetailsSave";

export default function DetailsForm({
  video,
  backTo,
  onLeave,
  onSaved,
  onGone,
  onReload,
  onChangedChange,
  onSavingChange,
  cancelRef,
  backRef,
}: {
  video: Video;
  backTo: string;
  onLeave: () => void;
  onSaved: (video: Video) => void;
  onGone: () => void;
  onReload: () => void;
  onChangedChange: (changed: boolean) => void;
  onSavingChange: (saving: boolean) => void;
  cancelRef?: Ref<HTMLButtonElement>;
  backRef?: Ref<HTMLAnchorElement>;
}) {
  const [loaded] = useState(() => draftFromVideo(video));
  const [draft, setDraft] = useState<Draft>(loaded);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const { saving, failure, problem, save } = useDetailsSave({
    video,
    draftRef,
    setDraft,
    onSaved,
    onGone,
    onReload,
  });
  const nameRef = useEditorForm({
    changed: draftChanged(draft, loaded),
    saving,
    problem,
    onChangedChange,
    onSavingChange,
  });

  const panelProps = {
    draft,
    update: (change: (current: Draft) => Draft) => setDraft((current) => change(current)),
    saving,
    problem,
    save,
  };

  return (
    <FormPage
      title={t.player.detailsEditor.title}
      detail={video.title}
      actions={
        <EditorBand
          saving={saving}
          saveDisabled={false}
          onCancel={onLeave}
          onSave={save}
          cancelRef={cancelRef}
        />
      }
      notice={
        failure === null ? undefined : <FailureLine>{failureText(failure)}</FailureLine>
      }
      asideLabel={t.player.detailsEditor.video}
      aside={
        <EditorAside video={video} backTo={backTo} onBack={onLeave} backRef={backRef} />
      }
      panels={[
        {
          key: "basics",
          legend: t.player.detailsEditor.basics,
          content: (
            <BasicsPanel
              {...panelProps}
              nameLabel={
                video.virtual
                  ? t.player.detailsEditor.titleField
                  : t.player.detailsEditor.name
              }
              namePlaceholder={video.fileTitle ?? video.title}
              nameRef={nameRef}
            />
          ),
        },
        {
          key: "credits",
          legend: t.player.detailsEditor.credits,
          content: <CreditsPanel {...panelProps} />,
        },
      ]}
    />
  );
}
