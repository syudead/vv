import { ExternalLink } from "lucide-react";
import { Fragment, type ReactNode, useLayoutEffect, useRef, useState } from "react";
import { Link } from "react-router";

import {
  type WorkInfo as WorkInfoValue,
  type WorkOrganizationRole,
  type WorkPersonRole,
  workDateKinds,
  workOrganizationRoles,
  workPersonRoles,
} from "../api/workInfo";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";

/** foldLimit は、長いリストを畳んだときに出しておく項目の数である。 */
const foldLimit = 3;

const foldButton = "h-auto px-0 py-0 text-muted-foreground hover:text-primary";

function dateLabel(kind: (typeof workDateKinds)[number]): UiText {
  const w = t.player.workInfo;
  if (kind === "release") return w.released;
  if (kind === "recorded") return w.recorded;
  return w.published;
}

function personRoleLabel(role: WorkPersonRole, count: number): UiText {
  const w = t.player.workInfo;
  if (role === "performer") return w.performer(count);
  if (role === "director") return w.director(count);
  if (role === "photographer") return w.photographer(count);
  return w.streamer(count);
}

function organizationRoleLabel(role: WorkOrganizationRole, count: number): UiText {
  const w = t.player.workInfo;
  return role === "maker" ? w.maker(count) : w.labelRole(count);
}

