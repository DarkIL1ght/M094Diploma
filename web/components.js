let nextId = 0;

function element(tag, className, text) {
  const node = document.createElement(tag);
  node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

export function createButton(label, { variant = 'primary', size = 'medium', disabled = false, type = 'button' } = {}) {
  const button = element('button', 'ui-button', label);
  button.dataset.variant = variant;
  button.dataset.size = size;
  button.type = type;
  button.disabled = disabled;
  return button;
}

export function createInput({ label, name, type = 'text', placeholder = '', helper, showHelper = true, error, disabled = false }) {
  const field = element('div', 'ui-field');
  const caption = element('label', 'ui-field-label', label);
  const input = element('input', 'ui-input');
  input.id = `ui-input-${++nextId}`;
  caption.htmlFor = input.id;
  if (name !== undefined) input.name = name;
  input.type = type;
  input.placeholder = placeholder;
  input.disabled = disabled;
  field.append(caption, input);

  if (error) input.setAttribute('aria-invalid', 'true');
  const message = error || (showHelper && helper);
  if (message) {
    const hint = element('div', 'ui-field-hint', message);
    hint.id = `${input.id}-hint`;
    input.setAttribute('aria-describedby', hint.id);
    field.append(hint);
  }

  return { field, input };
}

export function createTabs(label, items) {
  const id = `ui-tabs-${++nextId}`;
  const group = element('div', 'ui-tabs');
  const list = element('div', 'ui-tab-list');
  list.setAttribute('role', 'tablist');
  list.setAttribute('aria-label', label);
  group.append(list);

  const entries = items.map(({ label: title, content, disabled = false }, index) => {
    const tab = element('button', 'ui-tab', title);
    tab.type = 'button';
    tab.disabled = disabled;
    tab.id = `${id}-tab-${index}`;
    tab.setAttribute('role', 'tab');
    const panel = element('div', 'ui-tab-panel');
    panel.id = `${id}-panel-${index}`;
    panel.setAttribute('role', 'tabpanel');
    panel.setAttribute('aria-labelledby', tab.id);
    panel.tabIndex = 0;
    if (content !== undefined) panel.append(content);
    tab.setAttribute('aria-controls', panel.id);
    list.append(tab);
    group.append(panel);
    tab.addEventListener('click', () => select(tab));
    return { tab, panel };
  });

  function select(activeTab) {
    for (const { tab, panel } of entries) {
      const selected = tab === activeTab;
      tab.setAttribute('aria-selected', String(selected));
      tab.tabIndex = selected ? 0 : -1;
      panel.hidden = !selected;
    }
  }

  const enabled = entries.map(({ tab }) => tab).filter((tab) => !tab.disabled);
  select(enabled[0]);
  list.addEventListener('keydown', (event) => {
    const index = enabled.indexOf(event.target);
    if (index === -1) return;
    const next = {
      ArrowLeft: (index - 1 + enabled.length) % enabled.length,
      ArrowRight: (index + 1) % enabled.length,
      Home: 0,
      End: enabled.length - 1,
    };
    if (!Object.hasOwn(next, event.key)) return;
    event.preventDefault();
    enabled[next[event.key]].focus();
  });

  return group;
}

export function createBadge(label, { tone = 'neutral' } = {}) {
  const badge = element('span', 'ui-badge', label);
  badge.dataset.tone = tone;
  return badge;
}

export function createToastRegion() {
  const region = element('div', 'ui-toasts');
  const announcement = element('div', 'ui-visually-hidden');
  announcement.setAttribute('role', 'status');
  region.append(announcement);

  function show({ title, message, tone = 'info' }) {
    const previousFocus = document.activeElement;
    const toast = element('div', 'ui-toast');
    toast.dataset.tone = tone;
    const text = element('div', 'ui-toast-text');
    text.append(element('strong', 'ui-toast-title', title), element('p', 'ui-toast-message', message));
    const dismiss = createButton('Закрыть', { variant: 'ghost', size: 'small' });
    dismiss.setAttribute('aria-label', `Закрыть уведомление «${title}»`);
    dismiss.addEventListener('click', () => {
      const restoreFocus = toast.contains(document.activeElement);
      toast.remove();
      if (restoreFocus && previousFocus?.isConnected) previousFocus.focus();
    });
    toast.append(text, dismiss);
    region.append(toast);
    announcement.replaceChildren(`${title}. ${message}`);
    return toast;
  }

  return { region, show };
}

export function createModal({ title, body, variant = 'default', confirmLabel = 'Подтвердить', cancelLabel = 'Отмена' }) {
  const id = `ui-modal-${++nextId}`;
  const dialog = element('dialog', 'ui-modal');
  const heading = element('h2', 'ui-modal-title', title);
  heading.id = `${id}-title`;
  dialog.setAttribute('aria-labelledby', heading.id);
  const content = element('div', 'ui-modal-body');
  content.id = `${id}-body`;
  if (typeof body === 'string') dialog.setAttribute('aria-describedby', content.id);
  if (body !== undefined) content.append(body);
  const footer = element('div', 'ui-modal-footer');
  const cancel = createButton(cancelLabel, { variant: 'secondary' });
  cancel.autofocus = true;
  cancel.addEventListener('click', () => dialog.close('cancel'));
  dialog.addEventListener('cancel', () => { dialog.returnValue = 'cancel'; });
  const confirm = createButton(confirmLabel, { variant: variant === 'destructive' ? 'danger' : 'primary' });
  confirm.addEventListener('click', () => dialog.close('confirm'));
  footer.append(cancel, confirm);
  dialog.append(heading, content, footer);
  return dialog;
}

export function createSkeleton(shape = 'text') {
  const skeleton = element('span', 'ui-skeleton');
  skeleton.dataset.shape = shape;
  skeleton.setAttribute('aria-hidden', 'true');
  return skeleton;
}
