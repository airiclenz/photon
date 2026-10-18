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
    numberOf,
  } = page;
  const AUTO_FACTOR = 0;
  const DEFAULT_FACTOR = "2";
  // A target's nth: /api/state sends 0 for the first match and -1 for the last; an edit leaves
  // the first out and spells the last "last".
  const STATE_NTH_FIRST = 0;
  const STATE_NTH_LAST = -1;
  const NTH_LAST = "last";
  const TEXT_PLACEHOLDER = "a regex, such as ok";
  const NTH_PLACEHOLDER = "blank for the first; 2, 3 … or last";
  const AREA_PLACEHOLDER = "the whole screen";
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
    box.addEventListener("change", () => applyEdit(beat.id, "cut", box.checked));
    return fieldRow("cut", box);
  }

  // zoomPanel edits the beat's zoom, or offers to add one by its target.
  function zoomPanel(beat) {
    const heading = element("h3", {}, "zoom");
    const zoom = beat.zoom;
    const inputs = targetInputs(zoom ? zoom.target : null);
    const targetRows = [fieldRow("target", inputs.text), fieldRow("nth", inputs.nth), fieldRow("area", inputs.area)];
    if (!zoom) {
      const add = element("button", { type: "button" }, "Add zoom");
      add.addEventListener("click", () => applyEdit(beat.id, "zoom", targetOf(inputs)));
      return [heading, ...targetRows, add];
    }
    for (const input of Object.values(inputs)) {
      input.addEventListener("change", () => applyEdit(beat.id, "zoom.target", targetOf(inputs)));
    }
    const remove = element("button", { type: "button" }, "Remove zoom");
    remove.addEventListener("click", () => applyEdit(beat.id, "zoom", null));
    return [
      heading,
      ...targetRows,
      factorField(beat.id, zoom.factor),
      millisecondsField("in", beat.id, "zoom.in", zoom.in_ms),
      millisecondsField("out", beat.id, "zoom.out", zoom.out_ms),
      remove,
    ];
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
