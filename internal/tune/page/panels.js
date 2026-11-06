// photon tune's beat and frame panels: what the inspector shows for a selected beat — its
// timing, cut, pauses, report row and errors — and for the clip's frame.
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
    errorRows,
    needsRecord,
    numberOf,
  } = page;
  // The steps the frame's font size, line height and device scale are typed in, and the least
  // whole number a frame count takes: padding may be 0, max_colors at least 2, every other at
  // least 1.
  const FONT_SIZE_STEP = "0.5";
  const LINE_HEIGHT_STEP = "0.05";
  const DEVICE_SCALE_STEP = "0.25";
  const MIN_ZERO = "0";
  const MIN_ONE = "1";
  const MIN_COLORS = "2";

  // beatPanel is the selected beat's fields, its read-only pauses, its report row and the errors
  // standing on it.
  function beatPanel(beat) {
    return [
      element("h2", {}, "beat " + beat.id + " " + beat.title),
      millisecondsField("duration", beat.id, "duration", beat.duration_ms),
      millisecondsField("hold", beat.id, "hold", beat.hold_ms),
      cutField(beat),
      ...pausesPanel(beat),
      ...reportRows(beat.id),
      ...errorRows(beat.id),
    ];
  }

  // framePanel is the clip's frame: width, fps, the fields of its kind, a terminal's or a page's,
  // and max_colors edit. A size other than the take's — a terminal's cols or rows, a page's
  // viewport or device scale — previews at the take's size and stands as a clip problem until
  // the take is re-recorded.
  function framePanel() {
    const state = page.state();
    return [
      element("h2", {}, "Clip frame"),
      countField("width (px)", "frame.width", state.width, MIN_ONE),
      countField("fps", "frame.fps", state.fps, MIN_ONE),
      ...(state.viewport ? pageFrameFields(state) : terminalFrameFields(state)),
      countField("max_colors", "frame.max_colors", state.max_colors, MIN_COLORS),
      element("p", { className: "read-only" }, "Length " + formatTime(state.length_ms)),
      ...state.warnings.map((warning) => element("p", { className: "error" }, warning)),
      ...errorRows(CLIP),
    ];
  }

  // terminalFrameFields are a terminal frame's own fields: font size, line height, padding,
  // scale, cols and rows.
  function terminalFrameFields(state) {
    return [
      decimalField("font size", CLIP, "frame.font_size", state.font_size, FONT_SIZE_STEP),
      decimalField("line height", CLIP, "frame.line_height", state.line_height, LINE_HEIGHT_STEP),
      countField("padding (px)", "frame.padding", state.padding, MIN_ZERO),
      countField("scale", "frame.scale", state.scale, MIN_ONE),
      countField("cols", "frame.cols", state.cols, MIN_ONE),
      countField("rows", "frame.rows", state.rows, MIN_ONE),
    ];
  }

  // pageFrameFields are a page frame's own fields: its viewport, device scale and scale. A page
  // takes none of a terminal's font size, line height, padding, cols or rows.
  function pageFrameFields(state) {
    return [
      viewportField(state.viewport),
      decimalField("device scale", CLIP, "frame.device_scale", state.device_scale, DEVICE_SCALE_STEP),
      countField("scale", "frame.scale", state.scale, MIN_ONE),
    ];
  }

  // viewportField edits frame.viewport as the page's width and height in CSS pixels.
  function viewportField(viewport) {
    const input = (value) =>
      element("input", { type: "number", min: MIN_ONE, step: "1", value: String(value) });
    const width = input(viewport.width);
    const height = input(viewport.height);
    const send = () => {
      const size = { width: numberOf(width.value), height: numberOf(height.value) };
      applyEdit(CLIP, "frame.viewport", size);
    };
    width.addEventListener("change", send);
    height.addEventListener("change", send);
    return fieldRow("viewport (w, h)", element("span", { className: "inline" }, width, height));
  }

  function cutField(beat) {
    const box = element("input", { type: "checkbox", checked: beat.cut });
    box.addEventListener("change", () => applyEdit(beat.id, "cut", box.checked));
    return fieldRow("cut", box);
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
