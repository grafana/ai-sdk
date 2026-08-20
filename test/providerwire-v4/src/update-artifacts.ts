import { fileURLToPath } from "node:url";
import {
  generateSemanticRequestsArtifact,
  semanticRequestsPath,
  writeSemanticRequestsArtifact,
} from "./artifacts.ts";
import {
  classificationPath,
  generateClassificationArtifact,
  writeClassificationArtifact,
} from "./classification.ts";
import { validateEvidence } from "./validate-evidence.ts";

export async function updateArtifacts(): Promise<void> {
  const artifact = await generateSemanticRequestsArtifact();
  const errors = validateEvidence(artifact);
  if (errors.length > 0) {
    throw new Error(errors.join("\n"));
  }
  const classification = generateClassificationArtifact();
  writeSemanticRequestsArtifact(artifact);
  writeClassificationArtifact(classification);
  console.log(`wrote ${semanticRequestsPath}`);
  console.log(`wrote ${classificationPath}`);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await updateArtifacts();
}
