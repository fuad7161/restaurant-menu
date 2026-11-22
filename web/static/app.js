// Polling pauses while someone is typing or answering a question, so a refresh never wipes a
// half-written note or the button a dialog is about.
function noFocus() {
  if (document.querySelector('dialog[open]')) return false;
  var a = document.activeElement;
  return !a || !/^(INPUT|TEXTAREA|SELECT)$/.test(a.tagName);
}

// ---------- sound ----------
// Kitchen and counter alert sound. Browsers only allow sound after a tap, hence the button.
var audioCtx = null, lastAlert = {};

function enableSound(btn) {
  audioCtx = audioCtx || new (window.AudioContext || window.webkitAudioContext)();
  beep();
  if (btn && btn.dataset.on) btn.textContent = btn.dataset.on;
}

function beep() {
  if (!audioCtx) return;
  [0, 0.25, 0.5].forEach(function (t) {
    var o = audioCtx.createOscillator(), g = audioCtx.createGain();
    o.frequency.value = 880;
    g.gain.value = 0.25;
    o.connect(g); g.connect(audioCtx.destination);
    o.start(audioCtx.currentTime + t);
    o.stop(audioCtx.currentTime + t + 0.15);
  });
}

// Beep at once when something new needs attention, and again every 15 seconds until it is handled.
function checkAlerts(repeat) {
  document.querySelectorAll('[data-alert]').forEach(function (el) {
    var n = +el.dataset.alert, prev = lastAlert[el.id];
    if ((prev !== undefined && n > prev) || (repeat && n > 0)) beep();
    lastAlert[el.id] = n;
  });
}
document.addEventListener('DOMContentLoaded', function () { checkAlerts(false); });
document.addEventListener('htmx:afterSettle', function () { checkAlerts(false); });
setInterval(function () { checkAlerts(true); }, 15000);

// Guests never press a sound button, so their first tap prepares sound for the "food is ready" alert.
document.addEventListener('pointerdown', function () {
  if (!audioCtx && document.body.classList.contains('guest')) {
    try { audioCtx = new (window.AudioContext || window.webkitAudioContext)(); } catch (e) { /* no sound */ }
  }
}, { once: true });

// ---------- polling without flicker ----------
// The server sends a hash of each refreshed fragment (X-Hash); we send it back (X-Seen) and the
// server answers 204 No Content when nothing changed, so htmx leaves the page alone.
var seenHash = {};
document.addEventListener('htmx:configRequest', function (e) {
  var el = e.detail.elt, id = el.id;
  if (e.detail.verb === 'get' && id && seenHash[id]) e.detail.headers['X-Seen'] = seenHash[id];
  // The text typed into a reason dialog (data-ask) goes with the request as "reason".
  if (el._askValue !== undefined) {
    e.detail.parameters.reason = el._askValue;
    delete el._askValue;
  }
});
document.addEventListener('htmx:afterRequest', function (e) {
  var id = e.detail.elt.id, xhr = e.detail.xhr;
  if (!id || !xhr) return;
  var h = xhr.getResponseHeader('X-Hash');
  if (h) seenHash[id] = h; else delete seenHash[id]; // a button replaced it: compare afresh next time
});

// Slide in only the cards that were not on screen before (new order, new call), never the ones
// that were simply redrawn.
var seenKeys = {};
function markFresh(animate) {
  document.querySelectorAll('[data-key]').forEach(function (el) {
    var k = el.dataset.key;
    if (!seenKeys[k] && animate) el.classList.add('fresh');
    seenKeys[k] = true;
  });
}
document.addEventListener('DOMContentLoaded', function () { markFresh(false); });
document.addEventListener('htmx:afterSettle', function () { markFresh(true); });

// ---------- busy buttons ----------
// While a tap is being sent, its button is disabled and shows a spinner, so a double tap on a
// slow connection cannot send twice.
function busy(btn, on) {
  if (!btn) return;
  btn.disabled = on;
  if (on) btn.setAttribute('aria-busy', 'true'); else btn.removeAttribute('aria-busy');
}
function actionButtons(el) {
  if (el.matches('button')) return [el];
  if (el.matches('form')) return Array.prototype.slice.call(el.querySelectorAll('button:not([type=button])'));
  return []; // a refreshing fragment, not a tap
}
document.addEventListener('htmx:beforeRequest', function (e) {
  actionButtons(e.detail.elt).forEach(function (b) { busy(b, true); });
});
document.addEventListener('htmx:afterRequest', function (e) {
  actionButtons(e.detail.elt).forEach(function (b) { busy(b, false); });
});
// Back/forward cache: a page shown again must not keep a spinning button.
window.addEventListener('pageshow', function () {
  document.querySelectorAll('[aria-busy]').forEach(function (b) { busy(b, false); });
});

