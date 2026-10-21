import { useRef, useState } from "react";

import { type Credit, isCreditGone, type Person } from "../../api/credits";
import {
  type FaceAppearance,
  type FaceAppearanceMove,
  faceCropPath,
  isAppearanceGone,
  moveFaceAppearances,
} from "../../api/personFaces";
import { errorText, t, type UiText } from "../../i18n";
import { DialogError } from "../../tags/DialogError";
import { MergeTargetField } from "../../tags/MergeParts";
import CreditCommand, { type CreditChoice } from "../../ui/CreditCommand";
import { FormDialog } from "../../ui/patterns/form-dialog";
import { Spinner } from "../../ui/shadcn/spinner";
import { PersonFace } from "../../ui/PersonFace";
import { creditDisplayName } from "../displayName";
import { useCreditTargetSearch } from "../useCreditTargetSearch";
import { SimilarPeopleList, suggestsByFace, useSimilarPeople } from "./SimilarPeopleList";

const text = t.credits.personPage.moveDialog;

/**
 * `Move to person`: picks where the selected faces belong, by face before a name is typed, and moves them there with
 * their credits.
 */
export default function MoveToPersonDialog({
  person,
  faces,
  onClose,
  onMoved,
  onGone,
}: {
  person: Person;
  faces: readonly FaceAppearance[];
  onClose: () => void;
  onMoved: (moved: FaceAppearanceMove) => void;
  onGone: () => void;
}) {
  const cancel = useRef<HTMLButtonElement>(null);
  const [value, setValue] = useState("");
  const [target, setTarget] = useState<Credit | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  const ids = faces.map((face) => face.appearanceId);
  const similar = useSimilarPeople(person.id, ids);
  const suggesting = suggestsByFace(value, similar);
  const { shown, options, exactOption, searching, searchError } = useCreditTargetSearch(
    "person",
    person.id,
    value,
  );

  function selectTarget(option: CreditChoice) {
    const found = shown.find((credit) => String(credit.id) === option.id);
    if (found === undefined) return;
    setTarget(found);
    setValue(found.name);
    setError(null);
  }

  async function submit() {
    if (target === null || pending) return;
    setPending(true);
    setError(null);
    try {
      onMoved(await moveFaceAppearances(person.id, ids, target.id));
    } catch (failure) {
      setPending(false);
      if (isCreditGone(failure)) onGone();
      else if (isAppearanceGone(failure)) setError(text.facesGone);
      else setError(text.failed(errorText(failure)));
    }
  }

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={text.title(faces.length)}
      description={text.description}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? text.submitting : text.submit}
        </>
      }
      pending={pending}
      submitDisabled={target === null || pending}
      cancelRef={cancel}
      initialFocus={cancel}
      size="wide"
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs text-muted-foreground">{text.moving}</span>
        {faces.map((face) => (
          <span key={face.appearanceId} className="size-12">
            <PersonFace
              shape="scene"
              name={face.title}
              src={face.hasCrop ? faceCropPath(face.appearanceId) : undefined}
            />
          </span>
        ))}
      </div>
      <MergeTargetField
        Command={CreditCommand}
        label={text.search}
        value={value}
        onValueChange={(next) => {
          setValue(next);
          setTarget(null);
          setError(null);
        }}
        choices={suggesting ? [] : options}
        exactChoice={exactOption}
        onSelect={selectTarget}
        chosenId={target === null ? undefined : String(target.id)}
        searching={searching && !suggesting}
        searchError={
          suggesting || searchError === null ? null : text.searchFailed(searchError)
        }
        emptyText={suggesting ? undefined : text.noCandidates}
      />
      {suggesting && (
        <SimilarPeopleList
          heading={t.credits.similarPeople.likeFaces}
          similar={similar}
          selectedId={target?.id}
          onSelect={(picked) => {
            setTarget(picked);
            setError(null);
          }}
        />
      )}
      {target !== null && (
        <p className="text-sm text-muted-foreground">
          {text.summary(faces.length, creditDisplayName(target))}
        </p>
      )}
      {error !== null && <DialogError message={error} />}
    </FormDialog>
  );
}
