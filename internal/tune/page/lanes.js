// photon tune's position, zoom and caption lanes: the storyboard's camera positions with the
// transitions between them, its camera zooms and its captions on the timeline, one bar per entry
// from where it starts to where it ends on the output clock.
//
// The lanes register in window.tuneLanes, which timeline.js reads, so this file loads before it.
// A lane here is {name, draw(state), add}: draw takes the whole state and returns the lane's bars
// as plain data, {from, to, className, label, title, entry}, which the timeline places on its axis,
// makes selectable as entry, {kind, index}, the shape window.tune's selectEntry takes, and drags
// to shift that entry's start unless the bar says draggable: false. add, which a lane may leave
// out, is the lane's add button, {title, edit(state, ms)}: edit is the window.tune edit that adds
// a new entry starting at ms on the output clock, its owner the entry it becomes. Nothing here
// touches the page or computes timing: every bar spans a start and stop the state reports, and
// the transitions are kinds.js's, read through window.tunePage, which loads before this file.
"use strict";

(() => {
  const page = window.tunePage;
  const ENTRY_POSITION = page.ENTRY_POSITION;
  const ENTRY_TRANSITION = page.ENTRY_TRANSITION;
  const ENTRY_ZOOM = "zoom";
  const ENTRY_CAPTION = "caption";
  const KIND_CAPTION = "caption";
  // The state's list of each entry kind a lane adds to.
  const STATE_LISTS = Object.freeze({ [ENTRY_ZOOM]: "zooms", [ENTRY_CAPTION]: "captions" });
  // A moment's beat 0 is a clip time, offset_ms after the clip's first frame.
  const CLIP_TIME_BEAT = 0;

  // positionBars are state's camera positions and the transitions between them, by start: each
  // transition, in its own colour, spans from the end of the position before it to the start of
  // the one it leads into, so it meets both; each position covers its hold, or is a point at its
  // start when it holds for 0. None of them drags.
  function positionBars(state) {
    const transitionBars = page.transitions(state).map((transition) => ({
      from: transition.start_ms,
      to: transition.end_ms,
      className: "transition" + pointClass(transition),
      label: transition.ease,
      title: ENTRY_TRANSITION + " from position " + transition.from + " into position " + transition.index,
      entry: { kind: ENTRY_TRANSITION, index: transition.index },
      draggable: false,
    }));
    const positions = page.byStart(state.positions).map((position) => ({
      from: position.start_ms,
      to: position.end_ms,
      className: "position" + pointClass(position),
      label: page.aimText(position),
      title: ENTRY_POSITION + " " + position.index + " " + page.aimText(position),
      entry: { kind: ENTRY_POSITION, index: position.index },
      draggable: false,
    }));
    return [...transitionBars, ...positions];
  }

  // pointClass marks a stretch that starts where it ends as a point in time.
  function pointClass(stretch) {
    return stretch.end_ms === stretch.start_ms ? " point" : "";
  }

  // zoomBars are state's camera zooms, in storyboard order, each labelled with the factor it
  // reaches.
  function zoomBars(state) {
    return state.zooms.map((zoom) => ({
      from: zoom.start_ms,
      to: zoom.end_ms,
      className: "zoom",
      label: zoomLabel(zoom),
      title: ENTRY_ZOOM + " " + zoom.index + " " + zoomLabel(zoom),
      entry: { kind: ENTRY_ZOOM, index: zoom.index },
    }));
  }

  // zoomLabel is the factor a zoom reaches, as a magnification.
  function zoomLabel(zoom) {
    return zoom.factor + "×";
  }

  // captionBars are state's captions, in storyboard order, each labelled with its text.
  function captionBars(state) {
    return state.captions.map((caption) => ({
      from: caption.start_ms,
      to: caption.stop_ms,
      className: "caption",
      label: caption.text,
      title: ENTRY_CAPTION + " " + caption.index + " " + caption.text,
      entry: { kind: ENTRY_CAPTION, index: caption.index },
    }));
  }

  // adding is the add button of a lane whose new entry, newEntry(at) starting at the moment at,
  // field inserts into list: its title names noun, and its edit places the entry in start-time
  // order, after every entry of the list that starts at or before the playhead.
  function adding(noun, list, field, newEntry) {
    return {
      title: "Add a " + noun + " at the playhead",
      edit: (state, ms) => {
        const index = state[STATE_LISTS[list]].filter((entry) => entry.start_ms <= ms).length;
        const at = { beat: CLIP_TIME_BEAT, offset_ms: Math.round(ms) };
        return { owner: { kind: list, index }, field, value: newEntry(at) };
      },
    };
  }

  // window.tuneLanes are the lanes timeline.js draws under its own, top to bottom.
  window.tuneLanes = Object.freeze([
    {
      name: "position",
      draw: positionBars,
    },
    {
      name: "zoom",
      draw: zoomBars,
      add: adding("zoom", ENTRY_ZOOM, "zoom.insert", (at) => ({ at })),
    },
    {
      name: "captions",
      draw: captionBars,
      add: adding("caption", ENTRY_CAPTION, "caption.insert", (at) => ({ kind: KIND_CAPTION, at })),
    },
  ]);
})();
