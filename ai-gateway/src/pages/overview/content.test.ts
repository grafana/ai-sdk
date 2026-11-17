import fs from 'fs';
import path from 'path';
import { LINKS } from './content';

const REPOSITORY_ROOT = path.resolve(__dirname, '../../../..');
const BLOB_PREFIX = `${LINKS.repository}/blob/main/`;

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

const repositoryLinks = Object.entries(LINKS).filter(([, href]) => href.startsWith(BLOB_PREFIX));

describe('overview links', () => {
  test.each(repositoryLinks)('%s resolves to repository documentation', (_, href) => {
    const [file, anchor] = href.slice(BLOB_PREFIX.length).split('#');
    const markdown = fs.readFileSync(path.join(REPOSITORY_ROOT, file), 'utf8');

    if (anchor) {
      expect(headingAnchors(markdown)).toContain(anchor);
    }
  });
});
