import type { RefObject } from "react";

import type { RequestFailed } from "../../../api/client";
import type { Person } from "../../../api/credits";
import { errorText } from "../../../i18n";
import type { FieldProblem } from "../../../player/detailsEditor/fields";
import type { EditorSave } from "../../../player/detailsEditor/useDetailsSave";
import { problemOf } from "../../creditEditFields";
import { useCreditSave } from "../../edit/useCreditSave";
import {
  buildPersonPatch,
  type PersonDraft,
  profileControl,
  type SentTargets,
} from "./personDraft";

/** The page's own controls first, so a `names` path lands on a row; IDs use the shared mapping. */
function placeProblem(error: RequestFailed, sent: SentTargets): FieldProblem | null {
  const control =
    error.field === undefined ? undefined : profileControl(error.field, sent);
  return control === undefined
    ? problemOf(error, sent)
    : { control, text: errorText(error) };
}

export function usePersonSave(args: {
  id: number;
  person: Person;
  loaded: PersonDraft;
  draftRef: RefObject<PersonDraft>;
  onSaved: (person: Person) => void;
  onGone: () => void;
}): EditorSave {
  return useCreditSave({
    kind: "person",
    id: args.id,
    credit: args.person,
    loaded: args.loaded,
    draftRef: args.draftRef,
    build: buildPersonPatch,
    place: placeProblem,
    onSaved: args.onSaved,
    onGone: args.onGone,
  });
}
