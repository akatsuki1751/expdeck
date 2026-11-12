(function () {
  'use strict';
  var keys = { left:'exp.left.width', right:'exp.right.width', rightClosed:'exp.right.collapsed', theme:'exp.theme' };
  function get(key, fallback) { try { return localStorage.getItem(key) || fallback; } catch (_) { return fallback; } }
  function put(key, value) { try { localStorage.setItem(key, value); } catch (_) {} }
  function apply() {
    var app = document.getElementById('app'); if (!app) return;
    var left = Math.max(160, Math.min(440, Number(get(keys.left, 234)) || 234));
    var right = Math.max(230, Math.min(520, Number(get(keys.right, 306)) || 306));
    var available = Math.max(500, window.innerWidth - 310);
    if (left + right > available) { var factor = available / (left + right); left = Math.max(160,Math.floor(left * factor)); right = Math.max(230,Math.floor(right * factor)); }
    document.documentElement.style.setProperty('--left-width', left + 'px');
    document.documentElement.style.setProperty('--right-width', right + 'px');
    var closed = get(keys.rightClosed, '0') === '1'; app.classList.toggle('right-collapsed', closed);
    var button = app.querySelector('[data-toggle-right]');
    if (button) { button.textContent = closed ? '‹' : '›'; button.setAttribute('aria-expanded',String(!closed)); button.setAttribute('aria-label',closed ? '展开右侧栏' : '收起右侧栏'); }
    var theme = get(keys.theme, 'light') === 'dark' ? 'dark' : 'light';
    document.documentElement.dataset.theme = theme;
    var radio = app.querySelector('input[name="theme"][value="' + theme + '"]'); if (radio) radio.checked = true;
  }
  document.addEventListener('DOMContentLoaded', apply);
  document.addEventListener('htmx:afterSwap', apply);
  window.addEventListener('resize', apply);
  document.addEventListener('change', function (event) {
    if (event.target.matches('input[name="theme"]')) { put(keys.theme, event.target.value); apply(); }
  });
  document.addEventListener('click', function (event) {
    var target = event.target;
    if (target.closest('[data-toggle-right]')) { var app=document.getElementById('app'); put(keys.rightClosed, app.classList.contains('right-collapsed') ? '0' : '1'); apply(); }
    if (target.closest('[data-open-settings]')) { var settings=document.getElementById('settings-dialog'); if (settings) settings.showModal(); }
    var close=target.closest('[data-close-dialog]'); if (close) { var dialog=close.closest('dialog'); if(dialog) dialog.close(); }
    var rename=target.closest('[data-rename-url]'); if(rename) {
      var dialog=document.getElementById('rename-dialog'), form=document.getElementById('rename-form'), input=document.getElementById('rename-path');
      form.action=rename.dataset.renameUrl; input.value=rename.dataset.renamePath; var menu=rename.closest('details'); if(menu)menu.open=false; dialog.showModal(); input.focus(); input.select();
    }
    document.querySelectorAll('details.menu[open]').forEach(function(menu){ if(!menu.contains(target)) menu.open=false; });
  });
  document.addEventListener('click', function(event){ if(event.target.matches('dialog.dialog')) event.target.close(); });
  document.addEventListener('pointerdown', function(event) {
    var handle=event.target.closest('[data-resize]'); if(!handle || window.innerWidth<=850) return;
    var app=document.getElementById('app'), side=handle.dataset.resize;
    if ((side==='left' && app.classList.contains('rail-collapsed')) || (side==='right' && app.classList.contains('right-collapsed'))) return;
    event.preventDefault(); handle.setPointerCapture(event.pointerId); handle.classList.add('dragging');
    function move(e) { var width=side==='left' ? e.clientX : window.innerWidth-e.clientX; put(side==='left'?keys.left:keys.right, Math.round(width)); apply(); }
    function end() { handle.classList.remove('dragging'); handle.removeEventListener('pointermove',move); handle.removeEventListener('pointerup',end); handle.removeEventListener('pointercancel',end); }
    handle.addEventListener('pointermove',move); handle.addEventListener('pointerup',end); handle.addEventListener('pointercancel',end);
  });
  document.addEventListener('keydown', function(event) {
    var handle=event.target.closest('[data-resize]'); if(!handle || !['ArrowLeft','ArrowRight'].includes(event.key)) return;
    event.preventDefault(); var side=handle.dataset.resize, key=side==='left'?keys.left:keys.right;
    var delta=event.key==='ArrowRight'?10:-10; if(side==='right')delta=-delta;
    put(key,(Number(get(key,side==='left'?234:306))||0)+delta); apply();
  });
})();
