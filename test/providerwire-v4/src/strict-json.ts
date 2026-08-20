import {
  getNodeValue,
  parseTree,
  printParseErrorCode,
  type Node,
  type ParseError,
} from "jsonc-parser";

export function parseStrictJson(source: string): unknown {
  const errors: ParseError[] = [];
  const root = parseTree(source, errors, {
    allowEmptyContent: false,
    allowTrailingComma: false,
    disallowComments: true,
  });
  if (!root || errors.length > 0) {
    const details = errors
      .map((error) => `${printParseErrorCode(error.error)} at offset ${error.offset}`)
      .join(", ");
    throw new Error(`invalid JSON syntax: ${details || "empty input"}`);
  }
  assertNoDuplicateMembers(root, "$");
  return normalizeParsedValue(getNodeValue(root));
}

function normalizeParsedValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(normalizeParsedValue);
  }
  if (typeof value !== "object" || value === null) {
    return value;
  }
  return Object.fromEntries(
    Object.entries(value).map(([key, child]) => [key, normalizeParsedValue(child)]),
  );
}

function assertNoDuplicateMembers(node: Node, path: string): void {
  if (node.type === "object") {
    const names = new Set<string>();
    for (const property of node.children ?? []) {
      const nameNode = property.children?.[0];
      const valueNode = property.children?.[1];
      const name = nameNode?.value;
      if (typeof name !== "string" || !valueNode) {
        throw new Error(`invalid JSON object at ${path}`);
      }
      if (names.has(name)) {
        throw new Error(`duplicate JSON member ${JSON.stringify(name)} at ${path}`);
      }
      names.add(name);
      assertNoDuplicateMembers(valueNode, `${path}/${escapeJsonPointer(name)}`);
    }
    return;
  }
  if (node.type === "array") {
    for (const [index, child] of (node.children ?? []).entries()) {
      assertNoDuplicateMembers(child, `${path}/${index}`);
    }
  }
}

function escapeJsonPointer(value: string): string {
  return value.replaceAll("~", "~0").replaceAll("/", "~1");
}
