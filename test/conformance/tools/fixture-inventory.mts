import { readdirSync } from "node:fs";
import { join } from "node:path";

const providerInputPattern = /^(?:input(?:-\d+)?\.chunks\.txt|input\.response\.json)$/;

export function listProviderInputFiles(root: string): string[] {
  const inputs: string[] = [];

  function walk(dir: string): void {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name);
      if (entry.isDirectory()) {
        walk(path);
      } else if (entry.isFile() && providerInputPattern.test(entry.name)) {
        inputs.push(path);
      }
    }
  }

  walk(root);
  return inputs.sort();
}
