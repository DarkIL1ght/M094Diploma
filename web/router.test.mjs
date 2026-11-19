import assert from 'node:assert/strict';
import { test } from 'node:test';
import { matchRoute } from './router.js';
import { notFound } from './routes.js';

test('распознаёт главную и вложенные статические маршруты', () => {
  for (const path of ['/', '/practice/gaps', '/ai/history']) {
    const { route, params } = matchRoute(path);
    assert.equal(route.path, path);
    assert.deepEqual(params, {});
  }
});

test('извлекает и декодирует параметры вложенного маршрута', () => {
  const { route, params } = matchRoute('/learn/eng/grammar/intro%20part');
  assert.equal(route.path, '/learn/:subject/:topic/:section');
  assert.deepEqual(params, { subject: 'eng', topic: 'grammar', section: 'intro part' });
});

test('фиксированные маршруты /kt/new и /kt/new/:direction имеют приоритет', () => {
  assert.equal(matchRoute('/kt/new').route.path, '/kt/new');
  const { route, params } = matchRoute('/kt/new/profile');
  assert.equal(route.path, '/kt/new/:direction');
  assert.deepEqual(params, { direction: 'profile' });
  assert.equal(matchRoute('/kt/42').route.exam, true);
});

test('принимает завершающий слэш', () => {
  assert.equal(matchRoute('/learn/').route.path, '/learn');
});

test('возвращает 404 для неизвестного или неполного маршрута', () => {
  assert.equal(matchRoute('/nope').route, notFound);
  assert.equal(matchRoute('/learn/eng/grammar').route, notFound);
});

test('возвращает 404 при некорректной URL-кодировке', () => {
  assert.equal(matchRoute('/learn/%E0%A4%A').route, notFound);
});
