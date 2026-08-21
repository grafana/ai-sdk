import { fileURLToPath } from "node:url";
import {
  compareSemanticRequestsArtifact,
  generateSemanticRequestsArtifact,
} from "./artifacts.ts";
import {
  compareClassificationArtifact,
  generateClassificationArtifact,
} from "./classification.ts";
import { verifyProviderContractTests } from "./contract-tests.ts";
import { verifyCommittedSourceEquivalence } from "./source-equivalence.ts";
import { validateEvidence } from "./validate-evidence.ts";

export async function checkProviderWireV4(
  committedArtifactPath?: string,
  committedClassificationPath?: string,
  committedSourceEquivalencePath?: string,
): Promise<void> {
  verifyCommittedSourceEquivalence(committedSourceEquivalencePath);
  const artifact = await generateSemanticRequestsArtifact();
  const errors = [
    ...validateEvidence(artifact),
    ...compareSemanticRequestsArtifact(artifact, committedArtifactPath),
    ...compareClassificationArtifact(
      generateClassificationArtifact(),
      committedClassificationPath,
    ),
  ];
  if (errors.length > 0) {
    throw new Error(errors.join("\n"));
  }
  verifyProviderContractTests();
  console.log("ProviderWire V4 committed evidence and provider contract tests match");
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await checkProviderWireV4();
}
