import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { runInNewContext } from 'node:vm';

const source = readFileSync(new URL('./theme.js', import.meta.url), 'utf8');
const html = readFileSync(new URL('./index.html', import.meta.url), 'utf8');
const styles = readFileSync(new URL('./styles.css', import.meta.url), 'utf8');
const accentOptions = Array.from(html.matchAll(/<option value="(accent-\d)" data-color="([a-z]+)">/g),
  ([, value, color]) => ({ value, dataset: { color } }));

function startTheme(storage) {
  const dataset = {};
  let onChange;
  const document = {
    documentElement: { dataset },
    getElementById(id) {
      assert.equal(id, 'accent-options');
      return { content: { querySelectorAll: () => accentOptions } };
    },
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
  const { dataset } = startTheme(memoryStorage({ 'm094.theme': 'dark', 'm094.accent': 'accent-5' }));
  assert.deepEqual(dataset, { theme: 'dark', accent: 'accent-5', accentColor: 'teal' });
});

test('использует accent-1 при отсутствии выбора, старом none или неизвестном значении', () => {
  for (const old of [null, 'none', 'accent-9', 'invalid']) {
    const storage = memoryStorage({ 'm094.theme': 'invalid', 'm094.accent': old });
    assert.deepEqual(startTheme(storage).dataset, { theme: 'system', accent: 'accent-1', accentColor: 'red' });
    assert.equal(storage.getItem('m094.accent'), old === null ? null : 'accent-1');
  }
});

test('мигрирует все восемь названий цветов в порядке экспорта A-03', () => {
  const palette = JSON.parse(readFileSync(new URL('../design/tokens/Light.tokens.json', import.meta.url), 'utf8'));
  const colors = Object.keys(palette.color.accent).filter((color) => !color.startsWith('$') && !color.endsWith('-subtle'));
  assert.equal(colors.length, 8);
  assert.deepEqual(accentOptions.map(({ dataset }) => dataset.color), colors);
  for (const [index, color] of colors.entries()) {
    const id = `accent-${index + 1}`;
    const storage = memoryStorage({ 'm094.accent': color });
    assert.deepEqual(startTheme(storage).dataset, { theme: 'system', accent: id, accentColor: color });
    assert.equal(storage.getItem('m094.accent'), id);
    assert.ok(styles.includes(`[data-accent-color='${color}'] { --accent: var(--color-accent-${color});`));
  }
});

test('сохраняет выбор и игнорирует изменения других полей и неизвестные опции', () => {
  const storage = memoryStorage();
  const { dataset, change } = startTheme(storage);
  change('theme', 'light');
  change('accent', 'accent-7');
  change('theme', 'invalid');
  change('accent', 'red');
  change('accent', 'accent-1', false);
  change('unrelated', 'value');

  assert.deepEqual(dataset, { theme: 'light', accent: 'accent-7', accentColor: 'violet' });
  assert.deepEqual(startTheme(storage).dataset, dataset);
});

test('переключает тему при недоступном хранилище', () => {
  function denied() { throw new Error('Storage unavailable'); }
  const { dataset, change } = startTheme({ getItem: denied, setItem: denied });
  assert.deepEqual(dataset, { theme: 'system', accent: 'accent-1', accentColor: 'red' });

  change('theme', 'dark');
  assert.equal(dataset.theme, 'dark');
});
