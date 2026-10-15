import { describe, expect, it } from "vitest";

import type { Video } from "../../api/client";
import type { WorkInfo } from "../../api/workInfo";
import type { WorkInfoFetch, WorkInfoMatch } from "../../api/workInfoPlugins";
import { chipHint, creditKind, detailRow, type ListRow } from "./detailRows";
import { comparedFields, initialSelection, skippedLines } from "./fetchRules";

function info(overrides: Partial<WorkInfo> = {}): WorkInfo {
  return {
    dates: [],
    people: [],
    organizations: [],
    urls: [],
    externalIds: [],
    ...overrides,
  };
}

function result(overrides: Partial<WorkInfoFetch> = {}): WorkInfoFetch {
  return {
    fetchId: "f",
    expiresAt: 0,
    tags: [],
    dates: [],
    people: [],
    organizations: [],
    urls: [],
    externalIds: [],
    ...overrides,
  };
}

function performer(name: string, match: WorkInfoMatch) {
  return { role: "performer" as const, name, externalIds: [], match };
}

function list(row: ReturnType<typeof detailRow>): ListRow {
  if (row.kind !== "list") throw new Error("not a list row");
  return row;
}

const stored = info({
  code: "ABC-1",
  dates: [{ kind: "release", date: "2024-01-01" }],
  people: [{ role: "performer", person: { id: 9, name: "既存の人" } }],
  urls: ["https://works.example/w/1"],
  externalIds: [{ source: "works", value: "1" }],
});

describe("detailRows", () => {
  it("単一の値の行は、値が無ければ missing、今と同じなら same、違えば規則に外れても differs", () => {
    expect(detailRow("code", stored, result()).state).toBe("missing");
    expect(detailRow("code", stored, result({ code: "ABC-1" })).state).toBe("same");
    expect(detailRow("dates.release", stored, result()).state).toBe("missing");
    const date = detailRow(
      "dates.release",
      stored,
      result({ dates: [{ kind: "release", date: "2024/03/01" }] }),
    );
    expect(date).toEqual({
      kind: "single",
      state: "differs",
      now: "2024-01-01",
      fetched: "2024/03/01",
    });
  });

  it("人物の行は照合の答えで印を付け、Not linked と動画にある件を新規に数えない", () => {
    const row = list(
      detailRow(
        "people.performer",
        stored,
        result({
          people: [
            performer("既存の人", { state: "existing", id: 9, name: "既存の人" }),
            performer("べつめい", { state: "existing", id: 5, name: "別の人" }),
            performer("新しい人", { state: "new" }),
            performer("同名の人", { state: "ambiguous" }),
            { ...performer("監督", { state: "new" }), role: "director" },
          ],
        }),
      ),
    );
    expect(row.now.map((chip) => chip.name)).toEqual(["既存の人"]);
    expect(row.fetched.map((chip) => [chip.name, chip.mark, chip.fetchedAs])).toEqual([
      ["既存の人", "onVideo", undefined],
      ["別の人", undefined, "べつめい"],
      ["新しい人", "new", undefined],
      ["同名の人", "notLinked", undefined],
    ]);
    expect(row.fresh).toBe(2);
    expect(row.state).toBe("differs");
  });

  it("反映で同じ実体にまとまる件は、新規に 1 つと数える", () => {
    const withIds = (name: string, value: string) => ({
      ...performer(name, { state: "new" }),
      externalIds: [{ source: "works", value }],
    });
    const row = list(
      detailRow(
        "people.performer",
        stored,
        result({
          people: [
            performer("別の人", { state: "existing", id: 5, name: "別の人" }),
            performer("べつめい", { state: "existing", id: 5, name: "別の人" }),
            performer("あたらしい", { state: "new" }),
            performer(" アタラシイ ", { state: "new" }),
            withIds("ID の人", "p1"),
            withIds("ID の人の別名", "p1"),
            withIds("違う人", "p2"),
          ],
        }),
      ),
    );
    // チップは取得した件ごとに出し、数は実体ごと（別の人・あたらしい・ID の人・違う人）。
    expect(row.fetched).toHaveLength(7);
    expect(row.fresh).toBe(4);
  });

  it("チップのツールチップは、動画にある件でも取得した綴りを出す", () => {
    expect(chipHint({ key: "0", name: "別の人", fetchedAs: "べつめい" }, "person")).toBe(
      'Fetched as "べつめい"',
    );
    expect(
      chipHint(
        { key: "0", name: "別の人", mark: "onVideo", fetchedAs: "べつめい" },
        "person",
      ),
    ).toBe('On the video. Fetched as "べつめい"');
    expect(chipHint({ key: "0", name: "別の人", mark: "onVideo" }, "person")).toBe(
      undefined,
    );
    expect(chipHint({ key: "0", name: "同名", mark: "notLinked" }, "organization")).toBe(
      "Matches more than one organization, so it isn't linked or created",
    );
  });

  it("一覧の行は、すべて動画にあれば same、残りが Not linked だけなら nothing、件が無ければ missing", () => {
    const linked = performer("既存の人", { state: "existing", id: 9, name: "既存の人" });
    expect(
      detailRow("people.performer", stored, result({ people: [linked] })).state,
    ).toBe("same");
    expect(
      detailRow(
        "people.performer",
        stored,
        result({ people: [linked, performer("同名の人", { state: "ambiguous" })] }),
      ).state,
    ).toBe("nothing");
    expect(detailRow("people.director", stored, result({ people: [linked] })).state).toBe(
      "missing",
    );
  });

  it("リンクは scheme を除いて出し、保存済みの URL と ID の組を動画にある件にする", () => {
    const urls = list(
      detailRow(
        "urls",
        stored,
        result({ urls: ["https://works.example/w/1", "http://other.example/a"] }),
      ),
    );
    expect(urls.fetched.map((chip) => [chip.name, chip.full, chip.mark])).toEqual([
      ["works.example/w/1", "https://works.example/w/1", "onVideo"],
      ["other.example/a", "http://other.example/a", undefined],
    ]);
    expect(urls.fresh).toBe(1);
    const ids = list(
      detailRow(
        "externalIds",
        stored,
        result({ externalIds: [{ source: "works", value: "1" }] }),
      ),
    );
    expect(ids.fetched.map((chip) => chip.name)).toEqual(["works: 1"]);
    expect(ids.state).toBe("same");
  });

  it("creditKind は人物・団体の行の種類を返す", () => {
    expect(creditKind("people.streamer")).toBe("person");
    expect(creditKind("organizations.label")).toBe("organization");
    expect(creditKind("urls")).toBeUndefined();
    expect(creditKind("title")).toBeUndefined();
  });
});

