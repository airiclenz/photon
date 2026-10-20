// photon tune's position marker: selecting a camera position seeks the preview to its start, and
// while it stays selected the paused preview shows the full view, the whole frame whatever the
// camera's zoom and aim, with the camera's crop drawn as a rectangle and a marker on the cell the
// position aims at. Dragging the marker moves it cell by cell over any cell of the grid, clamped
// to it, and dropping it on another cell sends one position.cell edit; the rectangle and the
// marker then follow the camera's fresh answer for the new state. Selecting anything else returns
// the preview to the camera's frame.
// It builds on app.js's window.tune: selectedEntry() for the selection, seek(ms) to its start,
// state() for the grid's cols and rows, camera(ms, position) for the crop, the grid and the aim,
// edit(edit) for the drop, frameTime() for the frame shown and frame for the preview <img>. It
// adds followSelection, which app.js calls on every selection, and wantsFullView, which frames.js
// reads, to window.tunePage, the page's internal namespace, and reads frames.js's frameView and
// settleFrames and transport.js's reportStatus from it. The overlay draws only while the frame
// shown is the full view, so the camera's drafts while the preview moves, and a settle's draft
// first, show without it. Nothing here maps a cell to pixels on its own: the camera answers the
// crop and the grid's place in full-view pixels, scaled to the displayed <img> by its
// displayed/natural size ratio; it loads after fit.js, so on a resize the <img> is fitted first.
"use strict";

