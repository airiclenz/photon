// photon tune's inspector controls: the field rows the panels are built from, each sending its
// edit through app.js's applyEdit, the rows of errors and of a refused edit standing on what a
// panel shows, and the typed moment, target, colour, fraction and pixel inputs.
//
// It loads after app.js and kinds.js and adds its functions to window.tunePage, the page's internal namespace,
// where panels.js and entries.js build with them and app.js marks the beat select's errors.
"use strict";

(() => {
  const page = window.tunePage;
  const { CLIP, applyEdit, element, isEntry, entryValue } = page;
  const NEEDS_RECORD = "needs photon record";
  // A moment's beat 0 is a clip time, offset_ms after the clip's first frame.
  const CLIP_TIME_BEAT = 0;
  // A clearable moment field's select value for no moment, which sends null.
  const NO_MOMENT = "";
  // A target's nth: /api/state sends 0 for the first match and -1 for the last; an edit leaves
  // the first out and spells the last "last".
  const STATE_NTH_FIRST = 0;
  const STATE_NTH_LAST = -1;
  const NTH_LAST = "last";
  const TEXT_PLACEHOLDER = "a regex, such as ok";
  const NTH_PLACEHOLDER = "blank for the first; 2, 3 … or last";
  const AREA_PLACEHOLDER = "the whole screen";
  // A fraction slider's bounds and the step it moves in.
  const FRACTION_MIN = "0";
  const FRACTION_MAX = "1";
  const FRACTION_STEP = "0.05";
  // The kinds of a problem's owner besides a zoom's or a caption's, as /api/state sends them.
  const OWNER_CLIP = "clip";
  const OWNER_BEAT = "beat";

  function fieldRow(label, control) {
    return element("label", { className: "field" }, element("span", {}, label), control);
  }

  function readOnlyRow(label, text) {
    return fieldRow(label, element("span", { className: "read-only" }, text));
  }

  // millisecondsField edits field as a whole number of milliseconds.
  function millisecondsField(label, owner, field, ms) {
    const input = element("input", { type: "number", min: "0", step: "1", value: String(ms) });
    input.addEventListener("change", () => applyEdit(owner, field, numberOf(input.value)));
    return fieldRow(label + " (ms)", input);
  }

  // momentField edits field as a typed moment, {beat, offset_ms}: a select of the clip time or a
  // beat, and the milliseconds after it. A clearable field, whose moment may be null, offers
  // none as well, which sends null.
  function momentField(label, owner, field, moment, isClearable = false) {
    const beats = page.state().beats.map((beat) => page.option(String(beat.id), "beat " + beat.id));
    const none = isClearable ? [page.option(NO_MOMENT, "none")] : [];
    const clipTime = page.option(String(CLIP_TIME_BEAT), "clip time");
    const beat = element("select", {}, ...none, clipTime, ...beats);
    beat.value = moment ? String(moment.beat) : NO_MOMENT;
    const offsetMS = moment ? moment.offset_ms : 0;
    const offset = element("input", { type: "number", step: "1", value: String(offsetMS) });
    const send = () => {
      if (beat.value === NO_MOMENT) {
        applyEdit(owner, field, null);
        return;
      }
      applyEdit(owner, field, { beat: Number(beat.value), offset_ms: numberOf(offset.value) });
    };
    beat.addEventListener("change", send);
    offset.addEventListener("change", send);
    const after = element("span", {}, "+");
    return fieldRow(label + " (ms)", element("span", { className: "inline" }, beat, after, offset));
  }

  // choiceField edits owner's enum field name as one of the choices /api/state describes for it,
  // chosen now, or the field's default while chosen is empty.
  function choiceField(label, owner, name, chosen) {
    const field = fieldsOf(owner).find((candidate) => candidate.name === name);
    const input = element("select", {}, ...field.choices.map((choice) => page.option(choice, choice)));
    input.value = chosen || field.default;
    input.addEventListener("change", () => applyEdit(owner, field.name, input.value));
    return fieldRow(label, input);
  }

  // decimalField edits owner's field as a number, typed in steps of step.
  function decimalField(label, owner, field, number, step) {
    const input = element("input", { type: "number", min: "0", step, value: String(number) });
    input.addEventListener("change", () => applyEdit(owner, field, numberOf(input.value)));
    return fieldRow(label, input);
  }

  // colorField edits owner's field as a #rrggbb colour chosen in the browser's colour picker.
  function colorField(label, owner, field, color) {
    const input = element("input", { type: "color", value: color });
    input.addEventListener("change", () => applyEdit(owner, field, input.value));
    return fieldRow(label, input);
  }

  // fractionField edits owner's field as a fraction from 0 to 1 on a slider, whose value shows
  // beside it as it moves; the edit is sent once the slider is let go.
  function fractionField(label, owner, field, fraction) {
    const input = element("input", {
      type: "range",
      min: FRACTION_MIN,
      max: FRACTION_MAX,
      step: FRACTION_STEP,
      value: String(fraction),
    });
    const shown = element("span", { className: "read-only" }, String(fraction));
    input.addEventListener("input", () => {
      shown.textContent = input.value;
    });
    input.addEventListener("change", () => applyEdit(owner, field, numberOf(input.value)));
    return fieldRow(label, element("span", { className: "inline" }, input, shown));
  }

  // pixelsField edits owner's field as whole design pixels, {x, y}: x to the right and y down.
  function pixelsField(label, owner, field, pixels) {
    const input = (value) => element("input", { type: "number", step: "1", value: String(value) });
    const x = input(pixels.x);
    const y = input(pixels.y);
    const send = () => applyEdit(owner, field, { x: numberOf(x.value), y: numberOf(y.value) });
    x.addEventListener("change", send);
    y.addEventListener("change", send);
    return fieldRow(label + " (x, y)", element("span", { className: "inline" }, x, y));
  }

  // countField edits the clip's field as a whole number no less than min.
  function countField(label, field, count, min) {
    const input = element("input", { type: "number", min, step: "1", value: String(count) });
    input.addEventListener("change", () => applyEdit(CLIP, field, numberOf(input.value)));
    return fieldRow(label, input);
  }

  // numberOf is text typed into a number input as the number an edit takes, or the text itself
  // when it holds no number, so the server's refusal names what was typed.
  function numberOf(text) {
    const trimmed = text.trim();
    const number = Number(trimmed);
    return trimmed !== "" && Number.isFinite(number) ? number : trimmed;
  }

  // errorRows are the errors standing on owner — a beat id, 0 for the clip, or a zoom's or
  // caption's {kind, index} entry — and a refused edit of it.
  function errorRows(owner) {
    const messages = problemsOn(owner).map((problem) => problem.message);
    const key = isEntry(owner) ? entryValue(owner.kind, owner.index) : String(owner);
    const refusal = page.refusal();
    if (refusal && refusal.owner === key) {
      messages.push(refusal.message);
    }
    return messages.map((message) => element("p", { className: "error" }, message));
  }

  // errorMark marks a beat select option whose owner has an error standing on it.
  function errorMark(owner) {
    return problemsOn(owner).length > 0 ? "  (error)" : "";
  }

  // problemsOn are the problems standing on owner, matched by the owner each problem carries.
  function problemsOn(owner) {
    const wanted = problemOwner(owner);
    return page
      .state()
      .errors.filter((problem) => problem.owner.kind === wanted.kind && problem.owner.index === wanted.index);
  }

  // fieldsOf are the fields of owner as /api/state describes them, matched by the owner each
  // field carries.
  function fieldsOf(owner) {
    const wanted = problemOwner(owner);
    return page
      .state()
      .fields.filter((field) => field.owner.kind === wanted.kind && field.owner.index === wanted.index);
  }

  // problemOwner spells owner as a problem's or an edit's {kind, index}: the clip, a beat by its
  // id, or the zoom's or caption's entry, whose kind a problem's owner spells the same.
  function problemOwner(owner) {
    if (isEntry(owner)) {
      return owner;
    }
    return owner === CLIP ? { kind: OWNER_CLIP, index: 0 } : { kind: OWNER_BEAT, index: owner };
  }

  function needsRecord() {
    return element("span", { className: "needs-record" }, NEEDS_RECORD);
  }

  // targetInputs are a target's text, nth and area inputs, holding target when there is one.
  function targetInputs(target) {
    const input = (value, placeholder) => element("input", { type: "text", value, placeholder });
    return {
      text: input(target ? target.text : "", TEXT_PLACEHOLDER),
      nth: input(target ? nthText(target.nth) : "", NTH_PLACEHOLDER),
      area: input(target ? target.area : "", AREA_PLACEHOLDER),
    };
  }

  // targetRows are the rows of a target's inputs.
  function targetRows(label, inputs) {
    return [fieldRow(label, inputs.text), fieldRow("nth", inputs.nth), fieldRow("area", inputs.area)];
  }

  // nthText spells a state target's nth for its input: blank for the first match, "last" for the
  // last, else its number.
  function nthText(nth) {
    if (nth === STATE_NTH_FIRST) {
      return "";
    }
    return nth === STATE_NTH_LAST ? NTH_LAST : String(nth);
  }

  // targetOf is the typed target the inputs hold, as an edit takes it: nth left out for the
  // first match and area for the whole screen.
  function targetOf(inputs) {
    const target = { text: inputs.text.value };
    const nth = inputs.nth.value.trim();
    if (nth !== "") {
      target.nth = nth === NTH_LAST ? NTH_LAST : numberOf(nth);
    }
    const area = inputs.area.value.trim();
    if (area !== "") {
      target.area = area;
    }
    return target;
  }

  Object.assign(page, {
    fieldRow,
    readOnlyRow,
    millisecondsField,
    momentField,
    choiceField,
    decimalField,
    colorField,
    fractionField,
    pixelsField,
    countField,
    fieldsOf,
    errorRows,
    errorMark,
    problemOwner,
    numberOf,
    needsRecord,
    targetInputs,
    targetRows,
    targetOf,
  });
})();
