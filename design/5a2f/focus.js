// focus.js: the alternative of decision 6, and no part of the design as it
// stands. It lands only if the owner takes that alternative: the lines below
// then go into internal/web/static/tableaud.js after Job 2, before the last
// line, and TestScriptIsSmall takes a bound of one hundred lines.
// No one has run it: this host has no browser and no node.

  // Job 3. The same swap keeps the focus. HTMX returns it to an element that
  // has an id. An element with none gets it back by its place: the nearest
  // ancestor with an id, and its position among the controls inside that one.
  var CONTROLS = 'a[href], summary, button, input:not([type="hidden"]), textarea, [tabindex="0"]';
  var place = null;
  document.addEventListener('htmx:beforeSwap', function (event) {
    place = null;
    var el = document.activeElement;
    if (!event.detail.target || event.detail.target.id !== 'page') { return; }
    if (!el || el.id || !el.closest || !el.closest('#page')) { return; }
    var anchor = el.parentElement.closest('[id]');
    var inside = anchor.querySelectorAll(CONTROLS);
    place = { id: anchor.id, tag: el.tagName, index: Array.prototype.indexOf.call(inside, el) };
  });
  document.addEventListener('htmx:afterSwap', function () {
    if (!place) { return; }
    var anchor = document.getElementById(place.id);
    var next = anchor && place.index >= 0 ? anchor.querySelectorAll(CONTROLS)[place.index] : null;
    if (next && next.tagName === place.tag) { next.focus({ preventScroll: true }); }
    place = null;
  });
