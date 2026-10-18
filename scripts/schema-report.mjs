import { readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';

const [base, head, output] = process.argv.slice(2);
if (!base || !head || !output) throw new Error('usage: schema-report.mjs BASE_SQL HEAD_DOCS OUTPUT');
const diff = spawnSync('diff', ['-u', '--label', 'main/schema.sql', '--label', 'PR/schema.sql', base, `${head}/schema.sql`], { encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });
if (diff.error || ![0, 1].includes(diff.status)) throw diff.error ?? new Error(diff.stderr);
const markdown = readFileSync(`${head}/schema.md`, 'utf8');
// Fence length also handles backticks in SQL identifiers and string literals.
const fence = '`'.repeat(Math.max(3, ...[...diff.stdout.matchAll(/`+/g)].map(m => m[0].length + 1)));
const budget = 58000 - markdown.length;
if (budget < 1000) throw new Error('Schema diagram exceeds GitHub comment size; split the schema documentation');
const changes = diff.stdout.length > budget ? diff.stdout.slice(0, budget) + '\n… diff truncated; see workflow artifact for full SQL.\n' : diff.stdout;
const body = `<!-- sqlite-schema-docs -->\n## SQLite schema compared with main\n\n${diff.status === 0 ? 'No schema changes.\n' : `${fence}diff\n${changes}${fence}\n`}\n${markdown}\nRegenerate committed documentation with \`npm run db:docs\`. The CI check reports whether it is up to date.\n`;
if (body.length > 60000) throw new Error('Schema report exceeds comment size limit');
writeFileSync(output, body);
