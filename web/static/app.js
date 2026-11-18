// Polling pauses while someone is typing, so a refresh never wipes a half-written note.
function noFocus() {
  var a = document.activeElement;
  return !a || !/^(INPUT|TEXTAREA|SELECT)$/.test(a.tagName);
}

// Kitchen and counter alert sound. Browsers only allow sound after a tap, hence the button.
var audioCtx = null, lastAlert = {};

function enableSound(btn) {
  audioCtx = new (window.AudioContext || window.webkitAudioContext)();
  beep();
  if (btn) btn.textContent = '🔔 Sound is on';
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

// Polling without flicker. The server sends a hash of each refreshed fragment (X-Hash); we send it
// back (X-Seen) and the server answers 204 No Content when nothing changed, so htmx leaves the page alone.
var seenHash = {};
document.addEventListener('htmx:configRequest', function (e) {
  var id = e.detail.elt.id;
  if (e.detail.verb === 'get' && id && seenHash[id]) e.detail.headers['X-Seen'] = seenHash[id];
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

// On phones the staff tabs scroll sideways: bring the current one into view.
document.addEventListener('DOMContentLoaded', function () {
  var on = document.querySelector('.stop nav a.on');
  if (on) on.parentNode.scrollLeft = on.offsetLeft - 12;
});
