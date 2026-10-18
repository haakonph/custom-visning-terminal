import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';

function report(base, head, markdown = '```mermaid\nerDiagram\n    accounts {\n        INTEGER id PK\n    }\n```\n') {
  const dir = mkdtempSync(join(tmpdir(), 'schema-test-'));
  try {
    writeFileSync(join(dir, 'base.sql'), base);
    writeFileSync(join(dir, 'schema.sql'), head);
    writeFileSync(join(dir, 'schema.md'), markdown);
    const result = spawnSync(process.execPath, ['scripts/schema-report.mjs', join(dir, 'base.sql'), dir, join(dir, 'report.md')], { encoding: 'utf8' });
    return { ...result, body: result.status === 0 ? readFileSync(join(dir, 'report.md'), 'utf8') : '' };
  } finally { rmSync(dir, { recursive: true, force: true }); }
}

test('report includes schema diff and diagram in one marked comment', () => {
  const r = report('', 'CREATE TABLE accounts(id INTEGER PRIMARY KEY);\n');
  assert.equal(r.status, 0, r.stderr);
  assert.ok(r.body.startsWith('<!-- sqlite-schema-docs -->'));
  assert.match(r.body, /\+CREATE TABLE accounts/);
  assert.match(r.body, /```mermaid\nerDiagram/);
});
test('unchanged schemas are explicit', () => {
  assert.match(report('same\n', 'same\n').body, /No schema changes/);
});
test('SQL fences cannot terminate the diff block', () => {
  assert.match(report('', '```\n').body, /````diff\n/);
});
test('large diffs are bounded without removing diagram', () => {
  const r = report('', 'long SQL line\n'.repeat(10000));
  assert.equal(r.status, 0, r.stderr);
  assert.ok(r.body.length < 60000);
  assert.match(r.body, /diff truncated/);
  assert.match(r.body, /erDiagram/);
});
test('oversized diagram fails rather than posting invalid truncated Mermaid', () => {
  assert.notEqual(report('', '', 'x'.repeat(60000)).status, 0);
});
