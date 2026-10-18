// photon tune's inspector controls: the field rows the panels are built from, each sending its
// edit through app.js's applyEdit, the rows of errors and of a refused edit standing on what a
// panel shows, and the spellings of moments and targets as the storyboard writes them.
//
// It loads after app.js and adds its functions to window.tunePage, the page's internal namespace,
// where panels.js and entries.js build with them and app.js marks the beat select's errors.
"use strict";

(() => {
  const page = window.tunePage;
  const { CLIP, applyEdit, element, isEntry, entryValue } = page;
  const NEEDS_RECORD = "needs photon record";
  const MOMENT_PLACEHOLDER = "{beat: 3, offset: 200ms} or 4500ms";
  const TARGET_PLACEHOLDER = "{text: 'ok', area: list}";
  // The kinds of a problem's owner besides a camera move's and a caption's, as /api/state sends them.
  const OWNER_CLIP = "clip";
  const OWNER_BEAT = "beat";

  function fieldRow(label, control) {
    return element("label", { className: "field" }, element("span", {}, label), control);
  }

  function readOnlyRow(label, text) {
    return fieldRow(label, element("span", { className: "read-only" }, text));
  }

  // millisecondsField edits field as whole milliseconds: a number for a beat's field, the text
  // typed for a camera move's.
  function millisecondsField(label, owner, field, ms) {
    const input = element("input", { type: "number", min: "0", step: "1", value: String(ms) });
    input.addEventListener("change", () =>
      applyEdit(owner, field, isEntry(owner) ? input.value.trim() : numberOf(input.value)),
    );
    return fieldRow(label + " (ms)", input);
  }

  // textField edits field as the one line of text typed, a moment or a target as YAML flow text.
  function textField(label, owner, field, text, placeholder) {
    const input = element("input", { type: "text", value: text, placeholder });
    input.addEventListener("change", () => applyEdit(owner, field, input.value.trim()));
    return fieldRow(label, input);
  }

  // choiceField edits field as one of choices, chosen now.
  function choiceField(label, owner, field, choices, chosen) {
    const input = element("select", {}, ...choices.map((choice) => page.option(choice, choice)));
    input.value = chosen;
    input.addEventListener("change", () => applyEdit(owner, field, input.value));
    return fieldRow(label, input);
  }

  function countField(label, field, count) {
    const input = element("input", { type: "number", min: "1", step: "1", value: String(count) });
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

  // errorRows are the errors standing on owner — a beat id, 0 for the clip, or a camera move's
  // or caption's {kind, index} entry — and a refused edit of it.
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

  // problemOwner spells owner as a problem's or an edit's {kind, index}: the clip, a beat by its
  // id, or the camera move's or caption's entry, whose kinds a problem's owner spells the same.
  function problemOwner(owner) {
    if (isEntry(owner)) {
      return owner;
    }
    return owner === CLIP ? { kind: OWNER_CLIP, index: 0 } : { kind: OWNER_BEAT, index: owner };
  }

  function needsRecord() {
    return element("span", { className: "needs-record" }, NEEDS_RECORD);
  }

  // momentText spells a moment as the storyboard does: {beat: N, offset: D} or a bare clip time.
  function momentText(moment) {
    if (moment.beat === 0) {
      return moment.offset_ms + "ms";
    }
    const offset = moment.offset_ms === 0 ? "" : ", offset: " + moment.offset_ms + "ms";
    return "{beat: " + moment.beat + offset + "}";
  }

  // flowTarget spells a target as the YAML flow text a zoom.target edit takes.
  function flowTarget(target) {
    const parts = ["text: " + quoted(target.text)];
    if (target.area) {
      parts.push("area: " + quoted(target.area));
    }
    if (target.nth !== 0) {
      parts.push("nth: " + target.nth);
    }
    return "{" + parts.join(", ") + "}";
  }

  // quoted is text as a single-quoted YAML scalar.
  function quoted(text) {
    return "'" + text.replaceAll("'", "''") + "'";
  }

  Object.assign(page, {
    MOMENT_PLACEHOLDER,
    TARGET_PLACEHOLDER,
    fieldRow,
    readOnlyRow,
    millisecondsField,
    textField,
    choiceField,
    countField,
    errorRows,
    errorMark,
    problemOwner,
    numberOf,
    needsRecord,
    momentText,
    flowTarget,
  });
})();
