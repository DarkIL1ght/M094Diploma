(() => {
  const root = document.documentElement;
  const choices = {
    theme: ['system', 'light', 'dark'],
    accent: ['none', 'red', 'orange', 'amber', 'green', 'teal', 'blue', 'violet', 'pink'],
  };

  for (const [name, values] of Object.entries(choices)) {
    let saved;
    try {
      saved = localStorage.getItem(`m094.${name}`);
    } catch {
      // Без хранилища остаётся доступен выбор на время текущей страницы.
    }
    root.dataset[name] = values.includes(saved) ? saved : values[0];
  }

  document.addEventListener('change', ({ target }) => {
    if (!target.matches('select[data-appearance]')) return;
    const { name, value } = target;
    if (!Object.hasOwn(choices, name) || !choices[name].includes(value)) return;

    root.dataset[name] = value;
    try {
      localStorage.setItem(`m094.${name}`, value);
    } catch {
      // Текущий выбор уже применён к странице.
    }
  });
})();
