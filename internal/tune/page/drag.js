// photon tune's timeline drag: the pointer machinery behind every edge or bar the timeline lets
// a person drag. It builds on app.js's window.tune: pause() when a drag starts and edit(edit)
// when it is let go, which the server applies, validates and schedules. It loads before
// timeline.js, which reads window.tuneDrag.
//
// window.tuneDrag is {SNAP_MS, attach(drag), defersRedraw()}. attach makes drag.node draggable,
// where drag is
//   {node, track, value, min, max, axis, valueAt(ms, grabbedMs), extent(value), select(),
//    edit(value), redraw(), refused(message), reorder}:
// the node the pointer grabs and the lane track it moves on, the value it holds now and the
// bounds a drag keeps it in, the axis that maps the track to the output clock, {msAt(clientX,
// rect), percentOf(ms)}, the value the pointer at output time ms asks for when it grabbed the
// node at output time grabbedMs, where a value is drawn — {edge, from, to, label}: the node's
// left edge, and the stretch of the output clock the ghost spans, labelled — what the drag
// selects when it starts, the window.tune edit it sends, the redraw from state it runs on a
// refusal, a cancel, an unchanged value or a redraw it deferred, and what shows a refused edit's
// message once that redraw has run. reorder, which a drag may leave out, lets the node be dragged
// up or down its track to another row as well: {owner, field, rows, row, indexAt(row)}, the list
// entry the node draws and the field that moves it within its list, how many rows the track is
// tall and the row the node is in, and the list index of the entry drawn in a row.
//
// A drag captures the pointer, pauses the preview, moves its value in steps of SNAP_MS within its
// bounds, shows the new value on the node and a .drag-ghost locally, and sends one edit when it is
// let go at a changed value. A drag with a reorder commits, once the pointer has moved
// AXIS_COMMIT_PX from where it went down, to the axis it moved further along: across, it moves
// the value as any drag does; up or down, it moves the node a row each time its leading edge
// passes the next row's midpoint, within the track, and when let go in another row sends one
// {owner, field, value} edit whose value is that row's list index, then selects the entry there.
// While it runs, defersRedraw() says so to a renderer about to redraw from state and holds that
// redraw until the drag ends, so the dragged node is not replaced under the pointer.
"use strict";

(() => {
  const tune = window.tune;
  const SNAP_MS = 50;
  const PERCENT_UNIT = "%";
  const PIXEL_UNIT = "px";
  const AXIS_COMMIT_PX = 6;
  // The directions a drag moves in: across its track, shifting its value, or, for a drag with a
  // reorder, up or down it, to another row.
  const DIRECTION_SHIFT = "shift";
  const DIRECTION_REORDER = "reorder";

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
    const rect = drag.track.getBoundingClientRect();
    const grabbedMs = drag.axis.msAt(event.clientX, rect);
    active = {
      drag,
      ghost,
      rect,
      grabbedMs,
      value: drag.value,
      startX: event.clientX,
      startY: event.clientY,
      direction: drag.reorder ? null : DIRECTION_SHIFT,
      row: drag.reorder ? drag.reorder.row : null,
    };
    showDrag();
  }

  // moveDrag snaps the dragged value to SNAP_MS within the drag's bounds and shows it, or, on a
  // drag committed to a reorder, moves the node to the row the pointer asks for.
  function moveDrag(event) {
    if (!active) {
      return;
    }
    if (!active.direction) {
      active.direction = committedDirection(event);
    }
    if (active.direction === DIRECTION_REORDER) {
      active.row = rowAt(event.clientY);
      showReorder();
      return;
    }
    if (active.direction !== DIRECTION_SHIFT) {
      return;
    }
    const drag = active.drag;
    const raw = drag.valueAt(drag.axis.msAt(event.clientX, active.rect), active.grabbedMs);
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

  // committedDirection is the direction a drag with a reorder commits to once the pointer at event
  // is AXIS_COMMIT_PX from where it went down, the axis it moved further along; null before.
  function committedDirection(event) {
    const across = event.clientX - active.startX;
    const along = event.clientY - active.startY;
    if (Math.hypot(across, along) < AXIS_COMMIT_PX) {
      return null;
    }
    return Math.abs(along) > Math.abs(across) ? DIRECTION_REORDER : DIRECTION_SHIFT;
  }

  // rowHeight is how tall a row of the dragged node's track is, in displayed pixels.
  function rowHeight() {
    return active.rect.height / active.drag.reorder.rows;
  }

  // rowAt is the row the node moves to with the pointer at clientY: a row further each time the
  // node's leading edge passes the next row's midpoint, kept on the track.
  function rowAt(clientY) {
    const reorder = active.drag.reorder;
    const height = rowHeight();
    if (!(height > 0)) {
      return reorder.row;
    }
    const row = reorder.row + Math.round((clientY - active.startY) / height);
    return Math.min(Math.max(row, 0), reorder.rows - 1);
  }

  // showReorder moves the node to the dragged row and puts the ghost over that row, labelled with
  // the entry the move makes it.
  function showReorder() {
    const { drag, ghost, row } = active;
    const reorder = drag.reorder;
    const height = rowHeight();
    drag.node.style.transform = "translateY(" + (row - reorder.row) * height + PIXEL_UNIT + ")";
    ghost.style.top = row * height + PIXEL_UNIT;
    ghost.style.height = height + PIXEL_UNIT;
    ghost.textContent = reorder.owner.kind + " " + reorder.indexAt(row);
  }

  // endDrag sends the dragged value as one edit when it changed. A refused edit stands on its
  // owner in the inspector, which app.js shows; the drag redraws, then hands the message to
  // refused, which may show it elsewhere as well.
  function endDrag() {
    if (!active) {
      return;
    }
    const { drag, value, direction, row } = active;
    finishDrag();
    if (direction === DIRECTION_REORDER) {
      endReorder(drag, row);
      return;
    }
    if (value === drag.value) {
      drag.redraw();
      return;
    }
    tune.edit(drag.edit(value)).catch((error) => refuse(drag, error));
  }

  // endReorder sends the move of drag's entry to the list index of row when that is not the row
  // it started in, and selects the entry at that index once the move is applied.
  function endReorder(drag, row) {
    const reorder = drag.reorder;
    if (row === reorder.row) {
      drag.redraw();
      return;
    }
    const index = reorder.indexAt(row);
    tune.edit({ owner: reorder.owner, field: reorder.field, value: index }).then(
      () => tune.selectEntry({ kind: reorder.owner.kind, index }),
      (error) => refuse(drag, error),
    );
  }

  // refuse redraws drag from state and hands it the refused edit's message.
  function refuse(drag, error) {
    drag.redraw();
    drag.refused(error.message);
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