/** withoutScheme は URL の先頭の `http://`・`https://` を除いた文字である。 */
export function withoutScheme(url: string): string {
  return url.replace(/^https?:\/\//i, "");
}

/** hasWorkInfo は、作品情報に出す項目が 1 つでもあるかを返す。 */
export function hasWorkInfo(info: WorkInfoValue | undefined): info is WorkInfoValue {
  if (info === undefined) return false;
  return (
    info.code !== undefined ||
    info.description !== undefined ||
    info.dates.length > 0 ||
    info.people.length > 0 ||
    info.organizations.length > 0 ||
    info.urls.length > 0 ||
    info.externalIds.length > 0
  );
}

function Item({ label, children }: { label: UiText; children: ReactNode }) {
  return (
    <li className="flex max-w-full min-w-0 flex-wrap items-baseline gap-x-1.5">
      <span className="text-muted-foreground">{label}</span>
      {children}
    </li>
  );
}

/**
 * CreditLink は人物・団体の名前で、押すとそれで絞ったライブラリ（`/?person=<id>`・
 * `/?organization=<id>`）を開く（ui-design.md「Video page: work details row」、
 * 「Library: filter by a person or organization」）。本文の色で、上に乗ると `primary`。
 */
function CreditLink({
  kind,
  credit,
}: {
  kind: "person" | "organization";
  credit: { id: number; name: string };
}) {
  return (
    <Link
      to={`/?${kind}=${String(credit.id)}`}
      aria-label={t.player.workInfo.showVideosWith(credit.name)}
      className="text-foreground hover:text-primary"
    >
      {credit.name}
    </Link>
  );
}

/**
 * ListItem はいくつもの値を `, ` でつないだ項目である。4 つ以上あれば先頭の 3 つと
 * 「+{n} more」を出し、押すとその項目のすべてを出してフォーカスを 4 つ目へ移す
 * （ui-design.md「Work details row」の Long lists）。開いたことは覚えない。
 */
function ListItem({
  label,
  entries,
}: {
  label: UiText;
  entries: readonly { key: string; node: ReactNode }[];
}) {
  const [open, setOpen] = useState(false);
  const fourth = useRef<HTMLSpanElement | null>(null);
  const focusFourth = useRef(false);
  const folded = !open && entries.length > foldLimit;
  const shown = folded ? entries.slice(0, foldLimit) : entries;

  useLayoutEffect(() => {
    if (!open || !focusFourth.current) return;
    focusFourth.current = false;
    // 4 つ目が名前のリンクならそのリンクへ、そうでなければ値そのものへ移す。
    const entry = fourth.current;
    (entry?.querySelector<HTMLElement>("a[href]") ?? entry)?.focus();
  }, [open]);

  return (
    <Item label={label}>
      <span className="min-w-0 wrap-anywhere">
        {shown.map((entry, index) => (
          <Fragment key={entry.key}>
            {index > 0 && ", "}
            <span
              ref={index === foldLimit ? fourth : undefined}
              tabIndex={index === foldLimit ? -1 : undefined}
            >
              {entry.node}
            </span>
          </Fragment>
        ))}
        {entries.length > foldLimit && " "}
        {entries.length > foldLimit && (
          <Button
            variant="link"
            size="sm"
            className={foldButton}
            aria-label={
              folded ? t.player.workInfo.showAll(entries.length, label) : undefined
            }
            onClick={() => {
              focusFourth.current = folded;
              setOpen(folded);
            }}
          >
            {folded
              ? t.player.workInfo.more(entries.length - foldLimit)
              : t.player.workInfo.showFewer}
          </Button>
        )}
      </span>
    </Item>
  );
}

/**
 * Description は作品の説明で、3 行を超えるときは畳んで「Show more」を添える
 * （ui-design.md「Folded description」）。溢れるかは要素で測り、窓の大きさが変わったら
 * 測り直す。開いたことは覚えない（別の動画では呼び手が作り直す）。取得の比較の段も
 * 同じ畳み方で使い、今の側は `muted` で従の色にする。
 */
export function Description({
  text,
  tone = "default",
}: {
  text: string;
  tone?: "default" | "muted";
}) {
  const ref = useRef<HTMLParagraphElement | null>(null);
  const [open, setOpen] = useState(false);
  const [overflows, setOverflows] = useState(false);

  useLayoutEffect(() => {
    if (open) return;
    const element = ref.current;
    if (element === null) return;
    const measure = () => setOverflows(element.scrollHeight > element.clientHeight + 1);
    measure();
    window.addEventListener("resize", measure);
    return () => window.removeEventListener("resize", measure);
  }, [open, text]);

  return (
    <div className="flex flex-col items-start gap-1">
      <p
        ref={ref}
        className={cn(
          "max-w-prose text-sm wrap-anywhere whitespace-pre-line",
          tone === "muted" ? "text-muted-foreground" : "text-foreground",
          !open && "line-clamp-3",
        )}
      >
        {text}
      </p>
      {(open || overflows) && (
        <Button
          variant="link"
          size="sm"
          className={foldButton}
          onClick={() => setOpen(!open)}
        >
          {open ? t.player.workInfo.showLess : t.player.workInfo.showMore}
        </Button>
      )}
    </div>
  );
}

/**
 * WorkInfo は情報欄の作品情報の行である（specs/047-video-work-info/ui-design.md
 * 「Video page: work details row」）。ファイルの行と同じ形の折り返す並びに、値のある項目だけを
 * ラベル（薄い色）と値の組で出し、説明はその下に 3 行まで畳んで出す。項目が 1 つも無ければ
 * 何も描かない（情報欄は今までどおり）。ゲストの応答には `workInfo` が無いので出ない。
 * 日付は保存した文字列のまま出し、日付の書式は通さない。
 */
export default function WorkInfo({ info }: { info: WorkInfoValue | undefined }) {
  if (!hasWorkInfo(info)) return null;
  const w = t.player.workInfo;
  const dates = workDateKinds.flatMap((kind) => {
    const date = info.dates.find((entry) => entry.kind === kind);
    return date === undefined ? [] : [date];
  });
  const people = workPersonRoles.flatMap((role) => {
    const names = info.people.filter((entry) => entry.role === role);
    return names.length === 0 ? [] : [{ role, names }];
  });
  const organizations = workOrganizationRoles.flatMap((role) => {
    const names = info.organizations.filter((entry) => entry.role === role);
    return names.length === 0 ? [] : [{ role, names }];
  });
  const hasRow =
    info.code !== undefined ||
    dates.length > 0 ||
    people.length > 0 ||
    organizations.length > 0 ||
    info.urls.length > 0 ||
    info.externalIds.length > 0;

  return (
    <>
      {hasRow && (
        <ul
          aria-label={w.label}
          className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm sm:gap-x-5"
        >
          {info.code !== undefined && (
            <Item label={w.code}>
              <span className="min-w-0 tabular-nums wrap-anywhere">{info.code}</span>
            </Item>
          )}
          {dates.map((date) => (
            <Item key={date.kind} label={dateLabel(date.kind)}>
              <span className="tabular-nums">{date.date}</span>
            </Item>
          ))}
          {people.map(({ role, names }) => (
            <ListItem
              key={role}
              label={personRoleLabel(role, names.length)}
              entries={names.map((entry) => ({
                key: String(entry.person.id),
                node: <CreditLink kind="person" credit={entry.person} />,
              }))}
            />
          ))}
          {organizations.map(({ role, names }) => (
            <ListItem
              key={role}
              label={organizationRoleLabel(role, names.length)}
              entries={names.map((entry) => ({
                key: String(entry.organization.id),
                node: <CreditLink kind="organization" credit={entry.organization} />,
              }))}
            />
          ))}
          {info.urls.length > 0 && (
            <ListItem
              label={w.links}
              entries={info.urls.map((url) => ({
                key: url,
                node: (
                  <a
                    href={url}
                    title={url}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex max-w-xs items-center gap-1 align-bottom hover:text-primary"
                  >
                    <span className="truncate">{withoutScheme(url)}</span>
                    <ExternalLink className="size-3 shrink-0" aria-hidden="true" />
                  </a>
                ),
              }))}
            />
          )}
          {info.externalIds.length > 0 && (
            <ListItem
              label={w.ids}
              entries={info.externalIds.map((id) => ({
                key: `${id.source}\u0000${id.value}`,
                node: (
                  <span className="tabular-nums">
                    {w.externalId(id.source, id.value)}
                  </span>
                ),
              }))}
            />
          )}
        </ul>
      )}
      {info.description !== undefined && <Description text={info.description} />}
    </>
  );
}
