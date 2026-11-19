import { createBadge, createButton, createInput, createModal, createSkeleton, createTabs, createToastRegion } from './components.js';

const target = (name) => document.querySelector(`#demo-${name}`);

for (const [variant, label] of [
  ['primary', 'Основная'], ['secondary', 'Вторичная'], ['ghost', 'Без фона'], ['danger', 'Опасное действие'],
]) {
  const row = document.createElement('div');
  row.className = 'catalog-row';
  for (const size of ['medium', 'small']) {
    row.append(createButton(size === 'small' ? `${label}, малая` : label, { variant, size }));
    row.append(createButton(size === 'small' ? 'Малая недоступна' : 'Недоступна', { variant, size, disabled: true }));
  }
  target('buttons').append(row);
}

for (const options of [
  { label: 'Название темы', placeholder: 'Например, сортировки', helper: 'Введите ключевое слово.' },
  { label: 'Дата экзамена', placeholder: 'ДД.ММ.ГГГГ', error: 'Укажите дату в формате ДД.ММ.ГГГГ.' },
  { label: 'Код попытки', placeholder: 'Код появится после старта', disabled: true },
]) target('inputs').append(createInput(options).field);

target('tabs').append(createTabs('Демонстрация вкладок', [
  { label: 'Обзор', content: 'Здесь краткое описание темы.' },
  { label: 'Практика', disabled: true },
  { label: 'История', content: 'У этой темы ещё нет попыток.' },
]));

const statusTones = [['info', 'Информация'], ['success', 'Успех'], ['warning', 'Внимание'], ['error', 'Ошибка']];
for (const [tone, label] of [
  ['neutral', 'Нейтральный'], ...statusTones,
  ['red', 'Красный'], ['orange', 'Оранжевый'], ['amber', 'Янтарный'], ['green', 'Зелёный'],
  ['teal', 'Бирюзовый'], ['blue', 'Синий'], ['violet', 'Фиолетовый'], ['pink', 'Розовый'],
]) target('badges').append(createBadge(label, { tone }));

const toastList = createToastRegion();
const toastButtons = document.createElement('div');
toastButtons.className = 'catalog-row';
const messages = ['Напоминание о занятии.', 'Выбор сохранён.', 'Проверьте дату экзамена.', 'Не удалось сохранить данные.'];
for (const [index, [tone, title]] of statusTones.entries()) {
  const button = createButton(title, { variant: 'secondary' });
  button.addEventListener('click', () => toastList.show({ tone, title, message: messages[index] }));
  toastButtons.append(button);
}
target('toasts').append(toastButtons, toastList.region);

for (const [variant, label, title, body, confirmLabel] of [
  ['default', 'Открыть диалог', 'Начать тренировку?', 'На прохождение можно потратить столько времени, сколько нужно.', 'Начать'],
  ['destructive', 'Открыть подтверждение удаления', 'Удалить черновик?', 'Черновик будет удалён из списка попыток.', 'Удалить'],
]) {
  const dialog = createModal({ title, body, variant, confirmLabel });
  const button = createButton(label, { variant: variant === 'destructive' ? 'danger' : 'secondary' });
  button.addEventListener('click', () => dialog.showModal());
  target('modals').append(button, dialog);
}

for (const [shape, label] of [
  ['text', 'Текст'], ['title', 'Заголовок'], ['avatar', 'Аватар'], ['image', 'Изображение'], ['button', 'Кнопка'],
]) {
  const figure = document.createElement('figure');
  const caption = document.createElement('figcaption');
  caption.textContent = label;
  figure.append(createSkeleton(shape), caption);
  target('skeletons').append(figure);
}
