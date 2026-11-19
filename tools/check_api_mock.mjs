import assert from 'node:assert/strict';
import { createApi } from '../web/api.js';

const baseUrl = process.argv[2] ?? 'http://127.0.0.1:4010';
const api = createApi({ baseUrl });
const { items } = await api.listSubjects({
  headers: { Cookie: 'sid=mock' },
  signal: AbortSignal.timeout(5000),
});

assert.ok(Array.isArray(items) && items.length > 0, 'Prism должен вернуть непустой items');
for (const subject of items) {
  assert.ok(['english', 'tgo', 'algorithms', 'databases'].includes(subject.id));
  assert.equal(typeof subject.title, 'string');
  assert.ok(Number.isInteger(subject.topics_total));
  assert.ok(Number.isInteger(subject.topics_done));
  assert.ok(subject.accuracy === null || (typeof subject.accuracy === 'number' &&
    subject.accuracy >= 0 && subject.accuracy <= 1));
}

console.log(`Prism: GET /api/subjects → ${items.length} записей Subject (${items.map(({ id }) => id).join(', ')}).`);
