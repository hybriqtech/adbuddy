(function () {
  'use strict';
  var host = window.adbuddy;
  var term = new Terminal({
    fontSize: 12,
    scrollback: 2000,
    cursorBlink: true,
    allowProposedApi: false,
    // OSC 8 links: only activated by a tap, and the app shows the domain first.
    linkHandler: { allowNonHttpProtocols: false, activate: function (e, uri) { host.link(uri); } }
  });
  // No clipboard addon is loaded: OSC 52 (a program writing to the clipboard) does nothing.
  var fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.loadAddon(new WebLinksAddon.WebLinksAddon(function (e, uri) { host.link(uri); }));
  term.open(document.getElementById('t'));

  function bytes(b64) {
    var s = atob(b64), a = new Uint8Array(s.length);
    for (var i = 0; i < s.length; i++) a[i] = s.charCodeAt(i);
    return a;
  }
  // The host rules the size (protocol spec §7): the view only proposes one.
  function propose() {
    var d = fit.proposeDimensions();
    if (d && d.cols > 0 && d.rows > 0) host.size(d.cols, d.rows);
  }

  // Replayed output may hold queries (DSR, DA) that xterm.js answers through onData: those answers are not the person's keys.
  var quiet = 0;
  function put(b64, isReplay) {
    if (isReplay) {
      quiet++;
      // If write throws, the callback never runs: without the catch the person's keys would stay muted for good.
      try { term.write(bytes(b64), function () { quiet--; }); } catch (e) { quiet--; }
    } else {
      term.write(bytes(b64));
    }
  }
  window.termReset = function (cols, rows, b64, isReplay) { term.reset(); term.resize(cols, rows); put(b64, isReplay); };
  window.termWrite = function (b64, isReplay) { put(b64, isReplay); };
  window.termResize = function (cols, rows) { term.resize(cols, rows); };
  window.termTheme = function (bg, fg, cursor) {
    term.options.theme = { background: bg, foreground: fg, cursor: cursor };
    document.body.style.background = bg;
  };
  window.termFocus = function () { term.focus(); };

  term.onData(function (d) { if (quiet === 0) host.data(d); });
  new ResizeObserver(propose).observe(document.getElementById('t'));
  propose();
})();
