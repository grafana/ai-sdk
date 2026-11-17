import fs from 'fs';
import path from 'path';
import { LINKS } from './content';

const REPOSITORY_ROOT = path.resolve(__dirname, '../../../..');
const BLOB_PREFIX = `${LINKS.repository}/blob/main/`;
const CATALOG_README = path.resolve(__dirname, '../../README.md');

function headingAnchors(markdown: string): Set<string> {
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

function readmeLinks(): string[] {
  const markdown = fs.readFileSync(CATALOG_README, 'utf8');
  return Array.from(markdown.matchAll(/\]\((https:[^)\s]+)\)/g), (match) => match[1]);
}

const pageLinks = Object.values(LINKS);
const repositoryLinks = [...new Set([...pageLinks, ...readmeLinks()])].filter((href) => href.startsWith(BLOB_PREFIX));

describe('documentation links', () => {
  test.each(repositoryLinks)('%s resolves to repository documentation', (href) => {
    const [file, anchor] = href.slice(BLOB_PREFIX.length).split('#');
    const markdown = fs.readFileSync(path.join(REPOSITORY_ROOT, file), 'utf8');

    if (anchor) {
      expect(headingAnchors(markdown)).toContain(anchor);
    }
  });
});
