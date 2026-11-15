import { createLayout } from './layout.js';
import { startRouter } from './router.js';
import { createView } from './views.js';

const showPage = createLayout(document.querySelector('#app'));

startRouter(({ route, params }, { focus = true } = {}) => {
  document.title = `${route.title} · ExamPrep`;
  showPage(route, createView(route, params), focus);
});
