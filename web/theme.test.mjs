import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { runInNewContext } from 'node:vm';

const source = readFileSync(new URL('./theme.js', import.meta.url), 'utf8');

function startTheme(storage) {
  const dataset = {};
  let onChange;
  const document = {
    documentElement: { dataset },
    addEventListener(event, listener) {
      assert.equal(event, 'change');
      onChange = listener;
    },
  };
  runInNewContext(source, { document, localStorage: storage });
  return {
    dataset,
    change(name, value, appearanceControl = true) {
      onChange({ target: { name, value, matches: () => appearanceControl } });
    },
  };
}

function memoryStorage(values = {}) {
  return {
    getItem: (key) => values[key] ?? null,
    setItem: (key, value) => { values[key] = value; },
  };
}

test('восстанавливает сохранённые тему и акцент при запуске', () => {
  const { dataset } = startTheme(memoryStorage({ 'm094.theme': 'dark', 'm094.accent': 'teal' }));
  assert.deepEqual(dataset, { theme: 'dark', accent: 'teal' });
});

test('выбирает системную тему без акцента при неизвестных значениях', () => {
  const { dataset } = startTheme(memoryStorage({ 'm094.theme': 'invalid', 'm094.accent': 'invalid' }));
  assert.deepEqual(dataset, { theme: 'system', accent: 'none' });
});

test('сохраняет выбор и игнорирует изменения других полей и неизвестные опции', () => {
  const storage = memoryStorage();
  const { dataset, change } = startTheme(storage);
  change('theme', 'light');
  change('accent', 'violet');
  change('theme', 'invalid');
  change('accent', 'red', false);
  change('unrelated', 'value');

  assert.deepEqual(dataset, { theme: 'light', accent: 'violet' });
  assert.deepEqual(startTheme(storage).dataset, dataset);
});

test('переключает тему при недоступном хранилище', () => {
  function denied() { throw new Error('Storage unavailable'); }
  const { dataset, change } = startTheme({ getItem: denied, setItem: denied });
  assert.deepEqual(dataset, { theme: 'system', accent: 'none' });

  change('theme', 'dark');
  assert.equal(dataset.theme, 'dark');
});
