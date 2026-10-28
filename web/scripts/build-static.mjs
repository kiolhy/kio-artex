import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const nextBin = fileURLToPath(new URL("../node_modules/next/dist/bin/next", import.meta.url));
const nodeOptions = [process.env.NODE_OPTIONS, "--max-old-space-size=1024", "--max-semi-space-size=16"]
  .filter(Boolean)
  .join(" ");
const result = spawnSync(process.execPath, [nextBin, "build", "--webpack"], {
  env: { ...process.env, NEXT_EXPORT: "1", NODE_OPTIONS: nodeOptions },
  stdio: "inherit",
});

if (result.error) {
  console.error(result.error);
}

process.exit(result.status ?? 1);
