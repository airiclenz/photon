// photon tune's position, transition, zoom and caption panels: what the inspector shows for a
// selected entry — a camera position's timing and where it aims, a transition's edges and ease, a
// camera zoom's timing, ease and factor, or a caption's text, timing, placement and style, when it
// is on the output clock and the errors standing on it — and the buttons that move a caption up or
// down its list, or remove the entry.
//
// It loads after controls.js and adds entryPanel to window.tunePage, the page's internal
// namespace, where app.js's inspector draws it; every field is a control from controls.js.
"use strict";

(() => {
  const page = window.tunePage;
  const {
    ENTRY_POSITION,
    ENTRY_TRANSITION,
    ENTRY_ZOOM,
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
    targetInputs,
    targetRows,
    targetOf,
  } = page;
  const CAPTION_TEXT_ROWS = 3;
  // The step a caption's font size is typed in, in the unit of the frame's font size.
  const FONT_SIZE_STEP = "0.5";
  // The step a zoom's factor is typed in.
  const ZOOM_FACTOR_STEP = "0.1";
  // The list fields that move an entry within its list, or remove it, by list: a zoom is never
  // moved, since render places the zooms by their starts.
  const CAPTION_LIST = Object.freeze({ move: "caption.move", remove: "caption.remove" });
  const ZOOM_REMOVE = "zoom.remove";
  const POSITION_REMOVE = "position.remove";
  const POSITION_TRANSITION = "position.transition";

  // The panel of each entry kind the inspector shows.
  const PANELS = Object.freeze({
    [ENTRY_POSITION]: positionPanel,
    [ENTRY_TRANSITION]: transitionPanel,
    [ENTRY_ZOOM]: zoomPanel,
    [ENTRY_CAPTION]: captionPanel,
  });

  // entryPanel is the fields of an entry of kind; what the page does not edit shows read-only,
  // and the YAML edits it.
  function entryPanel(kind, entry) {
    return PANELS[kind](entry);
  }

  // positionPanel edits a camera position's start, its hold and where it aims — its cell, or its
  // text target's text, nth and area — and shows when it holds on the output clock and the errors
  // standing on it. The panel ends with its remove button; a position is never moved, since
  // render places the positions by their starts.
  function positionPanel(position) {
    const owner = { kind: ENTRY_POSITION, index: position.index };
    return [
      element("h2", {}, "position " + position.index),
      momentField("at", owner, "position.at", position.at),
      millisecondsField("hold", owner, "position.hold", position.hold_ms),
      ...aimRows(owner, position),
      readOnlyRow("holds", formatTime(position.start_ms) + " – " + formatTime(position.end_ms)),
      ...errorRows(owner),
      element("div", { className: "entry-actions" }, removeButton(owner, POSITION_REMOVE)),
    ];
  }

  // aimRows edit where a position aims: its cell, or its text target, whose text, nth and area
  // send the whole target when any of them changes.
  function aimRows(owner, position) {
    if (position.cell) {
      return [pixelsField("cell", owner, "position.cell", position.cell)];
    }
    const inputs = targetInputs(position.target);
    const send = () => applyEdit(owner, "position.target", targetOf(inputs));
    for (const input of [inputs.text, inputs.nth, inputs.area]) {
      input.addEventListener("change", send);
    }
    return targetRows("text", inputs);
  }

  // transitionPanel edits the camera transition into a position: its start and its length, each
  // sent as the transition's two edges, and its ease, the ease of the position it leads into. It
  // shows the errors standing on that position, a refused edge edit among them, since every field
  // here is that position's.
  function transitionPanel(transition) {
    const owner = { kind: ENTRY_POSITION, index: transition.index };
    const length = transition.end_ms - transition.start_ms;
    return [
      element("h2", {}, "transition into position " + transition.index),
      readOnlyRow("from", "position " + transition.from),
      edgesField("start", owner, transition.start_ms, (start) => edges(start, shifted(start, length))),
      edgesField("length", owner, length, (span) => edges(transition.start_ms, shifted(span, transition.start_ms))),
      choiceField("ease", owner, "position.ease", transition.ease),
      readOnlyRow("plays", formatTime(transition.start_ms) + " – " + formatTime(transition.end_ms)),
      ...errorRows(owner),
    ];
  }

  // edgesField edits a transition through one number of milliseconds, ms now: a change sends the
  // edges toEdges makes of the number typed.
  function edgesField(label, owner, ms, toEdges) {
    const input = element("input", { type: "number", min: "0", step: "1", value: String(ms) });
    input.addEventListener("change", () => applyEdit(owner, POSITION_TRANSITION, toEdges(numberOf(input.value))));
    return fieldRow(label + " (ms)", input);
  }

  // edges are a transition's edges as position.transition takes them.
  function edges(start, end) {
    return { start_ms: start, end_ms: end };
  }

  // shifted is ms moved by milliseconds; text typed in place of a number is passed on as it is,
  // so the server's refusal names it.
  function shifted(ms, milliseconds) {
    return typeof ms === "number" ? ms + milliseconds : ms;
  }

  // zoomPanel edits a camera zoom's start, its length, its ease and the factor it reaches, and
  // shows when it plays on the output clock and the errors standing on it. The panel ends with
  // its remove button.
  function zoomPanel(zoom) {
    const owner = { kind: ENTRY_ZOOM, index: zoom.index };
    return [
      element("h2", {}, "zoom " + zoom.index),
      momentField("at", owner, "zoom.at", zoom.at),
      millisecondsField("over", owner, "zoom.over", zoom.over_ms),
      choiceField("ease", owner, "zoom.ease", zoom.ease),
      decimalField("to (×)", owner, "zoom.to", zoom.factor, ZOOM_FACTOR_STEP),
      readOnlyRow("plays", formatTime(zoom.start_ms) + " – " + formatTime(zoom.end_ms)),
      ...errorRows(owner),
      element("div", { className: "entry-actions" }, removeButton(owner, ZOOM_REMOVE)),
    ];
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
  // selection following it, or remove it; fields are the list's move and remove fields.
  function listRow(owner, fields, length) {
    return element(
      "div",
      { className: "entry-actions" },
      moveButton("Move up", owner, fields.move, owner.index - 1, length),
      moveButton("Move down", owner, fields.move, owner.index + 1, length),
      removeButton(owner, fields.remove),
    );
  }

  // removeButton removes owner's entry through the remove field, leaving nothing selected. app.js
  // keeps a refused edit as the refusal standing on the entry, which its panel shows, so a
  // rejection is only caught here.
  function removeButton(owner, field) {
    const remove = element("button", { type: "button" }, "Remove");
    remove.addEventListener("click", () =>
      window.tune.edit({ owner, field, value: null }).then(
        () => window.tune.select(null),
        () => {},
      ),
    );
    return remove;
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
