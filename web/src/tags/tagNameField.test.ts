import { describe, expect, it } from "vitest";

import { RequestFailed } from "../api/client";
import { tagFieldError } from "./tagNameField";

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