// ---------- toasts ----------
// Short messages at the bottom of the screen. The server adds them out of band (template
// "toast"); app.js adds its own for errors. Each one leaves after a few seconds (10 with Undo).
function toast(msg, bad) {
  var box = document.getElementById('toasts');
  if (!box) return;
  var t = document.createElement('div');
  t.className = 'toast' + (bad ? ' bad' : '');
  t.setAttribute('role', bad ? 'alert' : 'status');
  t.textContent = msg;
  box.appendChild(t);
}
function timeToasts() {
  document.querySelectorAll('#toasts .toast:not([data-t])').forEach(function (t) {
    t.dataset.t = '1';
    setTimeout(function () { t.remove(); }, t.querySelector('button') ? 10000 : 4000);
  });
  var box = document.getElementById('toasts');
  if (box) while (box.children.length > 3) box.firstElementChild.remove();
}
document.addEventListener('DOMContentLoaded', function () {
  var box = document.getElementById('toasts');
  if (box && window.MutationObserver) new MutationObserver(timeToasts).observe(box, { childList: true });
});
// The Undo button's toast goes away once its request is done.
document.addEventListener('htmx:afterRequest', function (e) {
  var t = e.detail.elt.closest && e.detail.elt.closest('.toast');
  if (t) t.remove();
});

// ---------- connection banner ----------
// "No internet. Reconnecting…" while the phone or tablet is offline or requests fail. Refreshing
// screens keep trying by themselves; a tap that could not be sent is reported as a toast.
function netbar(show) {
  var bar = document.getElementById('netbar');
  if (!bar) return;
  if (show) bar.textContent = bar.dataset.offline;
  bar.hidden = !show;
}
function isTap(el) { return actionButtons(el).length > 0 || el.closest('.toast'); }
window.addEventListener('offline', function () { netbar(true); });
window.addEventListener('online', function () { netbar(false); });
document.addEventListener('DOMContentLoaded', function () { if (navigator.onLine === false) netbar(true); });
document.addEventListener('htmx:sendError', function (e) {
  netbar(true);
  var bar = document.getElementById('netbar');
  if (bar && isTap(e.detail.elt)) toast(bar.dataset.failed, true);
});
document.addEventListener('htmx:responseError', function (e) {
  var bar = document.getElementById('netbar'), st = e.detail.xhr.status;
  if (!bar || !isTap(e.detail.elt)) return;
  if (st === 429) toast(bar.dataset.busy, true);
  else if (st >= 500 || st === 403) toast(bar.dataset.error, true);
});

// A photo that cannot be loaded (photo storage down, slow network) shows a drawing instead of a
// broken image.
document.addEventListener('error', function (e) {
  var img = e.target;
  if (img.tagName !== 'IMG' || img.dataset.fallback || img.src.indexOf('/uploads/') < 0) return;
  img.dataset.fallback = '1';
  img.removeAttribute('srcset');
  img.classList.add('art');
  img.src = '/static/img/thali.svg';
}, true);
document.addEventListener('htmx:afterRequest', function (e) {
  if (e.detail.successful) netbar(false);
});

