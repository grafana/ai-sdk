import assert from "node:assert/strict";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import {
  commitRecording,
  makeBedrockFetchTap,
  normalizeRecordingValue,
  resolveRecordingPlaceholders,
} from "./record.mts";

test("recording placeholders", async t => {
  await t.test("resolves embedded values and normalizes snapshots", () => {
    const previous = process.env.RECORD_TEST_BUCKET;
    process.env.RECORD_TEST_BUCKET = "real-bucket";
    try {
      const resolved = resolveRecordingPlaceholders({
        url: "s3://${RECORD_TEST_BUCKET}/fixture.png",
      });
      assert.deepEqual(resolved.value, {
        url: "s3://real-bucket/fixture.png",
      });
      assert.deepEqual(
        normalizeRecordingValue(
          { uri: "s3://real-bucket/fixture.png" },
          resolved.replacements,
        ),
        { uri: "s3://${RECORD_TEST_BUCKET}/fixture.png" },
      );
    } finally {
      if (previous === undefined) delete process.env.RECORD_TEST_BUCKET;
      else process.env.RECORD_TEST_BUCKET = previous;
    }
  });

  await t.test("rejects missing values", () => {
    const previous = process.env.RECORD_TEST_MISSING;
    delete process.env.RECORD_TEST_MISSING;
    try {
      assert.throws(
        () => resolveRecordingPlaceholders("${RECORD_TEST_MISSING}"),
        /Missing recording environment variable/,
      );
    } finally {
      if (previous !== undefined) process.env.RECORD_TEST_MISSING = previous;
    }
  });
});

test("commitRecording", async t => {
  await t.test("replaces generated files and removes stale numbered inputs", () => {
    const dir = mkdtempSync(join(tmpdir(), "recording-test-"));
    try {
      writeFileSync(join(dir, "input-1.chunks.txt"), "old-1");
      writeFileSync(join(dir, "input-2.chunks.txt"), "old-2");
      writeFileSync(join(dir, "expected.jsonl"), "old-output");
      writeFileSync(join(dir, "expected-requests.jsonl"), "old-request");

      commitRecording(dir, stageDir => {
        writeFileSync(join(stageDir, "input.chunks.txt"), "new-input");
        writeFileSync(join(stageDir, "expected.jsonl"), "new-output");
        writeFileSync(join(stageDir, "expected-requests.jsonl"), "new-request");
      });

      assert.equal(readFileSync(join(dir, "input.chunks.txt"), "utf8"), "new-input");
      assert.equal(readFileSync(join(dir, "expected.jsonl"), "utf8"), "new-output");
      assert.equal(
        readFileSync(join(dir, "expected-requests.jsonl"), "utf8"),
        "new-request",
      );
      assert.equal(existsSync(join(dir, "input-1.chunks.txt")), false);
      assert.equal(existsSync(join(dir, "input-2.chunks.txt")), false);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  await t.test("rejects incomplete staged fixture families", () => {
    const dir = mkdtempSync(join(tmpdir(), "recording-test-"));
    try {
      writeFileSync(join(dir, "input.chunks.txt"), "old-input");
      writeFileSync(join(dir, "expected.jsonl"), "old-output");
      writeFileSync(join(dir, "expected-requests.jsonl"), "old-request");

      assert.throws(
        () => commitRecording(dir, stageDir => {
          writeFileSync(join(stageDir, "input-1.chunks.txt"), "new-1");
          writeFileSync(join(stageDir, "input-2.chunks.txt"), "new-2");
          writeFileSync(join(stageDir, "input-3.chunks.txt"), "new-3");
        }),
        /complete fixture set/,
      );

      assert.equal(readFileSync(join(dir, "input.chunks.txt"), "utf8"), "old-input");
      assert.equal(readFileSync(join(dir, "expected.jsonl"), "utf8"), "old-output");
      assert.equal(
        readFileSync(join(dir, "expected-requests.jsonl"), "utf8"),
        "old-request",
      );
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  await t.test("preserves existing files when staging fails", () => {
    const dir = mkdtempSync(join(tmpdir(), "recording-test-"));
    try {
      writeFileSync(join(dir, "input.chunks.txt"), "old-input");
      writeFileSync(join(dir, "expected.jsonl"), "old-output");
      writeFileSync(join(dir, "expected-requests.jsonl"), "old-request");

      assert.throws(
        () => commitRecording(dir, () => {
          throw new Error("staging failed");
        }),
        /staging failed/,
      );

      assert.equal(readFileSync(join(dir, "input.chunks.txt"), "utf8"), "old-input");
      assert.equal(readFileSync(join(dir, "expected.jsonl"), "utf8"), "old-output");
      assert.equal(
        readFileSync(join(dir, "expected-requests.jsonl"), "utf8"),
        "old-request",
      );
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});

test("Bedrock fetch capture", async () => {
  const originalFetch = globalThis.fetch;
  let releaseCapture: (() => void) | undefined;
  globalThis.fetch = async () => new Response(
    new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new Uint8Array());
        releaseCapture = () => controller.close();
      },
    }),
    { status: 200, headers: { "content-type": "application/vnd.amazon.eventstream" } },
  );

  try {
    let captured = false;
    const tap = makeBedrockFetchTap(
      () => {
        captured = true;
      },
      () => {},
    );
    const response = await tap.fetch(
      "https://bedrock-runtime.us-east-2.amazonaws.com/model/test/converse-stream",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: "{}",
      },
    );
    const forwarded = response.arrayBuffer();
    let waitFinished = false;
    const waiting = tap.waitForCaptures().then(() => {
      waitFinished = true;
    });

    await new Promise<void>(resolve => setImmediate(resolve));
    assert.equal(waitFinished, false);
    assert.ok(releaseCapture);
    releaseCapture();
    await Promise.all([forwarded, waiting]);
    assert.equal(captured, true);
  } finally {
    globalThis.fetch = originalFetch;
  }
});
