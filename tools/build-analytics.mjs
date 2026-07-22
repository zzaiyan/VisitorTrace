import { build } from "esbuild";
import { readFile, writeFile } from "node:fs/promises";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";

await build({
  entryPoints: ["web/analytics-entry.js"],
  outfile: "internal/server/assets/analytics.js",
  bundle: true,
  minify: true,
  legalComments: "none",
  target: ["es2020"],
  platform: "browser",
  banner: { js: "/*! Apache ECharts 6.1.0 | Apache-2.0 | https://echarts.apache.org/ */" },
});

const bundle = await readFile("internal/server/assets/analytics.js");
await Promise.all([
  writeFile("internal/server/assets/analytics.js.gz", gzipSync(bundle, { level: 9 })),
  writeFile("internal/server/assets/analytics.js.br", brotliCompressSync(bundle, {
    params: { [constants.BROTLI_PARAM_QUALITY]: 11 },
  })),
]);