(() => {
  const tune = window.tune;
  const page = window.tunePage;
  const FIELD_CELL = "position.cell";
  const KEY_SEPARATOR = ":";

  const frameImage = tune.frame;
  const view = {
    stage: document.getElementById("stage"),
    crop: document.getElementById("camera-crop"),
    marker: document.getElementById("camera-marker"),
  };

  // followed is the index of the selected position, or null while none is selected.
  let followed = null;
  // askedKey names the camera answer last asked for, version:ms:index.
  let askedKey = "";
  // camera is the last camera answer for followed, as {ms, index, answer}, or null.
  let camera = null;
  // drag is the marker's drag while its pointer is down, as {cell}.
  let drag = null;

  // selectedPosition is the index of the selected position, or null.
  function selectedPosition() {
    const entry = tune.selectedEntry();
    return entry && entry.kind === page.ENTRY_POSITION ? entry.index : null;
  }

  // wantsFullView says the settled preview shows the full view: while a position is selected.
  function wantsFullView() {
    return selectedPosition() !== null;
  }

  // followSelection seeks to a newly selected position's start, which settles the preview on its
  // full view, or settles the preview back on the camera's frame once no position is selected.
  function followSelection() {
    const index = selectedPosition();
    const wasFollowing = followed !== null;
    followed = index;
    drag = null;
    const position = index === null ? undefined : page.entryOf({ kind: page.ENTRY_POSITION, index });
    if (position) {
      tune.seek(position.start_ms);
    } else if (wasFollowing) {
      page.settleFrames();
    }
    refresh();
  }

  // --- the camera answer ---

  // refresh asks the camera for the frame shown while it shows the full view with a position
  // selected, unless that answer is already asked for, then draws.
  function refresh() {
    if (followed !== null && selectedPosition() !== followed) {
      followed = null;
      drag = null;
    }
    const ms = tune.frameTime();
    if (followed === null || ms === null || page.frameView() !== page.VIEW_FULL) {
      draw();
      return;
    }
    const index = followed;
    const key = [tune.state().version, ms, index].join(KEY_SEPARATOR);
    if (key !== askedKey) {
      askedKey = key;
      tune.camera(Math.round(ms), index).then(
        (answer) => {
          if (key === askedKey) {
            camera = { ms, index, answer };
            draw();
          }
        },
        (error) => page.reportStatus(error.message, true),
      );
    }
    draw();
  }

  // --- drawing, in the <img>'s displayed pixels ---

  // shownCamera is the camera answer for the frame shown, or null while the overlay hides: no
  // position is selected, the frame shown is not the full view, or no answer for it has come.
  function shownCamera() {
    const isFull = page.frameView() === page.VIEW_FULL;
    const isAnswered = camera && camera.index === followed && camera.ms === tune.frameTime();
    return followed !== null && isFull && isAnswered ? camera.answer : null;
  }

  // draw draws the crop rectangle and the marker on the aimed cell, or the dragged one, over the
  // full view shown, or hides them.
  function draw() {
    const answer = shownCamera();
    const ratio = naturalPerDisplayed();
    if (!answer || ratio === null) {
      view.crop.hidden = true;
      view.marker.hidden = true;
      return;
    }
    const crop = answer.crop;
    place(view.crop, crop.x / ratio, crop.y / ratio, crop.width / ratio, crop.height / ratio);
    const grid = answer.grid;
    const cell = drag ? drag.cell : answer.aim;
    const left = (grid.x + cell.x * grid.cell_width) / ratio;
    const top = (grid.y + cell.y * grid.cell_height) / ratio;
    place(view.marker, left, top, grid.cell_width / ratio, grid.cell_height / ratio);
  }

  // naturalPerDisplayed is how many full-view pixels one displayed pixel of the <img> spans, or
  // null while it shows nothing.
  function naturalPerDisplayed() {
    const displayed = frameImage.getBoundingClientRect().width;
    if (displayed <= 0 || frameImage.naturalWidth <= 0) {
      return null;
    }
    return frameImage.naturalWidth / displayed;
  }

  // place puts node over the <img> at left, top with width and height, all in displayed pixels.
  function place(node, left, top, width, height) {
    node.style.left = frameImage.offsetLeft + left + "px";
    node.style.top = frameImage.offsetTop + top + "px";
    node.style.width = width + "px";
    node.style.height = height + "px";
    node.hidden = false;
  }

  // --- the drag ---

  function onPointerDown(event) {
    const answer = shownCamera();
    if (!answer || event.button !== 0 || view.stage.classList.contains("picking")) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    view.marker.setPointerCapture(event.pointerId);
    drag = { cell: answer.aim };
    draw();
  }

  function onPointerMove(event) {
    if (drag) {
      moveTo(event);
    }
  }

  // onPointerUp drops the marker: a cell other than the one the position aims at is sent as one
  // edit, and the marker stands there until the fresh camera answer, or goes back on a refusal.
  function onPointerUp(event) {
    if (!drag) {
      return;
    }
    moveTo(event);
    const answer = shownCamera();
    const cell = drag.cell;
    drag = null;
    if (!answer || (cell.x === answer.aim.x && cell.y === answer.aim.y)) {
      draw();
      return;
    }
    const aimed = answer.aim;
    answer.aim = cell;
    draw();
    const owner = { kind: page.ENTRY_POSITION, index: followed };
    tune.edit({ owner, field: FIELD_CELL, value: { x: cell.x, y: cell.y } }).catch((error) => {
      answer.aim = aimed;
      draw();
      page.reportStatus(error.message, true);
    });
  }

  function onPointerCancel() {
    drag = null;
    draw();
  }

  // moveTo moves the dragged marker to the cell under the pointer of event, clamped to the grid.
  function moveTo(event) {
    const answer = shownCamera();
    const ratio = naturalPerDisplayed();
    if (!answer || ratio === null) {
      return;
    }
    const bounds = frameImage.getBoundingClientRect();
    const grid = answer.grid;
    const state = tune.state();
    const x = (event.clientX - bounds.left) * ratio;
    const y = (event.clientY - bounds.top) * ratio;
    drag.cell = {
      x: clamp(Math.floor((x - grid.x) / grid.cell_width), state.cols - 1),
      y: clamp(Math.floor((y - grid.y) / grid.cell_height), state.rows - 1),
    };
    draw();
  }

  // clamp is value held in [0, last].
  function clamp(value, last) {
    return Math.min(Math.max(value, 0), last);
  }

  function wire() {
    view.marker.addEventListener("pointerdown", onPointerDown);
    view.marker.addEventListener("pointermove", onPointerMove);
    view.marker.addEventListener("pointerup", onPointerUp);
    view.marker.addEventListener("pointercancel", onPointerCancel);
    // frames.js takes the new frame's time and view in its own load handler, which may run after
    // this one.
    frameImage.addEventListener("load", () => requestAnimationFrame(refresh));
    window.addEventListener("resize", draw);
    // fit.js resizes the <img> on more than a window resize.
    new ResizeObserver(draw).observe(frameImage);
    tune.onState(refresh);
  }

  Object.assign(page, { followSelection, wantsFullView });
  wire();
})();
