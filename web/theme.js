(() => {
  const root = document.documentElement;
  const accentOptions = document.getElementById('accent-options').content.querySelectorAll('option');
  const accentColors = new Map(Array.from(accentOptions, (option) => [option.value, option.dataset.color]));
  const accentIds = new Map(Array.from(accentColors, ([id, color]) => [color, id]));
  const choices = {
    theme: ['system', 'light', 'dark'],
    accent: [...accentColors.keys()],
  };

  function applyPreference(name, value) {
    root.dataset[name] = value;
    if (name === 'accent') root.dataset.accentColor = accentColors.get(value);
  }

  function savePreference(name, value) {
    try {
      localStorage.setItem(`m094.${name}`, value);
    } catch {
      // Текущий выбор уже применён к странице.
    }
  }

  for (const [name, values] of Object.entries(choices)) {
    let saved;
    try {
      saved = localStorage.getItem(`m094.${name}`);
    } catch {
      // Без хранилища остаётся доступен выбор на время текущей страницы.
    }
    const value = values.includes(saved) ? saved
      : (name === 'accent' && accentIds.get(saved)) || values[0];
    applyPreference(name, value);
    if (name === 'accent' && saved != null && saved !== value) savePreference(name, value);
  }

  document.addEventListener('change', ({ target }) => {
    if (!target.matches('select[data-appearance]')) return;
    const { name, value } = target;
    if (!Object.hasOwn(choices, name) || !choices[name].includes(value)) return;

    applyPreference(name, value);
    savePreference(name, value);
  });
})();
