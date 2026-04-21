// docs.gormes.ai interactive behavior. Vanilla, no deps.
(function () {
  'use strict';

  function onReady(fn) {
    if (document.readyState !== 'loading') fn();
    else document.addEventListener('DOMContentLoaded', fn);
  }

  function setDrawer(state) {
    var sidebar = document.getElementById('docs-sidebar');
    var btn = document.querySelector('[data-testid="drawer-open"]');
    if (!sidebar) return;
    sidebar.setAttribute('data-state', state);
    if (btn) btn.setAttribute('aria-expanded', state === 'open' ? 'true' : 'false');
  }

  function initDrawer() {
    var btn = document.querySelector('[data-testid="drawer-open"]');
    var backdrop = document.querySelector('.drawer-backdrop');
    if (!btn) return;
    btn.addEventListener('click', function () {
      var sidebar = document.getElementById('docs-sidebar');
      var isOpen = sidebar && sidebar.getAttribute('data-state') === 'open';
      setDrawer(isOpen ? 'closed' : 'open');
    });
    if (backdrop) backdrop.addEventListener('click', function () { setDrawer('closed'); });
  }

  onReady(function () {
    initDrawer();
  });
})();
