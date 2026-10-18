// photon tune's timeline drag: the pointer machinery behind every edge or bar the timeline lets
// a person drag. It builds on app.js's window.tune: pause() when a drag starts and edit(edit)
// when it is let go, which the server applies, validates and schedules. It loads before
// timeline.js, which reads window.tuneDrag.
//
// window.tuneDrag is {SNAP_MS, attach(drag), defersRedraw()}. attach makes drag.node draggable,
// where drag is
//   {node, track, value, min, max, axis, valueAt(ms), extent(value), select(), edit(value),
//    redraw()}:
// the node the pointer grabs and the lane track it moves on, the value it holds now and the
// bounds a drag keeps it in, the axis that maps the track to the output clock, {msAt(clientX,
// rect), percentOf(ms)}, the value the pointer at output time ms asks for, where a value is drawn
// — {edge, from, to, label}: the node's left edge, and the stretch of the output clock the ghost
// spans, labelled — what the drag selects when it starts, the window.tune edit it sends, and the
// redraw from state it runs on a refusal, a cancel, an unchanged value or a redraw it deferred.
//
// A drag captures the pointer, pauses the preview, moves its value in steps of SNAP_MS within its
// bounds, shows the new value on the node and a .drag-ghost locally, and sends one edit when it is
// let go at a changed value. While it runs, defersRedraw() says so to a renderer about to redraw
// from state and holds that redraw until the drag ends, so the dragged node is not replaced under
// the pointer.
"use strict";

(() => {
  const tune = window.tune;
  const SNAP_MS = 50;
  const PERCENT_UNIT = "%";

  let active = null;
  let isRedrawPending = false;

  // attach makes drag.node start, follow and end a drag of drag on its pointer events.
  function attach(drag) {
    drag.node.addEventListener("pointerdown", (event) => startDrag(event, drag));
    drag.node.addEventListener("pointermove", moveDrag);
    drag.node.addEventListener("pointerup", endDrag);
    drag.node.addEventListener("pointercancel", cancelDrag);
  }

  // defersRedraw says a drag is running, and then holds the redraw its caller skips until the
  // drag ends.
  function defersRedraw() {
    if (active) {
      isRedrawPending = true;
    }
    return Boolean(active);
  }

  function startDrag(event, drag) {
    event.preventDefault();
    event.stopPropagation();
    tune.pause();
    drag.select();
    const ghost = document.createElement("div");
    ghost.className = "drag-ghost";
    drag.track.append(ghost);
    drag.node.setPointerCapture(event.pointerId);
    drag.node.classList.add("dragging");
    isRedrawPending = false;
    active = { drag, ghost, rect: drag.track.getBoundingClientRect(), value: drag.value };
    showDrag();
  }

  // moveDrag snaps the dragged value to SNAP_MS within the drag's bounds and shows it.
  function moveDrag(event) {
    if (!active) {
      return;
    }
    const drag = active.drag;
    const raw = drag.valueAt(drag.axis.msAt(event.clientX, active.rect));
    const snapped = Math.round(raw / SNAP_MS) * SNAP_MS;
    active.value = Math.min(Math.max(snapped, drag.min), drag.max);
    showDrag();
  }

  // showDrag moves the node to the dragged value and spans the ghost over the stretch it draws,
  // labelled with the value the edit will send.
  function showDrag() {
    const { drag, ghost, value } = active;
    const extent = drag.extent(value);
    drag.node.style.left = drag.axis.percentOf(extent.edge) + PERCENT_UNIT;
    ghost.style.left = drag.axis.percentOf(extent.from) + PERCENT_UNIT;
    ghost.style.width = drag.axis.percentOf(extent.to - extent.from) + PERCENT_UNIT;
    ghost.textContent = extent.label;
  }

  // endDrag sends the dragged value as one edit when it changed. A refused edit stands on its
  // owner in the inspector, which app.js shows, so the drag only redraws.
  function endDrag() {
    if (!active) {
      return;
    }
    const { drag, value } = active;
    finishDrag();
    if (value === drag.value) {
      drag.redraw();
      return;
    }
    tune.edit(drag.edit(value)).catch(() => drag.redraw());
  }

  function cancelDrag() {
    if (active) {
      const drag = active.drag;
      finishDrag();
      drag.redraw();
    }
  }

  function finishDrag() {
    const drag = active.drag;
    active.ghost.remove();
    drag.node.classList.remove("dragging");
    active = null;
    if (isRedrawPending) {
      isRedrawPending = false;
      drag.redraw();
    }
  }

  window.tuneDrag = Object.freeze({ SNAP_MS, attach, defersRedraw });
})();
