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

function createNavigation(container, items) {
  const entries = items.map(([href, label, section]) => {
    const link = pageLink(href, label);
    container.append(link);
    return { href, section, link };
  });

  let current;
  return (route) => {
    const exact = entries.find(({ href }) => href === route.path);
    const next = exact ?? entries.find(({ section }) => section === route.section);
    if (current !== next) current?.link.removeAttribute('aria-current');
    next?.link.setAttribute('aria-current', exact ? 'page' : 'location');
    current = next;
  };
}

export function createLayout(root) {
  root.innerHTML = `
    <a class="skip-link" href="#main">К содержанию</a>
    <header class="site-header">
      <div class="header-inner">
        <button class="menu-open" type="button" aria-controls="site-menu" aria-haspopup="dialog" aria-expanded="false"><span aria-hidden="true">☰</span> Меню</button>
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
  const mobile = root.querySelector('.mobile-nav');
  const dialog = root.querySelector('.site-menu');
  const openButton = root.querySelector('.menu-open');
  const brand = root.querySelector('.brand');
  const main = root.querySelector('main');
  const markDesktop = createNavigation(root.querySelector('.desktop-nav'), desktopLinks);
  const markAccount = createNavigation(root.querySelector('.account-nav'), [
    ['/me', 'Профиль', 'profile'], ['/settings', 'Настройки', 'profile'],
  ]);
  const markDrawer = createNavigation(root.querySelector('.drawer-nav'), menuLinks);
  const markMobile = createNavigation(mobile, mobileLinks);

  root.querySelector('.skip-link').addEventListener('click', (event) => {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    (main.querySelector('h1') ?? main).focus();
  });

  openButton.addEventListener('click', () => {
    dialog.showModal();
    openButton.setAttribute('aria-expanded', 'true');
  });
  root.querySelector('.menu-close').addEventListener('click', () => dialog.close());
  dialog.addEventListener('close', () => openButton.setAttribute('aria-expanded', 'false'));
  window.matchMedia('(min-width: 1024px)').addEventListener('change', (event) => {
    if (event.matches && dialog.open) {
      dialog.close();
      brand.focus();
    }
  });

  return (route, content, focus) => {
    if (dialog.open) dialog.close();
    const exam = route.exam === true;
    header.hidden = exam;
    mobile.hidden = exam;

    markDesktop(route);
    markAccount(route);
    markDrawer(route);
    markMobile(route);

    root.classList.toggle('exam-layout', exam);
    const heading = content.querySelector('h1');
    if (heading) heading.tabIndex = -1;
    main.replaceChildren(content);
    if (focus) {
      window.scrollTo(0, 0);
      (heading ?? main).focus();
    }
  };
}
