import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { uniqueEvents } from './otlp-unique.mjs';

test('lists names across signals and keeps unknown attributes and examples', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'otlp-'));
  try {
    const path = join(dir, 'events.jsonl');
    const payload = {
      resourceLogs: [{ scopeLogs: [{ logRecords: [
        { eventName: 'agent.start', attributes: [{ key: 'new.field', value: { stringValue: 'test' } }] },
        { attributes: [{ key: 'event.name', value: { stringValue: 'agent.start' } }] },
        { body: { stringValue: 'unnamed' } }
      ] }] }],
      resourceSpans: [{ scopeSpans: [{ spans: [{ name: 'agent', events: [{ name: 'tool.call' }] }] }] }],
      resourceMetrics: [{ scopeMetrics: [{ metrics: [{ name: 'tokens' }] }] }]
    };
    await writeFile(path, JSON.stringify({ payload }) + '\n');
    const groups = await uniqueEvents(path);
    assert.equal(groups.length, 5);
    const start = groups.find(g => g.name === 'agent.start');
    assert.equal(start.count, 2);
    assert.deepEqual(start.attribute_keys, ['event.name', 'new.field']);
    assert.equal(start.example.attributes[0].value.stringValue, 'test');
  } finally { await rm(dir, { recursive: true, force: true }); }
});
