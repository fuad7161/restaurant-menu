// Polling pauses while someone is typing, so a refresh never wipes a half-written note.
function noFocus() {
  var a = document.activeElement;
  return !a || !/^(INPUT|TEXTAREA|SELECT)$/.test(a.tagName);
}

// Kitchen and counter alert sound. Browsers only allow sound after a tap, hence the button.
var audioCtx = null, lastAlert = {}, ticks = 0;

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

// Beep when something new needs attention, and repeat about every 15 seconds until it is handled.
function checkAlerts() {
  ticks++;
  document.querySelectorAll('[data-alert]').forEach(function (el) {
    var n = +el.dataset.alert, prev = lastAlert[el.id];
    if ((prev !== undefined && n > prev) || (n > 0 && ticks % 5 === 0)) beep();
    lastAlert[el.id] = n;
  });
}
document.addEventListener('DOMContentLoaded', checkAlerts);
document.addEventListener('htmx:afterSettle', function (e) {
  if (e.target && e.target.dataset && e.target.dataset.alert !== undefined) checkAlerts();
});
