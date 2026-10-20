// photon tune's entry kinds: the camera zooms and captions the inspector shows in place of a beat.
// An entry is {kind, index}: kind one of ENTRY_KINDS' kinds, index the entry's place in that
// kind's state list. The beat select spells it kind:index, which no beat id can be.
//
// It loads after app.js and before controls.js, and adds to window.tunePage, the page's internal
// namespace, the entry kinds' names and the lookups the selection, the inspector and its error
// rows build on: entryValue and entryOfValue spell an entry as a beat select value and back,
// isEntryKind tells an entry's kind from a beat's or the clip's, entryOf finds an entry in the
// current state, and entryOptions are the beat select's options for every entry. Another entry
// kind is one more row of ENTRY_KINDS.
"use strict";

(() => {
  const page = window.tunePage;
  const ENTRY_ZOOM = "zoom";
  const ENTRY_CAPTION = "caption";
  const ENTRY_SEPARATOR = ":";
  // ENTRY_KINDS are the entry kinds the inspector shows, in the beat select's order: each kind's
  // state list, and the text that names one of its entries after its kind and index.
  const ENTRY_KINDS = Object.freeze([
    { kind: ENTRY_ZOOM, list: "zooms", name: (zoom) => "to " + zoom.factor + "×" },
    { kind: ENTRY_CAPTION, list: "captions", name: (caption) => caption.text },
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

  // entryOf is the zoom or caption the entry, {kind, index}, names in the current state, or
  // undefined when the state has none.
  function entryOf(entry) {
    const entryKind = kindOf(entry.kind);
    return entryKind && page.state()[entryKind.list].find((item) => item.index === entry.index);
  }

  // entryOptions are the beat select's options for every entry of the current state, by kind in
  // ENTRY_KINDS' order, each marked when an error stands on it.
  function entryOptions() {
    const state = page.state();
    return ENTRY_KINDS.flatMap(({ kind, list, name }) =>
      state[list].map((entry) => {
        const mark = page.errorMark({ kind, index: entry.index });
        return page.option(entryValue(kind, entry.index), kind + " " + entry.index + " " + name(entry) + mark);
      }),
    );
  }

  // kindOf is the row of ENTRY_KINDS for kind, or undefined for a beat's or the clip's kind.
  function kindOf(kind) {
    return ENTRY_KINDS.find((entryKind) => entryKind.kind === kind);
  }

  Object.assign(page, { ENTRY_ZOOM, ENTRY_CAPTION, entryValue, entryOfValue, isEntryKind, entryOf, entryOptions });
})();
