// photon tune's timeline: lanes on the clip's output-time axis under the transport bar.
// It builds on app.js's window.tune: state() and onState(fn) for what to draw, time() for the
// playhead, selected() and select(id) for the beat the inspector shows, selectedEntry() and
// selectEntry(entry) for the position, transition, zoom or caption it shows instead, seek(ms)
// for the axis, and edit(edit) for a lane's add, which the server applies, validates and
// schedules; a drag sends its edit through window.tuneDrag. From the page's internal window.tunePage it takes
// reportStatus, to show a refused add or bar drag in the transport bar, and beginScrub and
// endScrub, to tell the transport a scrub's pointer went down or up. A scrub starts on a press on
// the axis and ends on a pointer release or cancel anywhere in the window, so a redraw that
// replaces the axis mid-drag does not leave it running. Nothing here computes timing: every
// position comes from the state's sections, beats, positions, zooms and captions.
//
// The timeline is two lists of lanes, drawn top to bottom. Its own lanes follow the beats: such a
// lane is {name, draw(section, state), handles(section, state)}, where draw returns the nodes it
// places for one section and handles the edges a drag moves. The lanes in the registry
// window.tuneLanes, which lanes.js fills before this file runs, follow the whole state: such a
// lane is {name, draw(state), add}, where draw returns its bars as data, {from, to, className,
// label, title, entry, edges}, each placed from its from to its to and selecting its entry, {kind,
// index}, on a click, and add, {title, edit(state, ms)}, is a button beside the lane's name that
// sends edit(state, playhead) and selects the entry the edit's owner names; a lane without add has
// no button. Dragging such a bar shifts its entry's start and keeps its length: one shift edit for
// a caption, one at edit keeping its beat for a zoom or a position, one transition edit with both
// edges moved for a transition. A grip on each edge a bar's edges names, "start" or "end", resizes
// it: one transition edit with that edge moved for a transition; for a position, one hold edit for
// its end, and one span edit for its start that keeps its end. A position or a transition may
// shrink to 0, a point or a hard cut, whose grips sit beside its marker. Another lane is one more
// entry in either list.
//
// A handle is {beat, field, anchor, sign, value, min, max}: the edit's beat and field, the value
// it holds now in milliseconds, the bounds a drag keeps it in, and where its edge sits on the
// output clock — at anchor + sign × value. drag.js's window.tuneDrag, which loads before this
// file, runs the drag: it moves the edge in steps of SNAP_MS, shows the new extent locally, and
// sends one edit when it is let go; while it runs, a redraw from state waits for its end.
"use strict";

