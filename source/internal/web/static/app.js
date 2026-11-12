(function () {
  'use strict';
  var timer;
  function restoreRail() {
    var app = document.getElementById('app');
    var button = document.querySelector('.rail-toggle');
    if (!app || !button) return;
    var collapsed = false;
    try { collapsed = localStorage.getItem('exp.rail.collapsed') === '1'; } catch (_) {}
    app.classList.toggle('rail-collapsed', collapsed);
    button.textContent = collapsed ? '›' : '‹';
    button.setAttribute('aria-expanded', String(!collapsed));
    button.setAttribute('aria-label', collapsed ? '展开侧栏' : '收起侧栏');
  }
  document.addEventListener('click', function (event) {
    var button = event.target.closest('.rail-toggle');
    if (!button) return;
    var app = document.getElementById('app');
    var collapsed = !app.classList.contains('rail-collapsed');
    try { localStorage.setItem('exp.rail.collapsed', collapsed ? '1' : '0'); } catch (_) {}
    app.classList.toggle('rail-collapsed', collapsed);
    button.textContent = collapsed ? '›' : '‹';
    button.setAttribute('aria-expanded', String(!collapsed));
    button.setAttribute('aria-label', collapsed ? '展开侧栏' : '收起侧栏');
  });
  function startLog() {
    if (timer) clearInterval(timer);
    var pre = document.getElementById('log');
    if (!pre) return;
    var run = pre.getAttribute('data-run');
    var offset = Number(pre.getAttribute('data-offset'));
    function poll() {
      if (!pre.isConnected) { clearInterval(timer); return; }
      fetch('/log/' + encodeURIComponent(run) + '?offset=' + offset, {cache:'no-store'})
        .then(function (r) { return r.ok ? r.text() : ''; })
        .then(function (text) { if (!text || !pre.isConnected) return; offset += new TextEncoder().encode(text).length; pre.textContent += text; pre.scrollTop = pre.scrollHeight; })
        .catch(function () {});
    }
    timer = setInterval(poll, 1500);
  }
  document.addEventListener('DOMContentLoaded', function () { restoreRail(); startLog(); });
  document.addEventListener('htmx:afterSwap', function () { restoreRail(); startLog(); });
  document.addEventListener('htmx:beforeSwap', function (event) {
    if (event.detail.xhr.status === 409) { event.detail.shouldSwap = true; event.detail.isError = false; }
  });
})();
