// photon tune's camera move and caption panels: what the inspector shows for a selected entry —
// its timing, where a camera move goes, a caption's text and placement, when it is on the output
// clock and the errors standing on it.
//
// It loads after controls.js and adds entryPanel to window.tunePage, the page's internal
// namespace, where app.js's inspector draws it; every field is a control from controls.js.
"use strict";

(() => {
  const page = window.tunePage;
  const {
    ENTRY_CAMERA,
    ENTRY_CAPTION,
    applyEdit,
    element,
    formatTime,
    fieldRow,
    readOnlyRow,
    millisecondsField,
    momentField,
    choiceField,
    errorRows,
    numberOf,
    targetInputs,
    targetRows,
    targetOf,
  } = page;
  const KIND_ZOOM = "zoom";
  const CAPTION_TEXT_ROWS = 3;

  // entryPanel is a camera move's or caption's fields; what the page does not edit shows
  // read-only, and the YAML edits it.
  function entryPanel(kind, entry) {
    return kind === ENTRY_CAMERA ? cameraPanel(entry) : captionPanel(entry);
  }

  // cameraPanel edits a camera move's timing and where it goes, and shows when it plays on the
  // output clock and the errors standing on it.
  function cameraPanel(move) {
    const owner = { kind: ENTRY_CAMERA, index: move.index };
    return [
      element("h2", {}, "camera " + move.index + " " + move.kind),
      momentField("at", owner, "camera.at", move.at),
      millisecondsField("over", owner, "camera.over", move.over_ms),
      choiceField("ease", owner, "camera.ease", move.ease),
      ...aimRows(owner, move),
      readOnlyRow("plays", formatTime(move.start_ms) + " – " + formatTime(move.end_ms)),
      ...errorRows(owner),
    ];
  }

  // aimRows edit where a camera move goes, each sending a typed camera.to: a zoom's factor, or
  // a pan's target or cell, which a pick replaces with a target; the points of a pan's path
  // show read-only.
  function aimRows(owner, move) {
    if (move.kind === KIND_ZOOM) {
      const input = element("input", { type: "number", step: "0.1", value: String(move.factor) });
      input.addEventListener("change", () => applyEdit(owner, "camera.to", numberOf(input.value)));
      return [fieldRow("to (×)", input)];
    }
    if (move.target) {
      const inputs = targetInputs(move.target);
      for (const input of Object.values(inputs)) {
        input.addEventListener("change", () => applyEdit(owner, "camera.to", targetOf(inputs)));
      }
      return targetRows("to", inputs);
    }
    if (move.cell) {
      return [cellRow(owner, move.cell)];
    }
    return [readOnlyRow("via", move.via_count + " points")];
  }

  // cellRow edits the cell a pan goes to, its column x and row y from 0.
  function cellRow(owner, cell) {
    const input = (value) =>
      element("input", { type: "number", min: "0", step: "1", value: String(value) });
    const x = input(cell.x);
    const y = input(cell.y);
    const send = () =>
      applyEdit(owner, "camera.to", { cell: { x: numberOf(x.value), y: numberOf(y.value) } });
    x.addEventListener("change", send);
    y.addEventListener("change", send);
    return fieldRow("to (x, y)", element("span", { className: "inline" }, x, y));
  }

  // captionPanel edits a caption's text, timing and placement, and shows when it is on the
  // output clock and the errors standing on it. It ends after for or at until: either field
  // sets its end and drops the other, and an until of none ends it after a for of the stretch it
  // shows. A caption set by position has no place or align to edit.
  function captionPanel(caption) {
    const owner = { kind: ENTRY_CAPTION, index: caption.index };
    const placement = caption.place
      ? [
          choiceField("place", owner, "caption.place", caption.place),
          choiceField("align", owner, "caption.align", caption.align),
        ]
      : [];
    return [
      element("h2", {}, "caption " + caption.index),
      captionTextField(owner, caption.text),
      momentField("at", owner, "caption.at", caption.at),
      captionForField(owner, caption),
      momentField("until", owner, "caption.until", caption.until || null, true),
      ...placement,
      readOnlyRow("shows", formatTime(caption.start_ms) + " – " + formatTime(caption.stop_ms)),
      ...errorRows(owner),
    ];
  }

  // captionForField edits how long a caption shows; it is empty while the caption ends at until.
  function captionForField(owner, caption) {
    const input = element("input", {
      type: "number",
      min: "1",
      step: "1",
      value: caption.until ? "" : String(caption.for_ms),
    });
    input.addEventListener("change", () => applyEdit(owner, "caption.for", numberOf(input.value)));
    return fieldRow("for (ms)", input);
  }

  // captionTextField edits a caption's text, its lines as the textarea's lines.
  function captionTextField(owner, text) {
    const area = element("textarea", { rows: CAPTION_TEXT_ROWS, value: text });
    area.addEventListener("change", () => applyEdit(owner, "caption.text", area.value));
    return fieldRow("text", area);
  }

  Object.assign(page, { entryPanel });
})();
