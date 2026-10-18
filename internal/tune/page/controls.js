// photon tune's inspector controls: the field rows the panels are built from, each sending its
// edit through app.js's applyEdit, the rows of errors and of a refused edit standing on what a
// panel shows, and the spellings of moments and targets as the storyboard writes them.
//
// It loads after app.js and adds its functions to window.tunePage, the page's internal namespace,
// where panels.js and entries.js build with them and app.js marks the beat select's errors.
"use strict";

(() => {
  const page = window.tunePage;
  const { CLIP, ENTRY_CAMERA, ENTRY_CAPTION, applyEdit, element, isEntry, entryValue } = page;
  const NEEDS_RECORD = "needs photon record";
  const MOMENT_PLACEHOLDER = "{beat: 3, offset: 200ms} or 4500ms";
  const TARGET_PLACEHOLDER = "{text: 'ok', area: list}";
  // ENTRY_ERROR_PREFIXES start the messages of the errors standing on a camera move or caption,
  // followed by its index and "]", as the storyboard's validation and a refused edit spell them.
  const ENTRY_ERROR_PREFIXES = { [ENTRY_CAMERA]: "camera[", [ENTRY_CAPTION]: "captions[" };

  function fieldRow(label, control) {
    return element("label", { className: "field" }, element("span", {}, label), control);
  }

  function readOnlyRow(label, text) {
    return fieldRow(label, element("span", { className: "read-only" }, text));
  }

  function millisecondsField(label, owner, field, ms) {
    const input = element("input", { type: "number", min: "0", step: "1", value: String(ms) });
    input.addEventListener("change", () => applyEdit(owner, field, input.value.trim()));
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
    input.addEventListener("change", () => applyEdit(CLIP, field, input.value.trim()));
    return fieldRow(label, input);
  }

  // errorRows are the errors standing on owner — a beat id, 0 for the clip, or a camera move's
  // or caption's {kind, index} entry, whose errors stand on the clip and name it — and a refused
  // edit of it.
  function errorRows(owner) {
    const isOwnedBy = isEntry(owner)
      ? (problem) => problem.beat === CLIP && problem.message.startsWith(entryErrorPrefix(owner))
      : (problem) => problem.beat === owner;
    const messages = page.state().errors.filter(isOwnedBy).map((problem) => problem.message);
    const key = isEntry(owner) ? entryValue(owner.kind, owner.index) : String(owner);
    const refusal = page.refusal();
    if (refusal && refusal.owner === key) {
      messages.push(refusal.message);
    }
    return messages.map((message) => element("p", { className: "error" }, message));
  }

  // entryErrorPrefix starts every error message standing on entry: camera[2] or captions[0].
  function entryErrorPrefix(entry) {
    return ENTRY_ERROR_PREFIXES[entry.kind] + entry.index + "]";
  }

  function errorMark(beat) {
    const hasError = page.state().errors.some((problem) => problem.beat === beat);
    return hasError ? "  (error)" : "";
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
    needsRecord,
    momentText,
    flowTarget,
  });
})();
