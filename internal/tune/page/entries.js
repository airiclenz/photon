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
    textField,
    choiceField,
    errorRows,
    momentText,
    flowTarget,
    MOMENT_PLACEHOLDER,
    TARGET_PLACEHOLDER,
  } = page;
  const KIND_ZOOM = "zoom";
  const EASES = ["smooth", "linear"];
  const PLACES = ["top", "middle", "bottom"];
  const ALIGNS = ["left", "middle", "right"];
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
      textField("at", owner, "camera.at", momentText(move.at), MOMENT_PLACEHOLDER),
      millisecondsField("over", owner, "camera.over", move.over_ms),
      choiceField("ease", owner, "camera.ease", EASES, move.ease),
      aimRow(owner, move),
      readOnlyRow("plays", formatTime(move.start_ms) + " – " + formatTime(move.end_ms)),
      ...errorRows(owner),
    ];
  }

  // aimRow edits where a camera move goes: a zoom's factor, or a pan's target or cell; the
  // points of a pan's path show read-only.
  function aimRow(owner, move) {
    if (move.kind === KIND_ZOOM) {
      const input = element("input", { type: "number", step: "0.1", value: String(move.factor) });
      input.addEventListener("change", () => applyEdit(owner, "camera.to", input.value.trim()));
      return fieldRow("to (×)", input);
    }
    if (move.target) {
      return textField("to", owner, "camera.to", flowTarget(move.target), TARGET_PLACEHOLDER);
    }
    if (move.cell) {
      const cell = "{cell: {x: " + move.cell.x + ", y: " + move.cell.y + "}}";
      return textField("to", owner, "camera.to", cell, TARGET_PLACEHOLDER);
    }
    return readOnlyRow("via", move.via_count + " points");
  }

  // captionPanel edits a caption's text, timing and placement, and shows when it is on the
  // output clock and the errors standing on it. It ends after for or at until: either field
  // sets its end and drops the other. A caption set by position has no place or align to edit.
  function captionPanel(caption) {
    const owner = { kind: ENTRY_CAPTION, index: caption.index };
    const placement = caption.place
      ? [
          choiceField("place", owner, "caption.place", PLACES, caption.place),
          choiceField("align", owner, "caption.align", ALIGNS, caption.align),
        ]
      : [];
    return [
      element("h2", {}, "caption " + caption.index),
      captionTextField(owner, caption.text),
      textField("at", owner, "caption.at", momentText(caption.at), MOMENT_PLACEHOLDER),
      captionForField(owner, caption),
      captionUntilField(owner, caption),
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
    input.addEventListener("change", () => applyEdit(owner, "caption.for", input.value.trim()));
    return fieldRow("for (ms)", input);
  }

  // captionUntilField edits the moment a caption ends at; it is empty while the caption ends
  // after for.
  function captionUntilField(owner, caption) {
    const until = caption.until ? momentText(caption.until) : "";
    return textField("until", owner, "caption.until", until, MOMENT_PLACEHOLDER);
  }

  // captionTextField edits a caption's text, its lines as the textarea's lines.
  function captionTextField(owner, text) {
    const area = element("textarea", { rows: CAPTION_TEXT_ROWS, value: text });
    area.addEventListener("change", () => applyEdit(owner, "caption.text", area.value));
    return fieldRow("text", area);
  }

  Object.assign(page, { entryPanel });
})();
