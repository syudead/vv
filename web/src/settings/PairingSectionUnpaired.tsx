import { type FormEvent, useId, useState } from "react";

import { pairPeer, type PeerState } from "../api/peer";
import { t, type UiText } from "../i18n";
import { PageSection } from "../ui/patterns/page-section";
import { Button } from "../ui/shadcn/button";
import { Field, FieldDescription, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";

import { normalizePairingCode, pairFailure, type PairField } from "./pairing";
import { PairingCodeRow } from "./PairingSectionCode";
import { FingerprintAlert } from "./PairingSectionFingerprint";
import { FailureAlert } from "./SaveStatus";

type Values = Record<PairField, string>;

/** The fingerprint waits for the owner together with the address it came from. */
type Problem =
  | { kind: "alert"; text: UiText }
  | { kind: "untrusted"; fingerprint: string; address: string };

function PairInput({
  field,
  label,
  description,
  value,
  error,
  disabled,
  placeholder,
  mono = false,
  onChange,
}: {
  field: PairField;
  label: UiText;
  description: UiText;
  value: string;
  error: UiText | undefined;
  disabled: boolean;
  placeholder?: string;
  mono?: boolean;
  onChange: (field: PairField, value: string) => void;
}) {
  const id = useId();
  const descriptionId = `${id}-description`;
  const errorId = `${id}-error`;
  return (
    <Field data-invalid={error !== undefined || undefined}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        type={mono ? "text" : "url"}
        autoComplete="off"
        spellCheck={false}
        placeholder={placeholder}
        value={value}
        disabled={disabled}
        onChange={(event) =>
          onChange(field, mono ? event.target.value.toUpperCase() : event.target.value)
        }
        aria-invalid={error !== undefined || undefined}
        aria-describedby={
          error === undefined ? descriptionId : `${descriptionId} ${errorId}`
        }
        className={mono ? "font-mono sm:max-w-sm" : "sm:max-w-sm"}
      />
      <FieldDescription id={descriptionId}>{description}</FieldDescription>
      {error !== undefined && <FieldError id={errorId}>{error}</FieldError>}
    </Field>
  );
}

function ProblemAlert({
  problem,
  pending,
  onTrust,
}: {
  problem: Problem;
  pending: boolean;
  onTrust: (fingerprint: string) => void;
}) {
  const text = t.settings.pairing;
  if (problem.kind === "alert") {
    return <FailureAlert text={problem.text} />;
  }
  return (
    <FingerprintAlert
      title={text.form.untrustedTitle}
      address={problem.address}
      fingerprint={problem.fingerprint}
      action={text.form.trust}
      pending={pending}
      onTrust={() => onTrust(problem.fingerprint)}
    />
  );
}

/**
 * A refusal keeps every value; a field error clears when that field changes. Trusting a certificate
 * sends the same form again with the fingerprint shown.
 */
function PairForm({
  onPaired,
  onPendingChange,
}: {
  onPaired: (state: PeerState) => void;
  onPendingChange: (pending: boolean) => void;
}) {
  const text = t.settings.pairing.form;
  const titleId = useId();
  const [values, setValues] = useState<Values>({
    address: "",
    code: "",
    ownAddress: window.location.origin,
  });
  const [errors, setErrors] = useState<Partial<Record<PairField, UiText>>>({});
  const [problem, setProblem] = useState<Problem | null>(null);
  const [pending, setPending] = useState(false);

  const edit = (field: PairField, value: string) => {
    setValues((current) => ({ ...current, [field]: value }));
    setErrors((current) => ({ ...current, [field]: undefined }));
  };

  const pair = async (fingerprint?: string) => {
    const body = {
      address: values.address.trim(),
      code: normalizePairingCode(values.code),
      ownAddress: values.ownAddress.trim(),
      ...(fingerprint === undefined ? {} : { fingerprint }),
    };
    setPending(true);
    onPendingChange(true);
    setErrors({});
    setProblem(null);
    try {
      onPaired(await pairPeer(body));
      return;
    } catch (error) {
      const failure = pairFailure(error);
      if (failure.kind === "field") setErrors({ [failure.field]: failure.text });
      else if (failure.kind === "alert") setProblem(failure);
      else
        setProblem({
          kind: "untrusted",
          fingerprint: failure.fingerprint,
          address: body.address,
        });
    }
    setPending(false);
    onPendingChange(false);
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!pending) void pair();
  };

  const field = (
    name: PairField,
    label: UiText,
    description: UiText,
    extra: { placeholder?: string; mono?: boolean } = {},
  ) => (
    <PairInput
      field={name}
      label={label}
      description={description}
      value={values[name]}
      error={errors[name]}
      disabled={pending}
      onChange={edit}
      {...extra}
    />
  );

  return (
    <form
      aria-labelledby={titleId}
      noValidate
      onSubmit={submit}
      className="flex flex-col gap-3"
    >
      <h3 id={titleId} className="text-sm font-medium">
        {text.title}
      </h3>
      {field("address", text.address, text.addressDescription, {
        placeholder: text.addressPlaceholder,
      })}
      {field("code", text.code, text.codeDescription, { mono: true })}
      {field("ownAddress", text.ownAddress, text.ownAddressDescription)}
      {problem !== null && (
        <ProblemAlert
          problem={problem}
          pending={pending}
          onTrust={(fingerprint) => void pair(fingerprint)}
        />
      )}
      <div>
        <Button type="submit" size="sm" disabled={pending}>
          {pending && <Spinner aria-hidden="true" role={undefined} />}
          {pending ? text.submitting : text.submit}
        </Button>
      </div>
    </form>
  );
}

export function UnpairedSection({ onPaired }: { onPaired: (state: PeerState) => void }) {
  const text = t.settings.pairing.unpaired;
  const [pairing, setPairing] = useState(false);
  return (
    <PageSection title={text.heading} description={text.description}>
      <PairingCodeRow disabled={pairing} />
      <PairForm onPaired={onPaired} onPendingChange={setPairing} />
    </PageSection>
  );
}
