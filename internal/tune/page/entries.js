// photon tune's camera move and caption panels: what the inspector shows for a selected entry —
// its timing, where a camera move goes and the waypoints of a via pan's path, a caption's text,
// placement and style, when it is on the output clock and the errors standing on it — and the
// buttons that move it up or down its list or remove it.
//
// It loads after controls.js and adds entryPanel to window.tunePage, the page's internal
// namespace, where app.js's inspector draws it; every field is a control from controls.js. A
// waypoint's Set from pick reads the standing pick through the pickProposal and reportPick that
// picker.js, which loads later, adds there.
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
  const KIND_ZOOM = "zoom";
  const CAPTION_TEXT_ROWS = 3;
  // The step a caption's font size is typed in, in the unit of the frame's font size.
  const FONT_SIZE_STEP = "0.5";
  // The list fields that move an entry within its list, or remove it, by list.
  const CAMERA_LIST = Object.freeze({ move: "camera.move", remove: "camera.remove" });
  const CAPTION_LIST = Object.freeze({ move: "caption.move", remove: "caption.remove" });
  // The fields that set, insert and remove a waypoint of a via pan's path.
  const VIA_FIELDS = Object.freeze({
    set: "camera.via",
    insert: "camera.via.insert",
    remove: "camera.via.remove",
  });
  const PICK_FIRST = "Pick a target first: ⌖ Pick, then drag across the preview.";

  // entryPanel is a camera move's or caption's fields; what the page does not edit shows
  // read-only, and the YAML edits it.
  function entryPanel(kind, entry) {
    return kind === ENTRY_CAMERA ? cameraPanel(entry) : captionPanel(entry);
  }

  // cameraPanel edits a camera move's timing and where it goes, shows when it plays on the
  // output clock and the errors standing on it, and ends with its list buttons.
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
      listRow(owner, CAMERA_LIST, page.state().camera.length),
    ];
  }

  // aimRows edit where a camera move goes, each sending a typed camera.to: a zoom's factor, or
  // a pan's target or cell, which a pick replaces with a target. A pan's path select moves it
  // straight to its to or along a via path, and a via pan lists the waypoints of its path.
  function aimRows(owner, move) {
    if (move.kind === KIND_ZOOM) {
      const input = element("input", { type: "number", step: "0.1", value: String(move.factor) });
      input.addEventListener("change", () => applyEdit(owner, "camera.to", numberOf(input.value)));
      return [fieldRow("to (×)", input)];
    }
    // The pan's camera.path field defaults to the path it follows.
    const path = choiceField("path", owner, "camera.path", null);
    if (move.target) {
      const inputs = targetInputs(move.target);
      for (const input of Object.values(inputs)) {
        input.addEventListener("change", () => applyEdit(owner, "camera.to", targetOf(inputs)));
      }
      return [path, ...targetRows("to", inputs)];
    }
    if (move.cell) {
      return [path, cellRow(owner, move.cell)];
    }
    return [path, ...(move.via || []).flatMap((point, index) => waypointRows(owner, point, index))];
  }

  // cellRow edits the cell a pan goes to, its column x and row y from 0.
  function cellRow(owner, cell) {
    const x = cellInput(cell.x);
    const y = cellInput(cell.y);
    const send = () =>
      applyEdit(owner, "camera.to", { cell: { x: numberOf(x.value), y: numberOf(y.value) } });
    x.addEventListener("change", send);
    y.addEventListener("change", send);
    return fieldRow("to (x, y)", element("span", { className: "inline" }, x, y));
  }

  // cellInput is a cell's column or row input, holding value.
  function cellInput(value) {
    return element("input", { type: "number", min: "0", step: "1", value: String(value) });
  }

  // waypointRows edit the waypoint at index of a via pan's path, point as /api/state sends it, a
  // target or a cell. A target shows read-only and its cell inputs start empty; typing both of a
  // waypoint's cell inputs makes it that cell. Its buttons set it to the standing pick's target,
  // insert a copy of it after it, or remove it.
  function waypointRows(owner, point, index) {
    const label = "waypoint " + index;
    const rows = point.target ? [readOnlyRow(label, targetText(point.target))] : [];
    rows.push(waypointCellRow(owner, point, index, label), waypointActions(owner, point, index));
    return rows;
  }

  // waypointCellRow edits the waypoint at index as a cell, its column x and row y from 0, empty
  // while it is a target.
  function waypointCellRow(owner, point, index, label) {
    const cell = point.cell || { x: "", y: "" };
    const x = cellInput(cell.x);
    const y = cellInput(cell.y);
    const send = () => {
      if (x.value.trim() === "" || y.value.trim() === "") {
        return;
      }
      const value = { index, point: { cell: { x: numberOf(x.value), y: numberOf(y.value) } } };
      applyEdit(owner, VIA_FIELDS.set, value);
    };
    x.addEventListener("change", send);
    y.addEventListener("change", send);
    return fieldRow(label + " (x, y)", element("span", { className: "inline" }, x, y));
  }

  // waypointActions are the buttons of the waypoint at index: Set from pick sets it to the
  // standing pick's target, or, with none standing, says to pick one first; Insert after adds a
  // copy of it after it; Remove removes it. Each button's title names its waypoint.
  function waypointActions(owner, point, index) {
    const fromPick = waypointButton(
      "Set from pick",
      "Set waypoint " + index + " from the pick",
      () => {
        const target = page.pickProposal();
        if (target === null) {
          page.reportPick(PICK_FIRST, true);
          return;
        }
        applyEdit(owner, VIA_FIELDS.set, { index, point: target });
      },
    );
    const insert = waypointButton(
      "Insert after",
      "Insert a copy of waypoint " + index + " after it",
      () => applyEdit(owner, VIA_FIELDS.insert, { index: index + 1, point: wirePoint(point) }),
    );
    const remove = waypointButton("Remove", "Remove waypoint " + index, () =>
      applyEdit(owner, VIA_FIELDS.remove, { index }),
    );
    return element("div", { className: "waypoint-actions" }, fromPick, insert, remove);
  }

  function waypointButton(label, title, onClick) {
    const button = element("button", { type: "button", title }, label);
    button.addEventListener("click", onClick);
    return button;
  }

  // wirePoint is a waypoint as /api/state sends it, spelled as an edit's point takes it: its cell,
  // or its target as the target inputs type it.
  function wirePoint(point) {
    if (point.cell) {
      return { cell: { x: point.cell.x, y: point.cell.y } };
    }
    return targetOf(targetInputs(point.target));
  }

  // targetText describes a waypoint's target: its text, and its nth and area where it has them,
  // each spelled as the target inputs show it.
  function targetText(target) {
    const inputs = targetInputs(target);
    const parts = ["target " + inputs.text.value];
    if (inputs.nth.value !== "") {
      parts.push("nth " + inputs.nth.value);
    }
    if (inputs.area.value !== "") {
      parts.push("in " + inputs.area.value);
    }
    return parts.join(", ");
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
