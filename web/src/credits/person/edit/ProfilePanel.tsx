import { t, type UiText } from "../../../i18n";
import {
  controlId,
  type FieldProblem,
  invalidProps,
  Problem,
  saveOnEnter,
} from "../../../player/detailsEditor/fields";
import { Field, FieldDescription, FieldLabel } from "../../../ui/shadcn/field";
import { Input } from "../../../ui/shadcn/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../../../ui/shadcn/select";
import { Textarea } from "../../../ui/shadcn/textarea";
import {
  digitsOnly,
  type NumberField,
  numberFields,
  type PersonDraft,
  type ProfileField,
} from "./personDraft";

export interface PersonPanelProps {
  draft: PersonDraft;
  update: (change: (draft: PersonDraft) => PersonDraft) => void;
  /** Keep inputs read-only while saving. */
  saving: boolean;
  problem: FieldProblem | null;
  save: () => void;
}

/** Select values cannot be empty, so Not set needs a sentinel. */
const notSet = "none";

const text = t.credits.personEdit;

function isNumberField(field: ProfileField): field is NumberField {
  return (numberFields as readonly string[]).includes(field);
}

function setField(panel: PersonPanelProps, field: ProfileField, value: string) {
  panel.update((current) => ({
    ...current,
    profile: { ...current.profile, [field]: value },
  }));
}

function ProfileInput({
  panel,
  field,
  label,
  hint,
}: {
  panel: PersonPanelProps;
  field: ProfileField;
  label: UiText;
  hint?: UiText;
}) {
  const control = `profile-${field}`;
  const numeric = isNumberField(field);
  return (
    <Field>
      <FieldLabel htmlFor={controlId(control)}>{label}</FieldLabel>
      <Input
        id={controlId(control)}
        value={panel.draft.profile[field]}
        inputMode={numeric ? "numeric" : undefined}
        readOnly={panel.saving}
        onChange={(event) =>
          setField(
            panel,
            field,
            numeric ? digitsOnly(event.target.value) : event.target.value,
          )
        }
        onKeyDown={saveOnEnter(panel.save)}
        className={numeric || hint !== undefined ? "h-8 tabular-nums" : "h-8"}
        {...invalidProps(panel.problem, control)}
      />
      {hint !== undefined && <FieldDescription>{hint}</FieldDescription>}
      <Problem problem={panel.problem} control={control} />
    </Field>
  );
}

function ProfileSelect({
  panel,
  field,
  label,
  options,
}: {
  panel: PersonPanelProps;
  field: "gender" | "bloodType";
  label: UiText;
  // Blood types are symbols, not words, so labels need not be UiText.
  options: readonly { value: string; label: string }[];
}) {
  const control = `profile-${field}`;
  const value = panel.draft.profile[field];
  return (
    <Field data-invalid={panel.problem?.control === control || undefined}>
      <FieldLabel htmlFor={controlId(control)}>{label}</FieldLabel>
      <Select
        value={value === "" ? notSet : value}
        disabled={panel.saving}
        onValueChange={(next) => setField(panel, field, next === notSet ? "" : next)}
      >
        <SelectTrigger
          id={controlId(control)}
          size="sm"
          className="w-full"
          {...invalidProps(panel.problem, control)}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={notSet}>{text.notSet}</SelectItem>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Problem problem={panel.problem} control={control} />
    </Field>
  );
}

const genders = t.credits.personPage.genders;
const genderOptions = (["female", "male", "other"] as const).map((value) => ({
  value,
  label: genders[value],
}));
const bloodTypeOptions = ["A", "B", "O", "AB"].map((value) => ({ value, label: value }));

/** Pairs stay side by side and the measurements share one row at every width. */
export default function ProfilePanel(panel: PersonPanelProps) {
  return (
    <>
      <div className="grid grid-cols-2 gap-4">
        <ProfileSelect
          panel={panel}
          field="gender"
          label={text.gender}
          options={genderOptions}
        />
        <ProfileSelect
          panel={panel}
          field="bloodType"
          label={text.bloodType}
          options={bloodTypeOptions}
        />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <ProfileInput
          panel={panel}
          field="birthDate"
          label={text.birthDate}
          hint={text.dateHint}
        />
        <ProfileInput
          panel={panel}
          field="deathDate"
          label={text.deathDate}
          hint={text.dateHint}
        />
      </div>
      <ProfileInput panel={panel} field="birthplace" label={text.birthplace} />
      <div className="grid grid-cols-2 gap-4">
        <ProfileInput panel={panel} field="heightCm" label={text.heightCm} />
        <ProfileInput panel={panel} field="cup" label={text.cup} />
      </div>
      <div className="grid grid-cols-3 gap-4">
        <ProfileInput panel={panel} field="bustCm" label={text.bustCm} />
        <ProfileInput panel={panel} field="waistCm" label={text.waistCm} />
        <ProfileInput panel={panel} field="hipsCm" label={text.hipsCm} />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <ProfileInput panel={panel} field="careerStartYear" label={text.careerFrom} />
        <ProfileInput panel={panel} field="careerEndYear" label={text.careerUntil} />
      </div>
      <Field className="flex-1">
        <FieldLabel htmlFor={controlId("profile-bio")}>{text.bio}</FieldLabel>
        <Textarea
          id={controlId("profile-bio")}
          value={panel.draft.profile.bio}
          rows={4}
          readOnly={panel.saving}
          onChange={(event) => setField(panel, "bio", event.target.value)}
          className="flex-1 resize-none field-sizing-fixed"
          {...invalidProps(panel.problem, "profile-bio")}
        />
        <Problem problem={panel.problem} control="profile-bio" />
      </Field>
    </>
  );
}
