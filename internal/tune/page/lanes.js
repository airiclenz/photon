// photon tune's caption lane: the storyboard's captions on the timeline, one bar per entry from
// where it starts to where it stops on the output clock.
//
// The lanes register in window.tuneLanes, which timeline.js reads, so this file loads before it.
// A lane here is {name, draw(state), add}: draw takes the whole state and returns the lane's bars
// as plain data, {from, to, className, label, title, entry}, which the timeline places on its axis,
// makes selectable as entry, {kind, index}, the shape window.tune's selectEntry takes, and drags
// to shift that entry's start. add is the lane's add button, {title, edit(state, ms)}: edit is the
// window.tune edit that adds a new entry starting at ms on the output clock, its owner the entry
// it becomes. Nothing here touches the page or computes timing: every bar spans a start and stop
// the state reports.
"use strict";

(() => {
  const ENTRY_CAPTION = "caption";
  const KIND_CAPTION = "caption";
  // The state's list of each entry kind a lane adds to.
  const STATE_LISTS = Object.freeze({ [ENTRY_CAPTION]: "captions" });
  // A moment's beat 0 is a clip time, offset_ms after the clip's first frame.
  const CLIP_TIME_BEAT = 0;

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

  // adding is the add button of a lane whose new entry, of kind, field inserts into list: its
  // title names noun, and its edit places the entry in start-time order, after every entry of the
  // list that starts at or before the playhead.
  function adding(noun, list, field, kind) {
    return {
      title: "Add a " + noun + " at the playhead",
      edit: (state, ms) => {
        const index = state[STATE_LISTS[list]].filter((entry) => entry.start_ms <= ms).length;
        const at = { beat: CLIP_TIME_BEAT, offset_ms: Math.round(ms) };
        return { owner: { kind: list, index }, field, value: { kind, at } };
      },
    };
  }

  // window.tuneLanes are the lanes timeline.js draws under its own, top to bottom.
  window.tuneLanes = Object.freeze([
    {
      name: "captions",
      draw: captionBars,
      add: adding("caption", ENTRY_CAPTION, "caption.insert", KIND_CAPTION),
    },
  ]);
})();