describe("fetchRules for work details", () => {
  const video = {
    id: 7,
    title: "今の題名",
    tags: [],
    workInfo: stored,
  } as unknown as Video;

  it("題名とタグはいつも出し、作品の詳細は値のある行だけを契約の順に出す", () => {
    const fetched = result({
      title: "今の題名",
      urls: ["https://works.example/w/2"],
      code: "ABC-2",
      organizations: [
        { role: "maker", name: "会社", externalIds: [], match: { state: "new" } },
      ],
    });
    expect(comparedFields(video, fetched)).toEqual([
      "title",
      "tags",
      "code",
      "organizations.maker",
      "urls",
    ]);
    expect(initialSelection(video, fetched)).toEqual([
      "code",
      "organizations.maker",
      "urls",
    ]);
  });

  it("結果の段の行は、タグ（却下したものが先）の後に飛ばした値を応答の順で、理由を項目で分けて並べる", () => {
    const lines = skippedLines(
      [
        { name: "悪い名前", reason: "invalid" },
        { name: "却下した", reason: "rejected" },
      ],
      [
        { field: "dates.release", value: "2024/03/01", reason: "invalid" },
        { field: "people.performer", value: "百一人目", reason: "too_many" },
        { field: "people.performer", value: "同名の人", reason: "ambiguous" },
        { field: "organizations.maker", value: "同名の会社", reason: "ambiguous" },
        { field: "urls", value: "works.example", reason: "invalid" },
        { field: "code", value: "長い品番", reason: "invalid" },
        { field: "externalIds", value: "works: 21", reason: "too_many" },
      ],
    );
    expect(lines.map((line) => `${line.label}: ${line.value} — ${line.reason}`)).toEqual([
      "Tags: 却下した — you rejected this tag before",
      "Tags: 悪い名前 — not a valid tag name",
      "Released: 2024/03/01 — not a date in the form YYYY-MM-DD, YYYY-MM or YYYY",
      "Performers: 百一人目 — over the limit for performers",
      "Performers: 同名の人 — matches more than one person",
      "Makers: 同名の会社 — matches more than one organization",
      "Links: works.example — not a full URL starting with http:// or https://",
      "Code: 長い品番 — too long or contains characters that can't be used",
      "IDs: works: 21 — over the limit for IDs",
    ]);
  });
});
