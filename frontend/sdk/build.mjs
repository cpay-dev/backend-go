import * as esbuild from "esbuild";

const isProd = process.argv.includes("--prod");

await esbuild.build({
  entryPoints: ["sdk/src/main.tsx"],
  bundle: true,
  format: "iife",
  globalName: "__CPay",
  outfile: "public/sdk/cpay.js",
  minify: isProd,
  sourcemap: isProd ? false : "inline",
  target: ["es2020"],
  jsx: "automatic",
  define: {
    "process.env.NODE_ENV": isProd ? '"production"' : '"development"',
  },
  // wagmi connectors have dynamic imports for optional peer deps — mark external
  external: [
    "@coinbase/wallet-sdk",
    "@metamask/sdk",
    "porto",
    "porto/internal",
  ],
  // Node built-ins that some deps reference — shim as empty
  alias: {
    "node:buffer": "buffer",
  },
  logLevel: "info",
});

console.log("SDK built → public/sdk/cpay.js");
