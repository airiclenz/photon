// photon tune's caption panel: what the inspector shows for a selected entry — a caption's text,
// timing, placement and style, when it is on the output clock and the errors standing on it — and
// the buttons that move it up or down its list or remove it.
//
// It loads after controls.js and adds entryPanel to window.tunePage, the page's internal
// namespace, where app.js's inspector draws it; every field is a control from controls.js.
"use strict";

(() => {
  const page = window.tunePage;
  const {
    ENTRY_CAPTION,
    applyEdit,
    element,
    formatTime,
    fieldRow,
    readOnlyRow,
    millisecondsField,
    momentField,
    choiceField,
    decimalField,
    colorField,
    fractionField,
    pixelsField,
    errorRows,
    numberOf,
  } = page;
  const CAPTION_TEXT_ROWS = 3;
  // The step a caption's font size is typed in, in the unit of the frame's font size.
  const FONT_SIZE_STEP = "0.5";
  // The list fields that move an entry within its list, or remove it, by list.
  const CAPTION_LIST = Object.freeze({ move: "caption.move", remove: "caption.remove" });

  // The panel of each entry kind the inspector shows.
  const PANELS = Object.freeze({ [ENTRY_CAPTION]: captionPanel });

  // entryPanel is the fields of an entry of kind; what the page does not edit shows read-only,
  // and the YAML edits it.
  function entryPanel(kind, entry) {
    return PANELS[kind](entry);
  }

  // captionPanel edits a caption's text, timing, placement and style, and shows when it is on
  // the output clock and the errors standing on it. It ends after for or at until: either field
  // sets its end and drops the other, and an until of none ends it after a for of the stretch it
  // shows. The panel ends with its list buttons.
  function captionPanel(caption) {
    const owner = { kind: ENTRY_CAPTION, index: caption.index };
    return [
      element("h2", {}, "caption " + caption.index),
      captionTextField(owner, caption.text),
      momentField("at", owner, "caption.at", caption.at),
      captionForField(owner, caption),
      momentField("until", owner, "caption.until", caption.until || null, true),
      ...placementRows(owner, caption),
      ...styleRows(owner, caption),
      readOnlyRow("shows", formatTime(caption.start_ms) + " – " + formatTime(caption.stop_ms)),
      ...errorRows(owner),
      listRow(owner, CAPTION_LIST, page.state().captions.length),
    ];
  }

  // placementRows edit where a caption sits: its place, its align and its offset from where those
  // put it, or, with explicit position checked, its position, which comes instead of all three.
  function placementRows(owner, caption) {
    const toggle = positionToggle(owner, caption);
    if (caption.position) {
      return [toggle, pixelsField("position", owner, "caption.position", caption.position)];
    }
    return [
      choiceField("place", owner, "caption.place", caption.place),
      choiceField("align", owner, "caption.align", caption.align),
      pixelsField("offset", owner, "caption.offset", caption.offset),
      toggle,
    ];
  }

  // positionToggle is the explicit position checkbox. Checking it sets the caption's position to
  // the top-left of the box it is laid out in now, so it stays where it is; unchecking it drops
  // the position, and the caption sits where an absent place and align put it. It is disabled
  // while the caption has no laid-out box to seed the position from.
  function positionToggle(owner, caption) {
    const corner = caption.box_position;
    const input = element("input", {
      type: "checkbox",
      checked: Boolean(caption.position),
      disabled: !corner,
    });
    input.addEventListener("change", () =>
      applyEdit(owner, "caption.position", input.checked ? { x: corner.x, y: corner.y } : null),
    );
    return fieldRow("explicit position", input);
  }

  // styleRows edit a caption's font, its text and box colours, its box's opacity and its fade.
  function styleRows(owner, caption) {
    return [
      decimalField("font size", owner, "caption.font.size", caption.font_size, FONT_SIZE_STEP),
      choiceField("weight", owner, "caption.font.weight", caption.font_weight),
      colorField("colour", owner, "caption.color", caption.color),
      colorField("box colour", owner, "caption.box.color", caption.box_color),
      fractionField("box opacity", owner, "caption.box.opacity", caption.box_opacity),
      millisecondsField("fade in", owner, "caption.fade.in", caption.fade_in_ms),
      millisecondsField("fade out", owner, "caption.fade.out", caption.fade_out_ms),
    ];
  }

  // listRow is the buttons that move owner's entry up or down its list of length entries, the
  // selection following it, or remove it, leaving nothing selected; fields are the list's move
  // and remove fields. app.js keeps a refused edit as the refusal standing on the entry, which
  // this panel shows, so a rejection is only caught here.
  function listRow(owner, fields, length) {
    const remove = element("button", { type: "button" }, "Remove");
    remove.addEventListener("click", () =>
      window.tune.edit({ owner, field: fields.remove, value: null }).then(
        () => window.tune.select(null),
        () => {},
      ),
    );
    return element(
      "div",
      { className: "entry-actions" },
      moveButton("Move up", owner, fields.move, owner.index - 1, length),
      moveButton("Move down", owner, fields.move, owner.index + 1, length),
      remove,
    );
  }

  // moveButton moves owner's entry to destination in its list of length entries and selects it
  // there; it is disabled when destination is outside the list.
  function moveButton(label, owner, field, destination, length) {
    const isOutside = destination < 0 || destination >= length;
    const button = element("button", { type: "button", disabled: isOutside }, label);
    button.addEventListener("click", () =>
      window.tune.edit({ owner, field, value: destination }).then(
        () => window.tune.selectEntry({ kind: owner.kind, index: destination }),
        () => {},
      ),
    );
    return button;
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
