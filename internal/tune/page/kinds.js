// photon tune's entry kinds: the camera positions, the transitions between them, the camera zooms
// and the captions the inspector shows in place of a beat. An entry is {kind, index}: kind one of
// ENTRY_KINDS' kinds, index the entry's place in that kind's state list — for a transition, the
// place in state.positions of the position it leads into. The beat select spells it kind:index,
// which no beat id can be.
//
// It loads after app.js and before controls.js, and adds to window.tunePage, the page's internal
// namespace, the entry kinds' names and the lookups the selection, the inspector and its error
// rows build on: entryValue and entryOfValue spell an entry as a beat select value and back,
// isEntryKind tells an entry's kind from a beat's or the clip's, entryOf finds an entry in the
// current state, entryOptions are the beat select's options for every entry, transitions derives
// the transitions from a state's positions, byStart sorts entries by start, and aimText says
// where a position aims. Another entry kind is one more row of ENTRY_KINDS.
"use strict";

(() => {
  const page = window.tunePage;
  const ENTRY_POSITION = "position";
  const ENTRY_TRANSITION = "transition";
  const ENTRY_ZOOM = "zoom";
  const ENTRY_CAPTION = "caption";
  const ENTRY_SEPARATOR = ":";
  // ENTRY_KINDS are the entry kinds the inspector shows, in the beat select's order: each kind's
  // entries in a state, and the text that names one of them after its kind and index.
  const ENTRY_KINDS = Object.freeze([
    { kind: ENTRY_POSITION, entries: (state) => state.positions, name: aimText },
    { kind: ENTRY_TRANSITION, entries: transitions, name: (transition) => "from position " + transition.from },
    { kind: ENTRY_ZOOM, entries: (state) => state.zooms, name: (zoom) => "to " + zoom.factor + "×" },
    { kind: ENTRY_CAPTION, entries: (state) => state.captions, name: (caption) => caption.text },
  ]);

  // entryValue spells an entry as a beat select value, which no beat id can be.
  function entryValue(kind, index) {
    return kind + ENTRY_SEPARATOR + index;
  }

  // entryOfValue is the entry, {kind, index}, a beat select value names, or null for the value
  // of the clip's frame or of a beat.
  function entryOfValue(value) {
    const [kind, index] = value.split(ENTRY_SEPARATOR);
    return index === undefined ? null : { kind, index: Number(index) };
  }

  // isEntryKind says kind is an entry kind's, not a beat's or the clip's.
  function isEntryKind(kind) {
    return kindOf(kind) !== undefined;
  }

  // entryOf is the position, transition, zoom or caption the entry, {kind, index}, names in the
  // current state, or undefined when the state has none.
  function entryOf(entry) {
    const entryKind = kindOf(entry.kind);
    return entryKind && entryKind.entries(page.state()).find((item) => item.index === entry.index);
  }

  // entryOptions are the beat select's options for every entry of the current state, by kind in
  // ENTRY_KINDS' order, each marked when an error stands on it.
  function entryOptions() {
    const state = page.state();
    return ENTRY_KINDS.flatMap(({ kind, entries, name }) =>
      entries(state).map((entry) => {
        const mark = page.errorMark({ kind, index: entry.index });
        return page.option(entryValue(kind, entry.index), kind + " " + entry.index + " " + name(entry) + mark);
      }),
    );
  }

  // transitions are the camera transitions of state, by start: one into each position but the
  // first by start, from the end of the position before it by start — the latest to start
  // earlier, the first listed of those that start together — to the position's own start. A
  // transition is {index, from, start_ms, end_ms, ease}: index and ease are the position it leads
  // into's, the selection and edits it owns, and from is the index of the position before it.
  function transitions(state) {
    return byStart(state.positions).flatMap((into) => {
      const before = positionBefore(state.positions, into);
      if (!before) {
        return [];
      }
      return [{
        index: into.index,
        from: before.index,
        start_ms: before.end_ms,
        end_ms: into.start_ms,
        ease: into.ease,
      }];
    });
  }

  // positionBefore is the position of positions that comes before into by start, or null for
  // the first by start.
  function positionBefore(positions, into) {
    let before = null;
    for (const candidate of positions) {
      const isEarlier = candidate.start_ms < into.start_ms;
      if (isEarlier && (!before || candidate.start_ms > before.start_ms)) {
        before = candidate;
      }
    }
    return before;
  }

  // byStart is entries sorted by start_ms, those that start together kept in list order.
  function byStart(entries) {
    return [...entries].sort((first, second) => first.start_ms - second.start_ms);
  }

  // aimText is where a position aims: its text target's text, or its cell as (x, y).
  function aimText(position) {
    if (position.cell) {
      return "(" + position.cell.x + ", " + position.cell.y + ")";
    }
    return position.target.text;
  }

  // kindOf is the row of ENTRY_KINDS for kind, or undefined for a beat's or the clip's kind.
  function kindOf(kind) {
    return ENTRY_KINDS.find((entryKind) => entryKind.kind === kind);
  }

  Object.assign(page, {
    ENTRY_POSITION,
    ENTRY_TRANSITION,
    ENTRY_ZOOM,
    ENTRY_CAPTION,
    entryValue,
    entryOfValue,
    isEntryKind,
    entryOf,
    entryOptions,
    transitions,
    byStart,
    aimText,
  });
})();
