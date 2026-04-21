// docs.gormes.ai interactive behavior. Vanilla, no deps.
// Loaded deferred from baseof.html. Runs on DOMContentLoaded.
(function () {
  'use strict';

  function onReady(fn) {
    if (document.readyState !== 'loading') fn();
    else document.addEventListener('DOMContentLoaded', fn);
  }

  // Populated by later tasks: drawer (Task 2/3), collapsibles (Task 4), scrollspy (Task 7).
  onReady(function () {
    // intentionally empty — wiring lands in subsequent tasks
  });
})();
