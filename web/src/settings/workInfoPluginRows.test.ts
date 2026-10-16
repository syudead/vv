import { describe, expect, it } from "vitest";

import type { WorkInfoPluginSetting } from "../api/workInfoPlugins";
import {
  draftsOf,
  missingRequired,
  pluginName,
  runFailureText,
  settingChanges,
  settingsSummary,
  staleSettings,
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
const sort: WorkInfoPluginSetting = {
  key: "sort",
  label: "Sort",
  secret: false,
  required: true,
  set: false,
  default: "new",
  options: ["new", "popular"],
};
const mode: WorkInfoPluginSetting = {
  key: "mode",
  label: "Mode",
  secret: false,
  required: false,
  set: true,
  value: "old",
  staleValue: "old",
  options: ["fast", "slow"],
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

  it("既定値で動く設定は default、選択肢の外の保存値は not set と要約する", () => {
    expect(settingsSummary({ settings: [sort, mode, language] })).toBe(
      "Settings: Sort default, Mode not set, Language set",
    );
  });

  it("既定値のある必須の設定は値が無くても挙げず、選択肢の外の保存値は選び直しに挙げる", () => {
    expect(missingRequired({ settings: [sort, mode, region] })).toEqual(["Region"]);
    expect(staleSettings({ settings: [sort, mode, region] })).toEqual(["Mode"]);
  });

  it("選択肢の外の保存値はそのままでは送らず、先頭の項目を選ぶと null を送る", () => {
    const settings = [mode, sort];
    expect(settingChanges(settings, draftsOf(settings))).toEqual({});
    expect(
      settingChanges(settings, {
        mode: { value: "", forgotten: false },
        sort: { value: "popular", forgotten: false },
      }),
    ).toEqual({ mode: null, sort: "popular" });
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

  it("実行の失敗は 049 の理由の文にし、接続先を持たない失敗は接続先の無い形にする", () => {
    expect(runFailureText("plugin_failed")).toBe("The plugin failed.");
    expect(runFailureText("timed_out")).toBe(
      "The plugin didn't finish in 30 seconds and was stopped.",
    );
    expect(runFailureText("memory_exceeded")).toBe(
      "The plugin used more than 128 MB of memory and was stopped.",
    );
    expect(runFailureText("output_too_large")).toBe("The plugin's answer was over 1 MB.");
    expect(runFailureText("blocked_address")).toBe(
      "The plugin tried to reach VVMDM itself or a device on your network.",
    );
    expect(runFailureText("network_error")).toBe("The plugin couldn't reach a site.");
  });
});
