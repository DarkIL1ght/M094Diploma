import { pageLink } from './layout.js';
import { notFound } from './routes.js';

const subjects = [
  ['eng', 'Английский язык'],
  ['tgo', 'Тест на готовность к обучению'],
  ['algo', 'Алгоритмы'],
  ['db', 'Базы данных'],
];

const relatedPages = {
  '/': [
    ['/learn', 'Учёба'], ['/practice', 'Практика'], ['/kt', 'Пробный КТ'],
    ['/ai', 'ИИ-наставник'], ['/plan', 'План до КТ'], ['/news', 'Новости и даты'],
  ],
  '/learn': subjects.map(([id, label]) => [`/learn/${id}`, label]),
  '/practice': [
    ['/practice/mistakes', 'Работа над ошибками'], ['/practice/gaps', 'Карта дыр'],
    ['/practice/mixed', 'Смешанный тест'],
  ],
  '/kt': [['/kt/new', 'Выбрать направление']],
  '/ai': [['/ai/history', 'История чатов']],
  '/me': [
    ['/me/achievements', 'Достижения'], ['/league', 'Лига недели'],
    ['/me/cosmetics', 'Косметика'], ['/settings', 'Настройки и перенос'],
  ],
  '/plan': [['/news', 'Новости и даты']],
  '/news': [['/plan', 'План до КТ']],
};

function linkList(items, className) {
  const list = document.createElement('ul');
  list.className = className;
  for (const [href, label] of items) {
    const item = document.createElement('li');
    item.append(pageLink(href, label));
    list.append(item);
  }
  return list;
}

function appearanceSettings() {
  const settings = document.createElement('fieldset');
  settings.className = 'appearance-settings';
  settings.innerHTML = `
    <legend>Оформление</legend>
    <label>Тема
      <select name="theme" data-appearance>
        <option value="system">Как в системе</option>
        <option value="light">Светлая</option>
        <option value="dark">Тёмная</option>
      </select>
    </label>
    <label>Акцент
      <select name="accent" data-appearance>
        <option value="none">Без акцента</option>
        <option value="red">Красный</option>
        <option value="orange">Оранжевый</option>
        <option value="amber">Янтарный</option>
        <option value="green">Зелёный</option>
        <option value="teal">Бирюзовый</option>
        <option value="blue">Синий</option>
        <option value="violet">Фиолетовый</option>
        <option value="pink">Розовый</option>
      </select>
    </label>`;

  for (const control of settings.querySelectorAll('select')) {
    const value = document.documentElement.dataset[control.name];
    if (value) control.value = value;
  }
  return settings;
}

export function createView(route, params) {
  const page = document.createElement('div');
  page.className = 'page';
  const content = document.createElement('div');
  content.className = 'page-content';
  const heading = document.createElement('h1');
  const description = document.createElement('p');
  description.className = 'page-description';

  heading.textContent = route.title;
  description.textContent = 'Этот раздел появится позже.';
  content.append(heading, description);

  if (route === notFound) {
    description.textContent = 'Проверьте адрес или вернитесь на главную.';
    const home = pageLink('/', 'На главную');
    home.className = 'action';
    content.append(home);
  } else if (route.path === '/') {
    heading.textContent = 'Подготовка к комплексному тестированию';
    description.textContent = 'Для поступающих в магистратуру.';
  } else if (route.path === '/settings') {
    description.replaceWith(appearanceSettings());
  } else if (route.path === '/learn' || route.path === '/practice' || route.path === '/me') {
    description.remove();
  }

  if (relatedPages[route.path]) {
    content.append(linkList(relatedPages[route.path], 'page-links'));
  }

  if (params.subject) {
    const subject = subjects.find(([id]) => id === params.subject);
    const subjectPath = `/learn/${encodeURIComponent(params.subject)}`;
    const localNav = document.createElement('details');
    localNav.className = 'local-nav';
    localNav.open = window.matchMedia('(min-width: 1024px)').matches;
    const summary = document.createElement('summary');
    summary.textContent = params.section ? 'Оглавление' : 'Темы предмета';
    const links = params.section
      ? [[subjectPath, 'К темам предмета']]
      : [[`${subjectPath}/demo/introduction`, 'Пример раздела']];
    localNav.append(summary, linkList(links, 'local-links'));
    page.classList.add('page-with-sidebar');
    page.append(localNav);
    if (subject) heading.textContent = `${route.title} · ${subject[1]}`;
  }

  page.append(content);
  return page;
}