(() => {
  const tune = window.tune;
  const page = window.tunePage;
  const dragging = window.tuneDrag;
  const SNAP_MS = dragging.SNAP_MS;
  const PERCENT = 100;
  const MILLISECONDS_PER_SECOND = 1000;
  const MIN_TICK_SPACING_PX = 72;
  const TICK_STEPS_MS = [100, 250, 500, 1000, 2000, 5000, 10000, 15000, 30000, 60000, 120000, 300000];
  const AXIS_NAME = "time";
  const TOO_FAST_GLYPH = "⚠";
  const OWNER_BEAT = "beat";
  const FIELD_CAPTION_SHIFT = "caption.shift";
  const FIELD_ZOOM_AT = "zoom.at";
  const FIELD_POSITION_AT = "position.at";
  const FIELD_POSITION_HOLD = "position.hold";
  const FIELD_POSITION_SPAN = "position.span";
  const FIELD_POSITION_TRANSITION = "position.transition";
  const ENTRY_ZOOM = "zoom";
  const ENTRY_POSITION = page.ENTRY_POSITION;
  const ENTRY_TRANSITION = page.ENTRY_TRANSITION;
  // The entry kinds a shift moves the at of, keeping its beat: the state list each kind's entries
  // are in, and the at field of the edit.
  const AT_SHIFTS = Object.freeze({
    [ENTRY_ZOOM]: { list: "zooms", field: FIELD_ZOOM_AT },
    [ENTRY_POSITION]: { list: "positions", field: FIELD_POSITION_AT },
  });
  const EDGE_START = "start";
  const ADD_GLYPH = "+";

  const root = document.getElementById("timeline");

  // lanes are the timeline's rows under the axis, top to bottom.
  const lanes = [
    { name: "beats", draw: drawBeat, handles: beatHandles },
    { name: "hold", draw: drawHold, handles: holdHandles },
  ];
  // registeredLanes are lanes.js's, drawn under the timeline's own.
  const registeredLanes = window.tuneLanes || [];

  let clipLength = 0;
  let playhead = null;
  let shownTime = null;
  let shownSelection = null;
  let isScrubbing = false;
  // axisTrack is the axis track the timeline shows now; a redraw replaces it.
  let axisTrack = null;

  // --- the lanes ---

  // drawBeat is the section's block, flagged when it plays too fast, or a hatched marker for a cut
  // beat.
  function drawBeat(section, state) {
    const beat = beatOf(state, section.beat);
    const title = beat ? beat.title : "";
    if (section.cut) {
      return [cutMarker(section, state)];
    }
    const isTooFast = isPlayedTooFast(section, state);
    const label = (isTooFast ? TOO_FAST_GLYPH + " " : "") + section.beat + " " + title;
    const block = span(section.start_ms, endOf(section), "block beat", label);
    block.classList.toggle("too-fast", isTooFast);
    block.title = section.report;
    return [selectable(block, section.beat)];
  }

  // beatHandles is the section's end: dragging it sets the beat's duration, kept above its hold.
  function beatHandles(section) {
    if (section.cut) {
      return [];
    }
    return [{
      beat: section.beat,
      field: "duration",
      anchor: section.start_ms,
      sign: 1,
      value: section.duration_ms,
      min: section.hold_ms + SNAP_MS,
      max: Infinity,
    }];
  }

  // drawHold is the section's hold, played at 1×, and the stretch after it at the section's speed.
  function drawHold(section, state) {
    if (section.cut) {
      return [];
    }
    const holdEnd = section.start_ms + section.hold_ms;
    const stretch = span(holdEnd, endOf(section), "block stretch", section.speed.toFixed(2) + "×");
    stretch.classList.toggle("too-fast", isPlayedTooFast(section, state));
    const nodes = [selectable(stretch, section.beat)];
    if (section.hold_ms > 0) {
      nodes.unshift(selectable(span(section.start_ms, holdEnd, "block hold", "hold"), section.beat));
    }
    return nodes;
  }

  // holdHandles is the hold's edge: dragging it sets the beat's hold, kept inside its duration.
  function holdHandles(section) {
    if (section.cut) {
      return [];
    }
    return [{
      beat: section.beat,
      field: "hold",
      anchor: section.start_ms,
      sign: 1,
      value: section.hold_ms,
      min: 0,
      max: section.duration_ms - SNAP_MS,
    }];
  }

  // cutMarker marks a cut beat where it would have played; cuts at one time sit side by side.
  function cutMarker(section, state) {
    const together = state.sections.filter((other) => other.cut && other.start_ms === section.start_ms);
    const index = together.indexOf(section);
    const marker = element("div", { className: "cut-marker", title: "beat " + section.beat + " cut" });
    marker.style.left = percentOf(section.start_ms) + "%";
    marker.style.transform = "translateX(" + (index * PERCENT - PERCENT / 2) + "%)";
    return selectable(marker, section.beat);
  }

  // --- drawing ---

  // render redraws the axis and every lane from state; a drag in progress defers it to its end.
  function render(state) {
    if (dragging.defersRedraw() || !state) {
      return;
    }
    clipLength = state.length_ms;
    playhead = element("div", { className: "playhead" });
    const playheadColumn = element("div", { className: "playhead-column" }, playhead);
    root.replaceChildren(
      axisRow(),
      ...lanes.map((lane) => laneRow(lane, state)),
      ...registeredLanes.map((lane) => clipLaneRow(lane, state)),
      playheadColumn,
    );
    shownTime = null;
    shownSelection = selectionKey();
    markSelection();
    follow();
  }

  function laneRow(lane, state) {
    const track = element("div", { className: "lane-track" });
    for (const section of state.sections) {
      track.append(...lane.draw(section, state));
    }
    for (const section of state.sections) {
      for (const handle of lane.handles(section, state)) {
        track.append(handleNode(handle, track));
      }
    }
    return laneElement(lane.name, track);
  }

  // clipLaneRow is a registered lane: its bars over the whole clip, each selecting its entry, the
  // grips on their edges above them, and its add button when it has one.
  function clipLaneRow(lane, state) {
    const track = element("div", { className: "lane-track" });
    const bars = lane.draw(state);
    track.append(...bars.map((bar) => barNode(bar, track)));
    track.append(...bars.flatMap((bar) => (bar.edges || []).map((side) => gripNode(bar, side, track))));
    const controls = lane.add ? [addButton(lane.add)] : [];
    return laneElement(lane.name, track, ...controls);
  }

  // addButton sends add's edit for the playhead and selects the new entry. A refused add stands
  // on the entry index it named, which the inspector need not show, so its message goes to the
  // transport bar's status as well.
  function addButton(add) {
    const properties = { type: "button", className: "lane-add", title: add.title };
    const button = element("button", properties, ADD_GLYPH);
    button.addEventListener("click", () => {
      const change = add.edit(tune.state(), tune.time());
      tune.edit(change).then(
        () => tune.selectEntry(change.owner),
        (error) => page.reportStatus(error.message, true),
      );
    });
    return button;
  }

  // barNode places a registered lane's bar on track, selecting the bar's entry on a click. It is
  // dragged through window.tuneDrag by a delta in milliseconds that keeps the whole bar on the
  // clip: a drag selects the entry and sends the shiftEdit of its delta; a refusal goes to the
  // transport bar's status.
  function barNode(bar, track) {
    const node = span(bar.from, bar.to, "block draggable " + bar.className, bar.label);
    node.title = bar.title;
    node.dataset.entry = entryKey(bar.entry);
    node.addEventListener("click", () => tune.selectEntry(bar.entry));
    attachShift(bar, node, track);
    return node;
  }

  // attachShift drags node, bar's node on track, to shift bar's entry.
  function attachShift(bar, node, track) {
    dragging.attach({
      node,
      track,
      value: 0,
      min: -bar.from,
      max: Math.max(clipLength - bar.to, 0),
      axis: { msAt, percentOf },
      valueAt: (ms, grabbedMs) => ms - grabbedMs,
      extent: (delta) => barExtent(bar, delta),
      select: () => tune.selectEntry(bar.entry),
      edit: (delta) => shiftEdit(bar, delta),
      redraw: () => render(tune.state()),
      refused: (message) => page.reportStatus(message, true),
    });
  }

  // barExtent draws bar shifted by delta: its left edge, and a ghost over its new stretch
  // labelled with the shift.
  function barExtent(bar, delta) {
    const sign = delta > 0 ? "+" : "";
    const from = bar.from + delta;
    return { edge: from, from, to: bar.to + delta, label: sign + delta + "ms" };
  }

  // shiftEdit is the edit that starts bar's entry, {kind, index}, delta milliseconds later: a
  // caption's shift moves its at and its until together, a zoom's or a position's at keeps its
  // beat and moves its offset, and a transition's edges both move.
  function shiftEdit(bar, delta) {
    const entry = bar.entry;
    if (entry.kind === ENTRY_TRANSITION) {
      return transitionEdit(entry, { from: bar.from + delta, to: bar.to + delta });
    }
    const atShift = AT_SHIFTS[entry.kind];
    if (atShift) {
      return { owner: entry, field: atShift.field, value: movedAt(entry, delta) };
    }
    return { owner: entry, field: FIELD_CAPTION_SHIFT, value: delta };
  }

  // movedAt is the at of entry, a zoom or a position, delta milliseconds later, keeping its beat.
  function movedAt(entry, delta) {
    const list = tune.state()[AT_SHIFTS[entry.kind].list];
    const at = list.find((candidate) => candidate.index === entry.index).at;
    return { beat: at.beat, offset_ms: at.offset_ms + delta };
  }

  // --- resizing a bar by an edge ---

  // gripNode is the grip on side, "start" or "end", of bar on track, dragged through
  // window.tuneDrag by a delta in milliseconds that keeps the edge on the clip and the bar no
  // shorter than 0: a drag selects bar's entry and sends the resizeEdit of the stretch it leaves;
  // a refusal goes to the transport bar's status. A bar of length 0 has its grips beside it, so
  // they leave its marker to the shift drag.
  function gripNode(bar, side, track) {
    const isStart = side === EDGE_START;
    const besideClass = bar.from === bar.to ? " beside-" + side : "";
    const properties = { className: "handle" + besideClass, title: "Drag to resize " + bar.title };
    const node = element("div", properties);
    node.dataset.grip = entryKey(bar.entry);
    node.dataset.edge = side;
    node.style.left = percentOf(isStart ? bar.from : bar.to) + "%";
    dragging.attach({
      node,
      track,
      value: 0,
      min: isStart ? -bar.from : bar.from - bar.to,
      max: isStart ? bar.to - bar.from : Math.max(clipLength - bar.to, 0),
      axis: { msAt, percentOf },
      valueAt: (ms, grabbedMs) => ms - grabbedMs,
      extent: (delta) => resizedExtent(bar, side, delta),
      select: () => tune.selectEntry(bar.entry),
      edit: (delta) => resizeEdit(bar, resized(bar, side, delta)),
      redraw: () => render(tune.state()),
      refused: (message) => page.reportStatus(message, true),
    });
    return node;
  }

  // resized is the stretch, {from, to}, bar spans with its edge on side moved by delta.
  function resized(bar, side, delta) {
    if (side === EDGE_START) {
      return { from: bar.from + delta, to: bar.to };
    }
    return { from: bar.from, to: bar.to + delta };
  }

  // resizedExtent draws bar with its edge on side moved by delta: that edge, and a ghost over the
  // new stretch labelled with its length.
  function resizedExtent(bar, side, delta) {
    const stretch = resized(bar, side, delta);
    const edge = side === EDGE_START ? stretch.from : stretch.to;
    return { edge, from: stretch.from, to: stretch.to, label: stretch.to - stretch.from + "ms" };
  }

  // resizeEdit is the edit that spans bar's entry, a transition or a position, over stretch, one
  // of its edges moved: a transition's edges; a position's hold when its end moved, else its at,
  // keeping its beat, and its hold at once, so its end stays.
  function resizeEdit(bar, stretch) {
    const entry = bar.entry;
    if (entry.kind === ENTRY_TRANSITION) {
      return transitionEdit(entry, stretch);
    }
    const hold = stretch.to - stretch.from;
    if (stretch.from === bar.from) {
      return { owner: entry, field: FIELD_POSITION_HOLD, value: hold };
    }
    const at = movedAt(entry, stretch.from - bar.from);
    return { owner: entry, field: FIELD_POSITION_SPAN, value: { at, hold_ms: hold } };
  }

  // transitionEdit is the edit that moves the edges of the transition entry to stretch: the
  // storyboard owns it through the position it leads into.
  function transitionEdit(entry, stretch) {
    const owner = { kind: ENTRY_POSITION, index: entry.index };
    return { owner, field: FIELD_POSITION_TRANSITION, value: { start_ms: stretch.from, end_ms: stretch.to } };
  }

  // laneElement is a lane's row: its name, with controls after it, beside its track.
  function laneElement(name, track, ...controls) {
    const label = element("span", { className: "lane-name" }, name, ...controls);
    return element("div", { className: "lane" }, label, track);
  }

  // axisRow is the time scale: a tick every step that keeps the labels apart; a click or drag
  // on it scrubs the playhead.
  function axisRow() {
    const track = element("div", { className: "lane-track axis", title: "Click or drag to scrub" });
    const step = tickStep(root.clientWidth);
    for (let ms = 0; step > 0 && ms < clipLength; ms += step) {
      const tick = element("span", { className: "tick" }, ms / MILLISECONDS_PER_SECOND + "s");
      tick.style.left = percentOf(ms) + "%";
      track.append(tick);
    }
    track.addEventListener("pointerdown", startScrub);
    axisTrack = track;
    return element("div", { className: "lane" }, element("span", { className: "lane-name" }, AXIS_NAME), track);
  }

  // tickStep is the smallest step whose ticks sit MIN_TICK_SPACING_PX apart across width;
  // the timeline's whole width stands in for a track's, which is only a lane name narrower.
  function tickStep(width) {
    if (clipLength <= 0 || width <= 0) {
      return 0;
    }
    const step = TICK_STEPS_MS.find((candidate) => (candidate / clipLength) * width >= MIN_TICK_SPACING_PX);
    return step || TICK_STEPS_MS[TICK_STEPS_MS.length - 1];
  }

  // follow keeps the playhead on tune.time() and the selected beat or entry marked, every
  // animation frame.
  function follow() {
    const at = tune.time();
    if (playhead && at !== shownTime) {
      shownTime = at;
      playhead.style.left = percentOf(at) + "%";
    }
    const selection = selectionKey();
    if (selection !== shownSelection) {
      shownSelection = selection;
      markSelection();
    }
  }

  function followLoop() {
    follow();
    requestAnimationFrame(followLoop);
  }

  function markSelection() {
    const beat = tune.selected();
    const entry = entryKey(tune.selectedEntry());
    for (const node of root.querySelectorAll("[data-beat]")) {
      node.classList.toggle("selected", Number(node.dataset.beat) === beat);
    }
    for (const node of root.querySelectorAll("[data-entry]")) {
      node.classList.toggle("selected", node.dataset.entry === entry);
    }
  }

  // selectionKey names what the inspector shows: the selected entry's key, else the beat's id.
  function selectionKey() {
    const entry = tune.selectedEntry();
    return entry ? entryKey(entry) : tune.selected();
  }

  // entryKey spells entry, {kind, index}, as one string; no entry is the empty string.
  function entryKey(entry) {
    return entry ? entry.kind + ":" + entry.index : "";
  }

  // --- dragging a handle ---

  // handleNode is handle's edge on track, dragged through window.tuneDrag: a drag selects the
  // handle's beat and sends the edit of the handle's field. A refusal stands on that beat, which
  // the drag selected, so the inspector shows it and nothing else need.
  function handleNode(handle, track) {
    const node = element("div", { className: "handle", title: "Drag to set " + handle.field });
    node.style.left = percentOf(edgeOf(handle, handle.value)) + "%";
    dragging.attach({
      node,
      track,
      value: handle.value,
      min: handle.min,
      max: handle.max,
      axis: { msAt, percentOf },
      valueAt: (ms) => handle.sign * (ms - handle.anchor),
      extent: (value) => handleExtent(handle, value),
      select: () => tune.select(handle.beat),
      edit: (value) => ({ owner: { kind: OWNER_BEAT, index: handle.beat }, field: handle.field, value }),
      redraw: () => render(tune.state()),
      refused: () => {},
    });
    return node;
  }

  // handleExtent draws handle at value: its edge, and a ghost from its anchor to the edge
  // labelled with the value the edit will send.
  function handleExtent(handle, value) {
    const edge = edgeOf(handle, value);
    return {
      edge,
      from: Math.min(edge, handle.anchor),
      to: Math.max(edge, handle.anchor),
      label: handle.field + " " + value + "ms",
    };
  }

  // --- scrubbing the axis ---

  // startScrub seeks to the press on the axis and follows the pointer on the window until it is
  // let go: the axis it was pressed on may be replaced by a redraw before then.
  function startScrub(event) {
    if (clipLength <= 0) {
      return;
    }
    event.preventDefault();
    isScrubbing = true;
    // Marked before the pause, so pausing a play to scrub does not settle the preview.
    page.beginScrub();
    axisTrack.setPointerCapture(event.pointerId);
    window.addEventListener("pointermove", moveScrub);
    window.addEventListener("pointerup", endScrub);
    window.addEventListener("pointercancel", endScrub);
    tune.pause();
    seekTo(event);
  }

  // moveScrub seeks to the pointer, measured against the axis the timeline shows now.
  function moveScrub(event) {
    if (isScrubbing) {
      seekTo(event);
    }
  }

  // endScrub ends the scrub once, however many releases arrive, and settles the preview.
  function endScrub() {
    if (!isScrubbing) {
      return;
    }
    isScrubbing = false;
    window.removeEventListener("pointermove", moveScrub);
    window.removeEventListener("pointerup", endScrub);
    window.removeEventListener("pointercancel", endScrub);
    page.endScrub();
  }

  // seekTo moves the playhead to the output time under the pointer of event on the axis.
  function seekTo(event) {
    tune.seek(msAt(event.clientX, axisTrack.getBoundingClientRect()));
  }

  // --- helpers ---

  // span is a node covering output times from to to on its lane, labelled.
  function span(from, to, className, label) {
    const node = element("div", { className, title: label }, element("span", { className: "label" }, label));
    node.style.left = percentOf(from) + "%";
    node.style.width = percentOf(Math.max(to - from, 0)) + "%";
    return node;
  }

  // selectable marks node as beat's: a click shows the beat in the inspector.
  function selectable(node, beat) {
    node.dataset.beat = String(beat);
    node.addEventListener("click", () => tune.select(beat));
    return node;
  }

  function edgeOf(handle, value) {
    return handle.anchor + handle.sign * value;
  }

  function endOf(section) {
    return section.start_ms + section.duration_ms;
  }

  // msAt is the output time under clientX on a lane whose box is rect.
  function msAt(clientX, rect) {
    if (rect.width <= 0) {
      return 0;
    }
    return ((clientX - rect.left) / rect.width) * clipLength;
  }

  function percentOf(ms) {
    return clipLength > 0 ? (ms / clipLength) * PERCENT : 0;
  }

  // isPlayedTooFast says the schedule warns that section plays faster than it allows (6×): its
  // warning names the beat first, as `beat <id> plays at …`.
  function isPlayedTooFast(section, state) {
    return state.warnings.some((warning) => warning.startsWith("beat " + section.beat + " "));
  }

  function beatOf(state, id) {
    return state.beats.find((beat) => beat.id === id);
  }

  // element builds a tag with properties set and children appended; strings become text.
  function element(tag, properties, ...children) {
    const node = Object.assign(document.createElement(tag), properties);
    node.append(...children);
    return node;
  }

  tune.onState(render);
  new ResizeObserver(() => render(tune.state())).observe(root);
  render(tune.state());
  requestAnimationFrame(followLoop);
})();
