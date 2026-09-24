import { build } from "esbuild";
import { readFile, writeFile } from "node:fs/promises";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";

const bundles = [
  {
    entryPoints: ["web/analytics-entry.js"],
    outfile: "internal/server/assets/analytics.js",
    banner: { js: "/*! Apache ECharts 6.1.0 | Apache-2.0 | https://echarts.apache.org/ */" },
  },
  {
    entryPoints: ["web/app-entry.js"],
    outfile: "internal/server/assets/app.js",
  },
];

for (const bundle of bundles) {
  await build({
    ...bundle,
    bundle: true,
    minify: true,
    legalComments: "none",
    target: ["es2020"],
    platform: "browser",
  });
  const artifact = await readFile(bundle.outfile);
  await Promise.all([
    writeFile(`${bundle.outfile}.gz`, gzipSync(artifact, { level: 9 })),
    writeFile(`${bundle.outfile}.br`, brotliCompressSync(artifact, {
      params: { [constants.BROTLI_PARAM_QUALITY]: 11 },
    })),
  ]);
}
