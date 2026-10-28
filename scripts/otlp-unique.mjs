import { createReadStream } from 'node:fs';
import { createInterface } from 'node:readline';

// Group by signal and emitted name, keeping an untouched example for inspection.
export async function uniqueEvents(path) {
  const groups = new Map();
  function add(signal, item) {
    const attrs = Object.fromEntries((item.attributes ?? []).map(a => [a.key, a.value?.stringValue]));
    const name = item.eventName || attrs['event.name'] || attrs['event_name'] || item.name || '(uten navn)';
    const key = JSON.stringify([signal, name]);
    const group = groups.get(key) ?? { signal, name, count: 0, attribute_keys: [], example: item };
    group.count++;
    group.attribute_keys = [...new Set([...group.attribute_keys, ...(item.attributes ?? []).map(a => a.key)])].sort();
    groups.set(key, group);
  }
  const lines = createInterface({ input: createReadStream(path), crlfDelay: Infinity });
  let lineNumber = 0;
  for await (const line of lines) {
    lineNumber++;
    if (!line.trim()) continue;
    let record;
    try { record = JSON.parse(line); } catch { throw new Error(`Ugyldig JSON på linje ${lineNumber}`); }
    const payload = record.payload;
    if (!payload) throw new Error(`Mangler payload på linje ${lineNumber}`);
    for (const resource of payload.resourceLogs ?? [])
      for (const scope of resource.scopeLogs ?? [])
        for (const log of scope.logRecords ?? []) add('logs', log);
    for (const resource of payload.resourceSpans ?? [])
      for (const scope of resource.scopeSpans ?? [])
        for (const span of scope.spans ?? []) {
          add('traces', span);
          for (const event of span.events ?? []) add('span_events', event);
        }
    for (const resource of payload.resourceMetrics ?? [])
      for (const scope of resource.scopeMetrics ?? [])
        for (const metric of scope.metrics ?? []) add('metrics', metric);
  }
  return [...groups.values()].sort((a, b) => a.signal.localeCompare(b.signal) || a.name.localeCompare(b.name));
}

if (process.argv[1] && import.meta.url === new URL(process.argv[1], 'file:').href) {
  if (!process.argv[2]) { console.error('Bruk: node scripts/otlp-unique.mjs /absolutt/sti/otlp.jsonl'); process.exitCode = 1; }
  else {
    try { console.log(JSON.stringify(await uniqueEvents(process.argv[2]), null, 2)); }
    catch (err) { console.error(err.message); process.exitCode = 1; }
  }
}
