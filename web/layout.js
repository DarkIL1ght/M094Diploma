const desktopLinks = [
  ['/', 'Главная', 'home'],
  ['/learn', 'Учёба', 'learn'],
  ['/practice', 'Практика', 'practice'],
  ['/kt', 'Пробный КТ', 'kt'],
  ['/ai', 'ИИ-наставник', 'ai'],
  ['/plan', 'План и новости', 'plan'],
];

const mobileLinks = [
  ...desktopLinks.slice(0, 3),
  ['/ai', 'ИИ', 'ai'],
  ['/me', 'Профиль', 'profile'],
];

const menuLinks = [
  ...desktopLinks.slice(0, 5),
  ['/plan', 'План до КТ', 'plan'],
  ['/news', 'Новости и даты', 'plan'],
  ['/me', 'Профиль', 'profile'],
  ['/league', 'Лига недели', 'profile'],
  ['/me/achievements', 'Достижения', 'profile'],
  ['/me/cosmetics', 'Косметика', 'profile'],
  ['/settings', 'Настройки', 'profile'],
];

export function pageLink(href, label) {
  const link = document.createElement('a');
  link.href = href;
  link.textContent = label;
  link.setAttribute('data-route', '');
  return link;
}

function populateLinks(container, items, route, groupSections = false) {
  const exactLink = items.find(([href]) => href === route.path);
  const currentLink = exactLink ?? (groupSections && items.find(([, , section]) => section === route.section));

  for (const item of items) {
    const [href, label] = item;
    const link = pageLink(href, label);
    if (item === currentLink) link.setAttribute('aria-current', exactLink ? 'page' : 'location');
    container.append(link);
  }
}

export function createLayout(root) {
  root.innerHTML = `
    <a class="skip-link" href="#main">К содержанию</a>
    <header class="site-header">
      <div class="header-inner">
        <button class="menu-open" type="button" aria-controls="site-menu" aria-haspopup="dialog" aria-expanded="false">☰ <span>Меню</span></button>
        <a class="brand" href="/" data-route>ExamPrep</a>
        <nav class="desktop-nav" aria-label="Основная навигация"></nav>
        <nav class="account-nav" aria-label="Аккаунт"></nav>
      </div>
    </header>
    <dialog class="site-menu" id="site-menu" aria-labelledby="menu-title">
      <div class="menu-heading">
        <h2 id="menu-title">Разделы сайта</h2>
        <button class="menu-close" type="button" aria-label="Закрыть меню" autofocus>×</button>
      </div>
      <nav class="drawer-nav" aria-label="Все разделы"></nav>
    </dialog>
    <main id="main" tabindex="-1"></main>
    <nav class="mobile-nav" aria-label="Нижние вкладки"></nav>
  `;

  const header = root.querySelector('.site-header');
  const desktop = root.querySelector('.desktop-nav');
  const account = root.querySelector('.account-nav');
  const mobile = root.querySelector('.mobile-nav');
  const drawer = root.querySelector('.drawer-nav');
  const dialog = root.querySelector('.site-menu');
  const openButton = root.querySelector('.menu-open');
  const main = root.querySelector('main');

  openButton.addEventListener('click', () => {
    dialog.showModal();
    openButton.setAttribute('aria-expanded', 'true');
  });
  root.querySelector('.menu-close').addEventListener('click', () => dialog.close());
  dialog.addEventListener('close', () => openButton.setAttribute('aria-expanded', 'false'));
  window.matchMedia('(min-width: 1024px)').addEventListener('change', (event) => {
    if (event.matches && dialog.open) dialog.close();
  });

  return (route, content, focus) => {
    if (dialog.open) dialog.close();
    const exam = route.exam === true;
    header.hidden = exam;
    mobile.hidden = exam;

    desktop.replaceChildren();
    account.replaceChildren();
    drawer.replaceChildren();
    mobile.replaceChildren();
    if (!exam) {
      populateLinks(desktop, desktopLinks, route, true);
      populateLinks(account, [['/me', 'Профиль', 'profile'], ['/settings', 'Настройки', 'profile']], route, true);
      populateLinks(drawer, menuLinks, route, true);
      populateLinks(mobile, mobileLinks, route, true);
    }

    root.classList.toggle('exam-layout', exam);
    main.replaceChildren(content);
    if (focus) {
      main.focus({ preventScroll: true });
      window.scrollTo(0, 0);
    }
  };
}
