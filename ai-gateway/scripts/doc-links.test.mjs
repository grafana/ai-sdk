import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { LINKS } from '../src/pages/overview/content.ts';

const REPOSITORY_ROOT = new URL('../../', import.meta.url);
const BLOB_PREFIX = `${LINKS.repository}/blob/main/`;
const CATALOG_README = new URL('../src/README.md', import.meta.url);

function headingAnchors(markdown) {
  const anchors = markdown
    .split('\n')
    .filter((line) => /^#{1,6} /.test(line))
    .map((line) =>
      line
        .replace(/^#+ /, '')
        .trim()
        .toLowerCase()
        .replace(/[^\p{L}\p{N} _-]/gu, '')
        .replace(/ /g, '-')
    );
  return new Set(anchors);
}

function readmeLinks() {
  const markdown = readFileSync(CATALOG_README, 'utf8');
  return Array.from(markdown.matchAll(/\]\((https:[^)\s]+)\)/g), (match) => match[1]);
}

const pageLinks = Object.values(LINKS);
const repositoryLinks = [...new Set([...pageLinks, ...readmeLinks()])].filter((href) => href.startsWith(BLOB_PREFIX));

assert.ok(repositoryLinks.length, 'no repository documentation links found');
for (const href of repositoryLinks) {
  test(`${href} resolves to repository documentation`, () => {
    const [file, anchor] = href.slice(BLOB_PREFIX.length).split('#');
    const markdown = readFileSync(new URL(file, REPOSITORY_ROOT), 'utf8');

    if (anchor) {
      assert.ok(headingAnchors(markdown).has(anchor), `${file} has no heading anchor #${anchor}`);
    }
  });
}