// ---------- questions: confirm and reason dialogs ----------
// data-confirm="Question?" asks before a form is sent or an htmx button fires; data-ask="Question"
// also asks for a reason, sent as the "reason" field. Optional: data-confirm-yes (label of the
// yes button) and data-confirm-show (a <template> whose content is shown under the question).
// The dialog is in the page's language, unlike the browser's own confirm() and prompt().
function ask(o) {
  var d = document.getElementById('ask');
  if (!d || !d.showModal) { // very old browser: fall back to the built-in pop-ups
    return Promise.resolve(o.prompt ? window.prompt(o.question, '') : (window.confirm(o.question) ? true : null));
  }
  var input = d.querySelector('.askin'), more = d.querySelector('.askmore');
  var yes = d.querySelector('button[value=yes]'), no = d.querySelector('button[value=no]');
  d.querySelector('.askq').textContent = o.question;
  more.innerHTML = '';
  var tpl = o.show && document.querySelector(o.show);
  if (tpl && tpl.content) more.appendChild(tpl.content.cloneNode(true));
  input.hidden = !o.prompt;
  input.value = '';
  yes.textContent = o.yes || (o.prompt ? d.dataset.ok : d.dataset.yes);
  no.textContent = o.prompt ? d.dataset.cancel : d.dataset.no;
  d.returnValue = '';
  return new Promise(function (resolve) {
    d.addEventListener('close', function () {
      if (d.returnValue !== 'yes') resolve(null);
      else resolve(o.prompt ? input.value.trim() : true);
    }, { once: true });
    d.showModal();
    (o.prompt ? input : yes).focus();
  });
}
document.addEventListener('htmx:confirm', function (e) {
  var el = e.detail.elt, q = el.getAttribute('data-confirm'), a = el.getAttribute('data-ask');
  if (!q && !a) return;
  e.preventDefault();
  ask({ question: q || a, prompt: !!a, yes: el.dataset.confirmYes, show: el.dataset.confirmShow }).then(function (v) {
    if (v === null) return;
    if (a) el._askValue = v;
    e.detail.issueRequest(true);
  });
});
// Plain forms (no htmx): ask first when data-confirm is set, then show the button as busy.
document.addEventListener('submit', function (e) {
  var f = e.target;
  if (f.method === 'dialog' || f.hasAttribute('hx-post') || e.defaultPrevented) return;
  if (f.dataset.confirm && !f._confirmed) {
    e.preventDefault();
    var submitter = e.submitter;
    ask({ question: f.dataset.confirm, yes: f.dataset.confirmYes, show: f.dataset.confirmShow }).then(function (v) {
      if (v === null) return;
      f._confirmed = true;
      if (f.requestSubmit) f.requestSubmit(submitter || undefined); else f.submit();
    });
    return;
  }
  f._confirmed = false;
  // Disable after the browser has read the form, so a named submit button is still sent.
  var b = e.submitter || f.querySelector('button:not([type=button])');
  setTimeout(function () { busy(b, true); });
});

// ---------- dish sheet ----------
// Tapping a dish loads its sheet into <dialog id="sheet">. Adding answers with an empty sheet,
// which closes it. The Add button shows the total of price, options and quantity; the server
// works the price out again on its own.
function taka(p) { return '৳' + (p % 100 === 0 ? String(p / 100) : (p / 100).toFixed(2)); }
function sheetTotal(form) {
  var extra = 0;
  form.querySelectorAll('input[data-extra]:checked').forEach(function (i) { extra += +i.dataset.extra; });
  var q = Math.max(1, Math.min(20, parseInt(form.elements.qty && form.elements.qty.value, 10) || 1));
  var t = form.querySelector('[data-total]');
  if (t) t.textContent = taka((+form.dataset.base + extra) * q);
}
// A group with "up to N" stops more ticks once N are chosen.
function limitGroups(form) {
  form.querySelectorAll('fieldset[data-max]').forEach(function (fs) {
    var max = +fs.dataset.max, boxes = fs.querySelectorAll('input[type=checkbox]');
    if (max < 2 || !boxes.length) return;
    var n = fs.querySelectorAll('input[type=checkbox]:checked').length;
    boxes.forEach(function (b) { b.disabled = !b.checked && n >= max; });
  });
}
document.addEventListener('htmx:afterSwap', function (e) {
  var s = e.detail.target;
  if (s.id !== 'sheet') return;
  var form = s.querySelector('form');
  if (form) {
    if (!s.open) s.showModal();
    sheetTotal(form);
    limitGroups(form);
  } else if (s.open) {
    s.close();
  }
});
document.addEventListener('DOMContentLoaded', function () {
  var s = document.getElementById('sheet');
  if (!s) return;
  s.addEventListener('close', function () { s.innerHTML = ''; });
  s.addEventListener('click', function (e) { if (e.target === s) s.close(); }); // tap outside
});
document.addEventListener('click', function (e) {
  var t = e.target;
  if (t.closest('[data-close-sheet]')) document.getElementById('sheet').close();
  var step = t.closest('[data-qty]');
  if (step) {
    var form = step.closest('form'), q = form.elements.qty;
    q.value = Math.max(1, Math.min(20, (parseInt(q.value, 10) || 1) + +step.dataset.qty));
    sheetTotal(form);
  }
  if (t.closest('[data-print]')) window.print();
});
// An optional single choice can be taken back by tapping it again.
var radioWas = null;
document.addEventListener('pointerdown', function (e) {
  var r = e.target.closest('.sheetform label.oline');
  var i = r && r.querySelector('input[type=radio]:not([required])');
  radioWas = i && i.checked ? i : null;
});
document.addEventListener('click', function (e) {
  var r = e.target.closest('.sheetform label.oline'), was = radioWas;
  if (!r || !was || !r.contains(was)) return;
  radioWas = null; // a tap on the label also clicks the input: act once
  setTimeout(function () { was.checked = false; sheetTotal(was.form); });
});
document.addEventListener('input', function (e) {
  var form = e.target.closest && e.target.closest('.sheetform');
  if (!form) return;
  limitGroups(form);
  sheetTotal(form);
});

