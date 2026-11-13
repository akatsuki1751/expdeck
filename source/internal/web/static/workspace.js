(function () {
  'use strict';
  var keys = { left:'exp.left.width', right:'exp.right.width', rightClosed:'exp.right.collapsed', theme:'exp.theme' };
  function get(key, fallback) { try { return localStorage.getItem(key) || fallback; } catch (_) { return fallback; } }
  function put(key, value) { try { localStorage.setItem(key, value); } catch (_) {} }
  function apply() {
    var app = document.getElementById('app'); if (!app) return;
    var left = Math.max(160, Math.min(440, Number(get(keys.left, 234)) || 234));
    var right = Math.max(230, Math.min(520, Number(get(keys.right, 306)) || 306));
    // Protect the reading area while keeping both panes resizable.
    if (window.innerWidth > 700 && window.innerWidth < 1200) {
      var available = Math.max(390, window.innerWidth - 440);
      if (left + right > available) {
        var factor = available / (left + right);
        left = Math.max(160, Math.floor(left * factor));
        right = Math.max(210, Math.floor(right * factor));
      }
    }
    document.documentElement.style.setProperty('--left-width', left + 'px');
    document.documentElement.style.setProperty('--right-width', right + 'px');
    var closed = get(keys.rightClosed, '1') === '1'; app.classList.toggle('right-collapsed', closed);
    var button = app.querySelector('[data-toggle-right]');
    if (button) { button.setAttribute('aria-expanded',String(!closed)); button.setAttribute('aria-label',closed ? '展开右侧栏' : '收起右侧栏'); button.title = closed ? '展开右侧栏' : '收起右侧栏'; }
    var theme = get(keys.theme, 'light') === 'dark' ? 'dark' : 'light';
    document.documentElement.dataset.theme = theme;
    var radio = app.querySelector('input[name="theme"][value="' + theme + '"]'); if (radio) radio.checked = true;
  }
  function restoreTree() {
    var states; try { states=JSON.parse(get('exp.tree.folders','{}')); } catch (_) { states={}; }
    document.querySelectorAll('.tree-branch[data-tree-path]').forEach(function (folder) {
      var path=folder.dataset.treePath; if (Object.prototype.hasOwnProperty.call(states,path)) folder.open=!!states[path];
    });
  }
  document.addEventListener('click', function (event) {
    if (event.target.closest('.folder-create')) return;
    var summary=event.target.closest('.tree-branch > summary'); if (!summary) return;
    var folder=summary.parentElement;
    var states; try { states=JSON.parse(get('exp.tree.folders','{}')); } catch (_) { states={}; }
    states[folder.dataset.treePath]=!folder.open; put('exp.tree.folders',JSON.stringify(states));
  });
  function restoreRuns() {
    var states; try { states=JSON.parse(get('exp.tree.runs','{}')); } catch (_) { states={}; }
    document.querySelectorAll('[data-run-toggle]').forEach(function(button) {
      var list=document.getElementById(button.getAttribute('aria-controls')); if(!list) return;
      var id=button.dataset.runToggle;
      var expanded=Object.prototype.hasOwnProperty.call(states,id) ? !!states[id] : !!list.querySelector('.run-link.is-active');
      list.hidden=!expanded; button.setAttribute('aria-expanded',String(expanded));
      button.setAttribute('aria-label',(expanded ? '收起 ' : '展开 ') + (button.dataset.runName || '') + ' 的运行记录');
    });
  }
  function updateSearch() {
    var input=document.getElementById('rail-search-input');
    var tree=document.querySelector('.rail-tree'), results=document.getElementById('rail-search-results');
    if(!input || !tree || !results) return;
    var searching=!!input.value.trim();
    tree.hidden=searching; results.hidden=!searching;
    if(!searching) results.replaceChildren();
  }
  document.addEventListener('input', function(event) { if(event.target.id==='rail-search-input') updateSearch(); });
  document.addEventListener('search', function(event) { if(event.target.id==='rail-search-input') updateSearch(); }, true);
  document.addEventListener('DOMContentLoaded', function () { apply(); restoreTree(); restoreRuns(); });
  document.addEventListener('htmx:load', function () { apply(); restoreTree(); restoreRuns(); });
  window.addEventListener('resize', apply);
  document.addEventListener('change', function (event) {
    if (event.target.matches('input[name="theme"]')) { put(keys.theme, event.target.value); apply(); }
  });
  document.addEventListener('click', function (event) {
    var target = event.target;
    var searchToggle=target.closest('[data-search-toggle]');
    if(searchToggle) { var search=document.querySelector('.rail-search'); search.hidden=!search.hidden; searchToggle.setAttribute('aria-expanded',String(!search.hidden)); if(!search.hidden) search.querySelector('input').focus(); else { search.querySelector('input').value=''; updateSearch(); } }
    if(target.closest('[data-search-clear]')) { var input=document.getElementById('rail-search-input'); input.value=''; updateSearch(); input.focus(); }
    var folderCreate=target.closest('.folder-create'); if(folderCreate) { event.preventDefault(); event.stopPropagation(); }

    var runToggle=target.closest('[data-run-toggle]'); if (runToggle) {
      var states; try { states=JSON.parse(get('exp.tree.runs','{}')); } catch (_) { states={}; }
      states[runToggle.dataset.runToggle]=runToggle.getAttribute('aria-expanded') !== 'true';
      put('exp.tree.runs',JSON.stringify(states)); restoreRuns();
    }
    if (target.closest('[data-toggle-right]')) { var app=document.getElementById('app'); put(keys.rightClosed, app.classList.contains('right-collapsed') ? '0' : '1'); apply(); }
    var open=target.closest('[data-open-dialog]'); if (open) { var toOpen=document.getElementById(open.dataset.openDialog); var current=open.closest('details.menu'); if(current)current.open=false; if(toOpen) { if(open.dataset.openDialog==='create-dialog') { var path=toOpen.querySelector('input[name="path"]'); path.value=open.dataset.createPrefix ? open.dataset.createPrefix+'/' : ''; } toOpen.showModal(); if(open.dataset.openDialog==='create-dialog') { var field=toOpen.querySelector('input[name="path"]'); field.focus(); field.setSelectionRange(field.value.length,field.value.length); } } }
    var conclude=target.closest('[data-conclude-url]'); if(conclude) { var form=document.getElementById('conclude-form'), dialog=document.getElementById('conclude-dialog'); form.action=conclude.dataset.concludeUrl; var menu=conclude.closest('details'); if(menu)menu.open=false; dialog.showModal(); }
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
