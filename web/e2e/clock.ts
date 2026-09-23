import type { Page } from "@playwright/test";

/**
 * elapse は「一定時間待っても何も起きない」ことを確かめる前に、画面の時計だけを
 * ms 進める。実時間を待たずに、その間に満期になる setTimeout を全部走らせる。
 * 使う page では goto より前に page.clock.install() を呼んでおく。
 *
 * 時計を進めた直後は、タイマーの中で積まれた React の描画がまだ終わっていない
 * ことがある。React はその描画を MessageChannel で後回しにし、MessageChannel は
 * 偽の時計の対象外なので、同じ仕組みで1周回してから返す。これを省くと
 * 「video が無い」の検査が描画前に通ってしまい、検査として意味を持たない。
 */
export async function elapse(page: Page, ms: number) {
  await page.clock.runFor(ms);
  await page.evaluate(
    () =>
      new Promise<void>((resolve) => {
        const channel = new MessageChannel();
        channel.port1.onmessage = () => {
          channel.port1.close();
          resolve();
        };
        channel.port2.postMessage(null);
      }),
  );
}
