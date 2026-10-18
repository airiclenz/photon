// photon tune's beat and frame panels: what the inspector shows for a selected beat — its
// timing, cut, zoom, pauses, report row and errors — and for the clip's frame.
//
// It loads after controls.js and adds its panels to window.tunePage, the page's internal
// namespace, where app.js's inspector draws them; every field is a control from controls.js.
"use strict";

(() => {
  const page = window.tunePage;
  const {
    CLIP,
    applyEdit,
    element,
    formatTime,
    fieldRow,
    millisecondsField,
    countField,
    decimalField,
    fieldsOf,
    errorRows,
    needsRecord,
    numberOf,
    targetInputs,
    targetRows,
    targetOf,
  } = page;
  const AUTO_FACTOR = 0;
  const DEFAULT_FACTOR = "2";
  // The steps the frame's font size and line height are typed in, and the least whole number a
  // frame count takes: padding may be 0, every other at least 1.
  const FONT_SIZE_STEP = "0.5";
  const LINE_HEIGHT_STEP = "0.05";
  const MIN_ZERO = "0";
  const MIN_ONE = "1";

  // beatPanel is the selected beat's fields, its zoom, its read-only pauses, its report row and
  // the errors standing on it.
  function beatPanel(beat) {
    return [
      element("h2", {}, "beat " + beat.id + " " + beat.title),
      millisecondsField("duration", beat.id, "duration", beat.duration_ms),
      millisecondsField("hold", beat.id, "hold", beat.hold_ms),
      cutField(beat),
      ...zoomPanel(beat),
      ...pausesPanel(beat),
      ...reportRows(beat.id),
      ...errorRows(beat.id),
    ];
  }

  // framePanel is the clip's frame: width, fps, font size, line height, padding and scale edit,
  // and the fields /api/state describes as read-only show under their key, which the state sends
  // their value under.
  function framePanel() {
    const state = page.state();
    const readOnly = fieldsOf(CLIP).filter((field) => field.read_only);
    return [
      element("h2", {}, "Clip frame"),
      countField("width (px)", "frame.width", state.width, MIN_ONE),
      countField("fps", "frame.fps", state.fps, MIN_ONE),
      decimalField("font size", CLIP, "frame.font_size", state.font_size, FONT_SIZE_STEP),
      decimalField("line height", CLIP, "frame.line_height", state.line_height, LINE_HEIGHT_STEP),
      countField("padding (px)", "frame.padding", state.padding, MIN_ZERO),
      countField("scale", "frame.scale", state.scale, MIN_ONE),
      ...readOnly.map((field) =>
        fieldRow(field.name, element("span", { className: "read-only" }, String(state[field.name]))),
      ),
      element("p", { className: "read-only" }, "Length " + formatTime(state.length_ms)),
      ...state.warnings.map((warning) => element("p", { className: "error" }, warning)),
      ...errorRows(CLIP),
    ];
  }

  function cutField(beat) {
    const box = element("input", { type: "checkbox", checked: beat.cut });
    box.addEventListener("change", () => applyEdit(beat.id, "cut", box.checked));
    return fieldRow("cut", box);
  }

  // zoomPanel edits the beat's zoom, or offers to add one by its target.
  function zoomPanel(beat) {
    const heading = element("h3", {}, "zoom");
    const zoom = beat.zoom;
    const inputs = targetInputs(zoom ? zoom.target : null);
    const rows = targetRows("target", inputs);
    if (!zoom) {
      const add = element("button", { type: "button" }, "Add zoom");
      add.addEventListener("click", () => applyEdit(beat.id, "zoom", targetOf(inputs)));
      return [heading, ...rows, add];
    }
    for (const input of Object.values(inputs)) {
      input.addEventListener("change", () => applyEdit(beat.id, "zoom.target", targetOf(inputs)));
    }
    const remove = element("button", { type: "button" }, "Remove zoom");
    remove.addEventListener("click", () => applyEdit(beat.id, "zoom", null));
    return [
      heading,
      ...rows,
      factorField(beat.id, zoom.factor),
      millisecondsField("in", beat.id, "zoom.in", zoom.in_ms),
      millisecondsField("out", beat.id, "zoom.out", zoom.out_ms),
      remove,
    ];
  }

  // factorField edits the zoom factor; factor 0 is auto, fitted to the target.
  function factorField(beat, factor) {
    const isAuto = factor === AUTO_FACTOR;
    const input = element("input", {
      type: "number",
      step: "0.1",
      value: isAuto ? DEFAULT_FACTOR : String(factor),
      disabled: isAuto,
    });
    const auto = element("input", { type: "checkbox", checked: isAuto });
    input.addEventListener("change", () => applyEdit(beat, "zoom.factor", numberOf(input.value)));
    auto.addEventListener("change", () =>
      applyEdit(beat, "zoom.factor", auto.checked ? AUTO_FACTOR : numberOf(input.value)),
    );
    const autoLabel = element("label", { className: "inline" }, auto, "auto");
    return fieldRow("factor", element("span", { className: "inline" }, input, autoLabel));
  }

  // pausesPanel shows the beat's pauses: the take holds them, so they change only by recording.
  function pausesPanel(beat) {
    if (beat.pauses_ms.length === 0) {
      return [];
    }
    return [
      element("h3", {}, "do"),
      ...beat.pauses_ms.map((ms) =>
        fieldRow("pause", element("span", { className: "read-only" }, ms + "ms ", needsRecord())),
      ),
    ];
  }

  // reportRows is render's summary row for the beat, red when the schedule warns of its speed.
  function reportRows(beat) {
    const state = page.state();
    const section = state.sections.find((candidate) => candidate.beat === beat);
    if (!section) {
      return [];
    }
    const isTooFast = state.warnings.some((warning) => warning.startsWith("beat " + beat + " "));
    return [element("p", { className: isTooFast ? "report too-fast" : "report" }, section.report)];
  }

  Object.assign(page, { beatPanel, framePanel });
})();
