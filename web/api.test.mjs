import assert from 'node:assert/strict';
import { test } from 'node:test';
import { ApiError, createApi } from './api.js';

test('принимает явный credentials: include и возвращает объект items', async (t) => {
  const data = { items: [{ id: 'algorithms', title: 'Алгоритмы', topics_total: 14, topics_done: 9, accuracy: null }] };
  t.mock.method(globalThis, 'fetch', async (url, options) => {
    assert.equal(url, 'http://127.0.0.1:4010/api/subjects');
    assert.equal(options.credentials, 'include');
    assert.equal(options.headers.get('Accept'), 'application/json');
    return Response.json(data);
  });
  const api = createApi({ baseUrl: 'http://127.0.0.1:4010/', credentials: 'include' });
  assert.deepEqual(await api.listSubjects(), data);
});

test('работает без браузерных глобальных переменных и использует same-origin по умолчанию', async (t) => {
  assert.equal(typeof globalThis.window, 'undefined');
  assert.equal(typeof globalThis.document, 'undefined');
  const fetchMock = t.mock.method(globalThis, 'fetch', async (_url, options) => {
    assert.equal(options.credentials, 'same-origin');
    return Response.json({});
  });
  const api = createApi();
  await api.getMe();
  await api.listTopics('algorithms');
  assert.deepEqual(fetchMock.mock.calls.map(({ arguments: [url] }) => url), [
    '/api/me', '/api/subjects/algorithms/topics',
  ]);
});

test('передаёт JSON, метод и сигнал, поддерживает 204', async (t) => {
  const { signal } = new AbortController();
  t.mock.method(globalThis, 'fetch', async (url, options) => {
    assert.equal(url, '/api/me/settings');
    assert.equal(options.method, 'PATCH');
    assert.equal(options.headers.get('Content-Type'), 'application/json');
    assert.equal(options.body, '{"exam_date":null}');
    assert.equal(options.signal, signal);
    return Response.json({ exam_date: null });
  });
  const api = createApi();
  assert.deepEqual(await api.request('/api/me/settings', {
    method: 'PATCH', json: { exam_date: null }, signal,
  }), { exam_date: null });
  t.mock.restoreAll();
  t.mock.method(globalThis, 'fetch', async () => new Response(null, { status: 204 }));
  assert.equal(await api.request('/api/ai/chats/demo', { method: 'DELETE' }), undefined);
});

test('сохраняет HTTP-статус, ошибку из B-01 и Retry-After', async (t) => {
  const details = { limit: 20, resets_at: '2026-11-18T00:00:00Z' };
  t.mock.method(globalThis, 'fetch', async () => Response.json({
    code: 'ai_limit', message: 'Лимит на сегодня исчерпан', details,
  }, { status: 429, headers: { 'Retry-After': '60' } }));
  await assert.rejects(createApi().request('/api/ai/explain', { method: 'POST' }), {
    name: 'ApiError', kind: 'http', status: 429, code: 'ai_limit',
    message: 'Лимит на сегодня исчерпан', details, retryAfter: 60,
  });
});

test('различает не-JSON ошибку HTTP и невалидный успешный ответ', async (t) => {
  const fetchMock = t.mock.method(globalThis, 'fetch', async () => new Response('<p>503</p>', { status: 503 }));
  await assert.rejects(createApi().getMe(), { name: 'ApiError', kind: 'http', status: 503 });
  fetchMock.mock.mockImplementation(async () => new Response('<p>HTML</p>'));
  await assert.rejects(createApi().getMe(), { name: 'ApiError', kind: 'response', status: 200 });
});

test('различает сетевой сбой и отмену запроса', async (t) => {
  const cause = new TypeError('fetch failed');
  const fetchMock = t.mock.method(globalThis, 'fetch', async () => { throw cause; });
  await assert.rejects(createApi().getMe(), (error) =>
    error instanceof ApiError && error.kind === 'network' && error.cause === cause);

  const abort = new DOMException('Aborted', 'AbortError');
  fetchMock.mock.mockImplementation(async () => { throw abort; });
  await assert.rejects(createApi().getMe(), (error) => error === abort);
});
