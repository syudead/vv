import { RequestFailed, updatePluginProxySettings } from "../api/client";
import { t } from "../i18n";

import { isPluginProxyURL } from "./pluginProxyURL";
import { TextSettingRow, useTextSettingSave } from "./TextSettingRow";

async function saveProxy(url: string): Promise<string> {
  return (await updatePluginProxySettings(url)).url;
}

export function PluginProxyRow({ initialURL }: { initialURL: string }) {
  const network = t.settings.network;
  const text = network.proxy;
  const setting = useTextSettingSave({
    initial: initialURL,
    save: saveProxy,
    isValid: isPluginProxyURL,
    isInvalid: (failure) => failure instanceof RequestFailed && failure.status === 400,
    failed: text.saveFailed,
  });

  return (
    <TextSettingRow
      id="network-plugin-proxy"
      label={text.label}
      description={text.description}
      placeholder={text.placeholder}
      invalidText={text.invalid}
      savingText={network.saving}
      savedText={text.saved}
      setting={setting}
    />
  );
}
