import { describe, expect, it } from "vitest";

import type { WorkInfoPluginSetting } from "../api/workInfoPlugins";
import {
  draftsOf,
  missingRequired,
  pluginName,
  settingChanges,
  settingsSummary,
} from "./workInfoPluginRows";

const apiKey: WorkInfoPluginSetting = {
  key: "apiKey",
  label: "API key",
  secret: true,
  required: true,
  set: true,
};
const language: WorkInfoPluginSetting = {
  key: "lang",
  label: "Language",
  secret: false,
  required: false,
  set: true,
  value: "ja",
};
const region: WorkInfoPluginSetting = {
  key: "region",
  label: "Region",
  secret: false,
  required: true,
  set: false,
};

describe("work-info plugin rows", () => {
  it("名前の無いプラグインはファイル名で呼ぶ", () => {
    expect(pluginName({ name: "example", fileName: "a.wasm" })).toBe("example");
    expect(pluginName({ fileName: "a.wasm" })).toBe("a.wasm");
  });

  it("設定の要約はラベルと設定済みかを宣言の順に並べ、宣言が無ければ出さない", () => {
    expect(settingsSummary({ settings: [apiKey, region] })).toBe(
      "Settings: API key set, Region not set",
    );
    expect(settingsSummary({ settings: [] })).toBeNull();
  });

  it("値の無い必須の設定だけを挙げる", () => {
    expect(missingRequired({ settings: [apiKey, language, region] })).toEqual(["Region"]);
  });

  it("秘密の欄は空で始まり、秘密でない欄は保存された値で始まる", () => {
    expect(draftsOf([apiKey, language, region])).toEqual({
      apiKey: { value: "", forgotten: false },
      lang: { value: "ja", forgotten: false },
      region: { value: "", forgotten: false },
    });
  });

  it("変わったキーだけを送り、空にした値と忘れた秘密は null にする", () => {
    const settings = [apiKey, language, region];
    expect(settingChanges(settings, draftsOf(settings))).toEqual({});
    expect(
      settingChanges(settings, {
        apiKey: { value: "", forgotten: true },
        lang: { value: "", forgotten: false },
        region: { value: "jp", forgotten: false },
      }),
    ).toEqual({ apiKey: null, lang: null, region: "jp" });
    expect(
      settingChanges(settings, {
        apiKey: { value: "new", forgotten: false },
        lang: { value: "en", forgotten: false },
        region: { value: "", forgotten: false },
      }),
    ).toEqual({ apiKey: "new", lang: "en" });
  });

  it("保存されていない秘密を忘れても何も送らない", () => {
    const unsaved = { ...apiKey, set: false };
    expect(settingChanges([unsaved], { apiKey: { value: "", forgotten: true } })).toEqual(
      {},
    );
  });
});
