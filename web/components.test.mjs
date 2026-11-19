import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createBadge, createButton, createInput, createModal, createSkeleton, createTabs, createToastRegion } from './components.js';

function useDocument(t) {
  const document = { activeElement: null };
  document.createElement = (tagName) => {
    const attributes = new Map();
    const listeners = new Map();
    return {
      tagName, children: [], dataset: {}, disabled: false,
      setAttribute(name, value) { attributes.set(name, value); },
      getAttribute(name) { return attributes.get(name); },
      append(...children) { this.children.push(...children); },
      replaceChildren(...children) { this.children = children; },
      addEventListener(event, listener) { listeners.set(event, listener); },
      emit(event, data) { listeners.get(event)?.(data); },
      focus() { document.activeElement = this; },
      close(value) { this.returnValue = value; },
    };
  };
  const prior = Object.getOwnPropertyDescriptor(globalThis, 'document');
  Object.defineProperty(globalThis, 'document', { value: document, configurable: true });
  t.after(() => {
    if (prior) Object.defineProperty(globalThis, 'document', prior);
    else delete globalThis.document;
  });
  return document;
}

test('кнопка сохраняет свойства, а поле связывает подпись и ошибку', (t) => {
  useDocument(t);
  const button = createButton('Продолжить', { variant: 'ghost', size: 'small', disabled: true });
  assert.deepEqual([button.tagName, button.type, button.disabled], ['button', 'button', true]);
  assert.deepEqual(button.dataset, { variant: 'ghost', size: 'small' });

  const { field, input } = createInput({ label: 'Дата', helper: 'Подсказка', error: 'Введите дату' });
  assert.equal(field.children[0].htmlFor, input.id);
  assert.equal(input.getAttribute('aria-invalid'), 'true');
  assert.equal(input.getAttribute('aria-describedby'), field.children[2].id);
  assert.equal(field.children[2].textContent, 'Введите дату');
  const hiddenHelper = createInput({ label: 'Имя', helper: 'Введите имя', showHelper: false });
  assert.equal(hiddenHelper.field.children.length, 2);
});

test('стрелки пропускают недоступную вкладку, а активация меняет видимую панель', (t) => {
  const document = useDocument(t);
  const group = createTabs('Разделы', [{ label: 'А' }, { label: 'Б', disabled: true }, { label: 'В' }]);
  const [list, firstPanel, , lastPanel] = group.children;
  const [first, , last] = list.children;
  assert.equal(first.getAttribute('aria-controls'), firstPanel.id);
  assert.equal(firstPanel.hidden, false);
  assert.equal(lastPanel.hidden, true);

  let prevented = false;
  list.emit('keydown', { target: first, key: 'ArrowRight', preventDefault() { prevented = true; } });
  assert.equal(prevented, true);
  assert.equal(document.activeElement, last);
  assert.equal(first.getAttribute('aria-selected'), 'true');
  last.emit('click');
  assert.equal(firstPanel.hidden, true);
  assert.equal(lastPanel.hidden, false);
  assert.equal(last.tabIndex, 0);
});

test('диалог помечает опасное действие и различает подтверждение и отмену', (t) => {
  useDocument(t);
  const dialog = createModal({ title: 'Удалить?', body: 'Текст', variant: 'destructive' });
  const [title, body, footer] = dialog.children;
  const [cancel, confirm] = footer.children;
  assert.equal(dialog.getAttribute('aria-labelledby'), title.id);
  assert.equal(dialog.getAttribute('aria-describedby'), body.id);
  assert.equal(cancel.autofocus, true);
  assert.equal(confirm.dataset.variant, 'danger');
  confirm.emit('click');
  assert.equal(dialog.returnValue, 'confirm');
  dialog.emit('cancel');
  assert.equal(dialog.returnValue, 'cancel');
});

test('тост объявляет текст, бейдж хранит тон, скелетон скрыт от диктора', (t) => {
  useDocument(t);
  const toasts = createToastRegion();
  const toast = toasts.show({ title: 'Сохранено', message: 'Настройки применены', tone: 'success' });
  assert.equal(toast.dataset.tone, 'success');
  assert.equal(toasts.region.children[0].getAttribute('role'), 'status');
  assert.equal(toasts.region.children[0].children[0], 'Сохранено. Настройки применены');
  assert.equal(createBadge('Акцент', { tone: 'violet' }).dataset.tone, 'violet');
  assert.equal(createSkeleton('avatar').getAttribute('aria-hidden'), 'true');
});
