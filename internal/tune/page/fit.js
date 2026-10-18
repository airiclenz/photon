// photon tune's preview fit: in a window too short for the preview <img> at its width-fitted
// size with the transport bar and the timeline under it, the <img> shrinks, its aspect kept and
// never below MIN_HEIGHT_PX tall, so the timeline stays on screen; a window tall enough shows it
// width-fitted again.
// It builds on app.js's window.tune alone: frame for the preview <img>. It measures the page by
// getBoundingClientRect only and sets the <img>'s max height from window.innerHeight minus what
// the rest of the page takes above the <img>, below it in the preview column and under the
// preview; a taller inspector beside the preview is out of its reach. It loads before picker.js
// and path.js, so on a resize the <img> is fitted before they redraw over it; they also redraw
// on the <img>'s own resize.
"use strict";

(() => {
  const tune = window.tune;
  const MIN_HEIGHT_PX = 120;

  const frameImage = tune.frame;
  const view = {
    preview: document.getElementById("preview"),
    transport: document.getElementById("transport"),
    timeline: document.getElementById("timeline-panel"),
  };

  let isQueued = false;

  // fit caps the <img>'s height at the room the window leaves it, or lifts the cap while the
  // width-fitted <img> fits; it does nothing while the <img> shows nothing.
  function fit() {
    const fitted = widthFittedHeight();
    if (fitted === null) {
      return;
    }
    const room = roomForImage();
    if (room >= fitted) {
      frameImage.style.maxHeight = "";
      return;
    }
    frameImage.style.maxHeight = Math.max(MIN_HEIGHT_PX, Math.floor(room)) + "px";
  }

  // widthFittedHeight is the <img>'s height at the preview column's width, never upscaled past
  // its natural size, or null while it shows nothing.
  function widthFittedHeight() {
    const natural = { width: frameImage.naturalWidth, height: frameImage.naturalHeight };
    const width = Math.min(view.preview.getBoundingClientRect().width, natural.width);
    if (width <= 0 || natural.width <= 0) {
      return null;
    }
    return (width * natural.height) / natural.width;
  }

  // roomForImage is the window's height less everything stacked with the <img>: the page above
  // it, the rest of the preview column below it, the transport bar and the timeline, and the
  // gap before, between and after those two, measured from the gap between them.
  function roomForImage() {
    const page = document.body.getBoundingClientRect();
    const image = frameImage.getBoundingClientRect();
    const preview = view.preview.getBoundingClientRect();
    const transport = view.transport.getBoundingClientRect();
    const timeline = view.timeline.getBoundingClientRect();
    const gap = Math.max(timeline.top - transport.bottom, 0);
    const above = image.top - page.top;
    const below = Math.max(preview.bottom - image.bottom, 0);
    return window.innerHeight - (above + below + gap + transport.height + gap + timeline.height + gap);
  }

  // queueFit fits once in the next animation frame, after whatever resized has been laid out.
  function queueFit() {
    if (!isQueued) {
      isQueued = true;
      requestAnimationFrame(() => {
        isQueued = false;
        fit();
      });
    }
  }

  function wire() {
    frameImage.addEventListener("load", fit);
    window.addEventListener("resize", fit);
    // The transport bar and the timeline change height as the state redraws them, and the
    // preview column as a pick result shows or hides.
    const observer = new ResizeObserver(queueFit);
    for (const node of Object.values(view)) {
      observer.observe(node);
    }
  }

  wire();
})();
