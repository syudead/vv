import { describe, expect, it } from "vitest";

import { RequestFailed } from "../api/client";
import { kindFieldError, tagFieldError } from "./tagNameField";

describe("tagFieldError", () => {
  it("tag_name_taken はサーバーの message ではなくカタログの文を出す（name_is_tag）", () => {
    const failure = new RequestFailed(409, "tag_name_taken", "サーバーの文", {
      reason: "name_is_tag",
      tagName: "旅行",
    });
    expect(tagFieldError(failure)).toEqual({
      kind: "taken",
      message: 'A tag named "旅行" already exists.',
    });
  });

  it("tag_name_taken はサーバーの message ではなくカタログの文を出す（name_is_synonym）", () => {
    const failure = new RequestFailed(409, "tag_name_taken", "サーバーの文", {
      reason: "name_is_synonym",
      tagName: "Anime",
    });
    expect(tagFieldError(failure)).toEqual({
      kind: "taken",
      message: 'That name is already a synonym of the tag "Anime".',
    });
  });

  it("本文に message が無くても重なりの理由を出す", () => {
    const failure = new RequestFailed(409, "tag_name_taken", "");
    expect(tagFieldError(failure)).toEqual({
      kind: "taken",
      message: "That name is already used by another tag.",
    });
  });
});

describe("kindFieldError", () => {
  it("名前の重複と名前の規則違反は入力の下の誤り（taken）にする", () => {
    const taken = new RequestFailed(409, "conflict", "サーバーの文", {
      reason: "tag_kind_name_taken",
      tagName: "Genre",
    });
    expect(kindFieldError(taken)).toEqual({
      kind: "taken",
      message: 'A kind named "Genre" already exists.',
    });
    const empty = new RequestFailed(400, "invalid_request", "サーバーの文", {
      reason: "tag_name_empty",
    });
    expect(kindFieldError(empty).kind).toBe("taken");
  });

  it("名前と関わらない失敗と reason の無い失敗は一般の失敗（other）にする", () => {
    const gone = new RequestFailed(404, "not_found", "サーバーの文", {
      reason: "tag_kind_not_found",
    });
    expect(kindFieldError(gone).kind).toBe("other");
    expect(kindFieldError(new RequestFailed(500, "internal", "failed")).kind).toBe(
      "other",
    );
    expect(kindFieldError(new Error("offline")).kind).toBe("other");
  });
});
