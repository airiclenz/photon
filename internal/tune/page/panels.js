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
    errorRows,
    needsRecord,
    flowTarget,
    TARGET_PLACEHOLDER,
  } = page;
  const VALUE_AUTO = "auto";
  const VALUE_TRUE = "true";
  const VALUE_FALSE = "false";
  const DEFAULT_FACTOR = "2";
  // READ_ONLY_FRAME_FIELDS are the frame's keys photon tune shows but does not edit, named as the
  // storyboard and /api/state both spell them.
  const READ_ONLY_FRAME_FIELDS = [
    "cols",
    "rows",
    "padding",
    "font_size",
    "line_height",
    "scale",
    "max_colors",
  ];

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

  // framePanel is the clip's frame: width and fps edit, the rest is read-only here.
  function framePanel() {
    const state = page.state();
    return [
      element("h2", {}, "Clip frame"),
      countField("width (px)", "frame.width", state.width),
      countField("fps", "frame.fps", state.fps),
      ...READ_ONLY_FRAME_FIELDS.map((name) =>
        fieldRow(name, element("span", { className: "read-only" }, String(state[name]))),
      ),
      element("p", { className: "read-only" }, "Length " + formatTime(state.length_ms)),
      ...state.warnings.map((warning) => element("p", { className: "error" }, warning)),
      ...errorRows(CLIP),
    ];
  }

  function cutField(beat) {
    const box = element("input", { type: "checkbox", checked: beat.cut });
    box.addEventListener("change", () => applyEdit(beat.id, "cut", box.checked ? VALUE_TRUE : VALUE_FALSE));
    return fieldRow("cut", box);
  }

  // zoomPanel edits the beat's zoom, or offers to add one by its target.
  function zoomPanel(beat) {
    const heading = element("h3", {}, "zoom");
    if (!beat.zoom) {
      const target = element("input", { type: "text", placeholder: TARGET_PLACEHOLDER });
      const add = element("button", { type: "button" }, "Add zoom");
      add.addEventListener("click", () => applyEdit(beat.id, "zoom", target.value.trim()));
      return [heading, fieldRow("target", target), add];
    }
    const zoom = beat.zoom;
    const target = element("input", { type: "text", value: flowTarget(zoom.target) });
    target.addEventListener("change", () => applyEdit(beat.id, "zoom.target", target.value.trim()));
    const remove = element("button", { type: "button" }, "Remove zoom");
    remove.addEventListener("click", () => applyEdit(beat.id, "zoom", ""));
    return [
      heading,
      fieldRow("target", target),
      factorField(beat.id, zoom.factor),
      millisecondsField("in", beat.id, "zoom.in", zoom.in_ms),
      millisecondsField("out", beat.id, "zoom.out", zoom.out_ms),
      remove,
    ];
  }

  // factorField edits the zoom factor; factor 0 is auto, fitted to the target.
  function factorField(beat, factor) {
    const isAuto = factor === 0;
    const input = element("input", {
      type: "number",
      step: "0.1",
      value: isAuto ? DEFAULT_FACTOR : String(factor),
      disabled: isAuto,
    });
    const auto = element("input", { type: "checkbox", checked: isAuto });
    input.addEventListener("change", () => applyEdit(beat, "zoom.factor", input.value.trim()));
    auto.addEventListener("change", () =>
      applyEdit(beat, "zoom.factor", auto.checked ? VALUE_AUTO : input.value.trim()),
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
