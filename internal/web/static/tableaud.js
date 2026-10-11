// tableaud.js: the one script of tableaud's own, loaded in <head> after HTMX.
// Two jobs. A page reads in full without either, and without HTMX.
(function () {
  'use strict';

  // The storage key for a path: the two tableaux hold columns, no other view.
  function keyFor(pathname) {
    var view = pathname.replace(/^.*\//, '');
    return view === 'tableau' || view === 'context' ? 'tableaud.columns.' + view : null;
  }

  // The address a load moves to, or null to stay. A tableau address that
  // states neither columns nor a window takes the stored columns; an address
  // that states either is a shared link or a choice just made, and wins.
  function redirectFor(pathname, search, hash, stored) {
    var query = new URLSearchParams(search);
    if (!keyFor(pathname) || !stored || query.has('columns') || query.has('window')) { return null; }
    query.set('columns', stored);
    return pathname + '?' + query.toString() + hash;
  }

  // Under node, for the test: export the two functions and stop.
  if (typeof document === 'undefined') {
    module.exports = { keyFor: keyFor, redirectFor: redirectFor };
    return;
  }

  var store = null;
  try { store = window.localStorage; } catch (e) { store = null; }
  var key = keyFor(location.pathname);

  // Job 1, on load. replace() adds no history entry, so Back does not loop.
  if (store && key) {
    var next = redirectFor(location.pathname, location.search, location.hash, store.getItem(key));
    if (next) { location.replace(next); return; }
  }

  // Job 1, on a choice. A link that changes the columns carries the new list in
  // data-columns; the empty list means the default window and clears the store.
  function keep(columns) {
    var k = keyFor(location.pathname);
    if (!store || !k) { return; }
    if (columns) { store.setItem(k, columns); } else { store.removeItem(k); }
  }
  document.addEventListener('click', function (event) {
    var control = event.target.closest ? event.target.closest('a[data-columns]') : null;
    if (control) { keep(control.getAttribute('data-columns')); }
  }, true);
  document.addEventListener('submit', function (event) {
    var form = event.target, list = [], changed = false;
    if (!form.matches || !form.matches('form.columns')) { return; }
    var boxes = form.querySelectorAll('input[name="columns"]');
    for (var i = 0; i < boxes.length; i++) {
      if (boxes[i].checked) { list.push(boxes[i].value); }
      if (boxes[i].checked !== boxes[i].defaultChecked) { changed = true; }
    }
    if (changed) { keep(list.join(',')); } // boxes left as the page drew them make no choice
  }, true);

  // Job 2. A swap of #page, by the live poll or by a control, keeps each fold
  // the viewer opened or closed: every <details> with an id keeps its state.
  var folds = null;
  document.addEventListener('htmx:beforeSwap', function (event) {
    if (!event.detail.target || event.detail.target.id !== 'page') { return; }
    folds = {};
    var before = document.querySelectorAll('#page details[id]');
    for (var i = 0; i < before.length; i++) { folds[before[i].id] = before[i].open; }
  });
  document.addEventListener('htmx:afterSwap', function () {
    if (!folds) { return; }
    var after = document.querySelectorAll('#page details[id]');
    for (var i = 0; i < after.length; i++) {
      var was = folds[after[i].id];
      if (was !== undefined && was !== after[i].open) { after[i].open = was; }
    }
    folds = null;
  });
})();
