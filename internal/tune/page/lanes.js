// photon tune's camera and caption lanes: the storyboard's camera moves and captions on the
// timeline, one bar per entry from where it starts to where it stops on the output clock.
//
// The lanes register in window.tuneLanes, which timeline.js reads, so this file loads before it.
// A lane here is {name, draw(state), add}: draw takes the whole state and returns the lane's bars
// as plain data, {from, to, className, label, title, entry}, which the timeline places on its axis
// and makes selectable as entry, {kind, index}, the shape window.tune's selectEntry takes. add is
// the lane's add button, {title, edit(state, ms)}: edit is the window.tune edit that adds a new
// entry starting at ms on the output clock, its owner the entry it becomes. Nothing here touches
// the page or computes timing: every bar spans a start and stop the state reports.
"use strict";

(() => {
  const ENTRY_CAMERA = "camera";
  const ENTRY_CAPTION = "caption";
  const KIND_ZOOM = "zoom";
  const KIND_PAN = "pan";
  const KIND_CAPTION = "caption";
  // A moment's beat 0 is a clip time, offset_ms after the clip's first frame.
  const CLIP_TIME_BEAT = 0;

  // cameraBars are state's camera moves of kind, in storyboard order, as bars of className.
  function cameraBars(state, kind, className) {
    return state.camera
      .filter((move) => move.kind === kind)
      .map((move) => {
        const label = kind === KIND_ZOOM ? zoomLabel(move) : panLabel(move);
        return {
          from: move.start_ms,
          to: move.end_ms,
          className,
          label,
          title: ENTRY_CAMERA + " " + move.index + " " + move.kind + " " + label,
          entry: { kind: ENTRY_CAMERA, index: move.index },
        };
      });
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

  // zoomLabel is the magnification a zoom move ends at.
  function zoomLabel(move) {
    return "to " + move.factor + "×";
  }

  // panLabel is where a pan move aims: its target's text, its cell, or the points of its path.
  function panLabel(move) {
    if (move.target) {
      return "to " + move.target.text;
    }
    if (move.cell) {
      return "to cell " + move.cell.x + "," + move.cell.y;
    }
    return "via " + move.via_count + " points";
  }

  // adding is the add button of a lane whose new entry, of kind, field inserts into list: its
  // title names noun, and its edit places the entry in start-time order, after every entry of the
  // list that starts at or before the playhead.
  function adding(noun, list, field, kind) {
    return {
      title: "Add a " + noun + " at the playhead",
      edit: (state, ms) => {
        const entries = list === ENTRY_CAMERA ? state.camera : state.captions;
        const index = entries.filter((entry) => entry.start_ms <= ms).length;
        const at = { beat: CLIP_TIME_BEAT, offset_ms: Math.round(ms) };
        return { owner: { kind: list, index }, field, value: { kind, at } };
      },
    };
  }

  // window.tuneLanes are the lanes timeline.js draws under its own, top to bottom.
  window.tuneLanes = Object.freeze([
    {
      name: "camera zoom",
      draw: (state) => cameraBars(state, KIND_ZOOM, "camera-zoom"),
      add: adding("zoom", ENTRY_CAMERA, "camera.insert", KIND_ZOOM),
    },
    {
      name: "pan",
      draw: (state) => cameraBars(state, KIND_PAN, "pan"),
      add: adding("pan", ENTRY_CAMERA, "camera.insert", KIND_PAN),
    },
    {
      name: "captions",
      draw: captionBars,
      add: adding("caption", ENTRY_CAPTION, "caption.insert", KIND_CAPTION),
    },
  ]);
})();