// ---------- menu: search and the category bar ----------
document.addEventListener('DOMContentLoaded', function () {
  var q = document.getElementById('menusearch');
  if (q) q.addEventListener('input', function () {
    var v = q.value.trim().toLowerCase(), any = false;
    document.querySelectorAll('section.catsec').forEach(function (sec) {
      var n = 0;
      sec.querySelectorAll('.item').forEach(function (it) {
        var show = !v || it.dataset.search.indexOf(v) >= 0;
        it.hidden = !show;
        if (show) n++;
      });
      sec.hidden = n === 0;
      if (n) any = true;
    });
    document.querySelectorAll('.featured').forEach(function (f) { f.hidden = !!v; });
    var none = document.getElementById('nomatch');
    if (none) none.hidden = any;
  });

  // Mark the category whose dishes are on screen, and keep its chip in view.
  var cats = document.getElementById('cats');
  var secs = document.querySelectorAll('section.catsec');
  if (!cats || !secs.length || !('IntersectionObserver' in window)) return;
  var shown = {}, current = '';
  function mark(id) {
    if (id === current) return;
    current = id;
    cats.querySelectorAll('a').forEach(function (a) {
      var on = a.hash === '#' + id;
      a.classList.toggle('on', on);
      if (on) {
        a.setAttribute('aria-current', 'true');
        cats.scrollTo({ left: a.offsetLeft - (cats.clientWidth - a.offsetWidth) / 2, behavior: 'smooth' });
      } else {
        a.removeAttribute('aria-current');
      }
    });
  }
  var io = new IntersectionObserver(function (entries) {
    entries.forEach(function (en) { shown[en.target.id] = en.isIntersecting; });
    for (var i = 0; i < secs.length; i++) if (shown[secs[i].id]) { mark(secs[i].id); return; }
  }, { rootMargin: '-90px 0px -55% 0px' });
  secs.forEach(function (s) { io.observe(s); });
});

// ---------- guest: food is ready ----------
// When an order on the orders screen turns Ready, the phone vibrates, beeps if it can, shows a
// toast and puts "Ready" in the tab title until the guest looks.
var orderStatus = {}, plainTitle = document.title;
function checkReady(first) {
  var live = document.getElementById('live');
  if (!live) return;
  var ready = false;
  live.querySelectorAll('[data-order]').forEach(function (o) {
    var id = o.dataset.order, st = o.dataset.status, prev = orderStatus[id];
    if (!first && prev && prev !== 'ready' && prev !== 'served' && st === 'ready') ready = true;
    orderStatus[id] = st;
  });
  if (!ready) return;
  var msg = live.dataset.readyMsg;
  if (navigator.vibrate) navigator.vibrate([300, 150, 300]);
  beep();
  toast('✓ ' + msg);
  document.title = '✓ ' + msg + ' · ' + plainTitle;
}
document.addEventListener('DOMContentLoaded', function () { plainTitle = document.title; checkReady(true); });
document.addEventListener('htmx:afterSettle', function () { checkReady(false); });
document.addEventListener('visibilitychange', function () {
  if (document.visibilityState === 'visible') setTimeout(function () { document.title = plainTitle; }, 4000);
});

// ---------- staff: keep the screen on ----------
// Kitchen and counter screens ask the browser to keep the display awake. Where it cannot (older
// browsers, or a plain-HTTP address on the local Wi-Fi), a note asks staff to change the
// tablet's screen timeout instead.
document.addEventListener('DOMContentLoaded', function () {
  if (!document.body.hasAttribute('data-wake')) return;
  function note() { document.querySelectorAll('.wakenote').forEach(function (n) { n.hidden = false; }); }
  if (!('wakeLock' in navigator)) { note(); return; }
  function lock() { navigator.wakeLock.request('screen').catch(note); }
  lock();
  document.addEventListener('visibilitychange', function () { if (document.visibilityState === 'visible') lock(); });
});

// ---------- staff sidebar ----------
// On narrow screens the menu button slides it in, the dimmed page or Escape closes it.
function toggleNav(open) {
  document.body.classList.toggle('navopen', open);
  var btn = document.querySelector('.stop .menubtn');
  if (btn) btn.setAttribute('aria-expanded', open ? 'true' : 'false');
  if (open) {
    var first = document.querySelector('.side nav a.on') || document.querySelector('.side nav a');
    if (first) first.focus();
  } else if (btn && document.activeElement && document.activeElement.closest('.side')) {
    btn.focus();
  }
}
document.addEventListener('keydown', function (e) {
  if (e.key === 'Escape' && document.body.classList.contains('navopen')) toggleNav(false);
});
