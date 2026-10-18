// photon tune's via path overlay: while the inspector shows a pan along a via path, a polyline
// over the preview draws the path that pan runs on the frame the preview shows.
// It builds on app.js's window.tune alone: frame for the preview <img>, frameTime() for the
// output time of the frame it shows, selectedEntry() and state() for the pan the inspector
// shows, and onState(fn) to redraw with every new state. Nothing here maps cells to pixels: the
// path's points come from /api/path in output-frame pixels, and the overlay's viewBox, the
// <img>'s natural size, maps them onto the displayed <img>. window.tune tells of no selection
// change, so the overlay also redraws after every click and change on the page.
"use strict";

(() => {
  const tune = window.tune;
  const PATH_URL = "/api/path";
  const ENTRY_CAMERA = "camera";
  const KIND_PAN = "pan";
  // The class that shows the overlay; an <svg> has no hidden property to set.
  const DRAWN = "drawn";
  const KEY_SEPARATOR = "|";

  const frameImage = tune.frame;
  const view = {
    overlay: document.getElementById("via-path"),
    line: document.getElementById("via-line"),
  };

  // answered is the last path the server answered, by its key, its points null when it refused
  // it; pendingKey names the path asked for and not answered yet, or is null.
  let answered = { key: null, points: null };
  let pendingKey = null;
  let isQueued = false;

  // queueRender renders once in the next animation frame, after app.js has taken in whatever
  // the event that queued it changed.
  function queueRender() {
    if (!isQueued) {
      isQueued = true;
      requestAnimationFrame(() => {
        isQueued = false;
        render();
      });
    }
  }

  // render shows the path of the via pan the inspector shows on the frame the preview shows,
  // asking for it when the line holds another; it hides the overlay while no via pan is shown,
  // no frame is, or the path is not answered yet.
  function render() {
    const wanted = wantedPath();
    const isAnswered = wanted !== null && wanted.key === answered.key;
    if (wanted !== null && !isAnswered && wanted.key !== pendingKey) {
      requestPath(wanted);
    }
    if (isAnswered && answered.points) {
      place();
    } else {
      view.overlay.classList.remove(DRAWN);
    }
  }

  // wantedPath is the path to draw: the selected via pan's index on the frame shown at ms, keyed
  // by the state's version too, so an edit asks again; null while there is none to draw.
  function wantedPath() {
    const entry = tune.selectedEntry();
    const state = tune.state();
    const ms = tune.frameTime();
    if (!entry || !state || ms === null || entry.kind !== ENTRY_CAMERA) {
      return null;
    }
    const move = state.camera.find((each) => each.index === entry.index);
    if (!move || move.kind !== KIND_PAN || !move.via) {
      return null;
    }
    return { index: move.index, ms, key: [move.index, ms, state.version].join(KEY_SEPARATOR) };
  }

  // requestPath asks /api/path for wanted and renders its answer, unless another was asked for
  // since. A path the server refuses draws nothing: the inspector shows what stands on the pan.
  async function requestPath(wanted) {
    pendingKey = wanted.key;
    let points = null;
    try {
      const response = await fetch(PATH_URL + "?camera=" + wanted.index + "&t=" + wanted.ms);
      const answer = await response.json();
      points = response.ok ? answer.points : null;
    } catch (error) {
      points = null;
    }
    if (pendingKey !== wanted.key) {
      return;
    }
    pendingKey = null;
    answered = { key: wanted.key, points };
    if (points) {
      view.line.setAttribute("points", points.map((point) => point.x + "," + point.y).join(" "));
    }
    render();
  }

  // place lays the overlay over the displayed <img>, its viewBox the <img>'s natural size, and
  // shows it; it stays hidden while the <img> shows nothing.
  function place() {
    const bounds = frameImage.getBoundingClientRect();
    if (bounds.width <= 0 || frameImage.naturalWidth <= 0) {
      view.overlay.classList.remove(DRAWN);
      return;
    }
    const viewBox = [0, 0, frameImage.naturalWidth, frameImage.naturalHeight].join(" ");
    view.overlay.setAttribute("viewBox", viewBox);
    view.overlay.style.left = frameImage.offsetLeft + "px";
    view.overlay.style.top = frameImage.offsetTop + "px";
    view.overlay.style.width = bounds.width + "px";
    view.overlay.style.height = bounds.height + "px";
    view.overlay.classList.add(DRAWN);
  }

  function wire() {
    frameImage.addEventListener("load", queueRender);
    window.addEventListener("resize", queueRender);
    document.body.addEventListener("click", queueRender);
    document.body.addEventListener("change", queueRender);
    tune.onState(queueRender);
  }

  wire();
})();
