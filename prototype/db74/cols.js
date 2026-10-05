// tableaudCols returns the search string to load when a page opens, or null.
// A URL that carries a column choice (hide or show) wins and stays as it is.
// A URL without one gets the stored choice appended.
function tableaudCols(search, stored) {
  if (!stored) return null;
  var q = search.replace(/^\?/, '');
  if (/(^|&)(hide|show)=/.test(q)) return null;
  return '?' + (q ? q + '&' : '') + stored;
}
(function () {
  var K = 'tableaud.cols', s = null;
  try { s = localStorage.getItem(K); } catch (e) {}
  var n = tableaudCols(location.search, s);
  if (n) { location.replace(location.pathname + n); return; }
  document.addEventListener('click', function (e) {
    var a = e.target.closest && e.target.closest('a[data-c]');
    if (!a) return;
    try {
      if (a.dataset.c && a.dataset.c !== 'hide=') localStorage.setItem(K, a.dataset.c);
      else localStorage.removeItem(K);
    } catch (x) {}
  });
})();
