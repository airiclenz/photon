// photon tune's page: the preview, the inspector and the transport bar over the session's API.
// It holds no timing logic of its own: every value it shows comes from /api/state, every change
// is an edit the server applies and validates, and every frame is one the server composes.
//
// window.tune is the contract the timeline lanes and the target picker build on:
//   state()      the last /api/state JSON, or null before the first arrives
//   selected()   the id of the beat the inspector shows, or null for the clip's frame
//   select(id)   show beat id in the inspector, or the clip's frame for null
//   time()       the playhead, in output milliseconds
//   frameTime()  the output milliseconds of the frame the preview <img> shows, or null
//   seek(ms)     move the playhead to ms, clamped to the clip
//   pause()      stop playing
//   edit(edit)   apply {beat, field, value}; a Promise of the new state, rejected when refused
//   onState(fn)  call fn(state) with every new state
//   frame        the preview <img> element
"use strict";

(() => {
  const API = {
    state: "/api/state",
    frame: "/api/frame?t=",
    edit: "/api/edit",
    undo: "/api/undo",
    redo: "/api/redo",
    revert: "/api/revert",
    reload: "/api/reload",
    save: "/api/save",
  };
  const POLL_INTERVAL_MS = 1000;
  const MILLISECONDS_PER_SECOND = 1000;
  const MILLISECONDS_PER_MINUTE = 60000;
  const SECOND_STEP_MS = 1000;
  const CLIP = 0;
  const NEEDS_RECORD = "needs photon record";
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
  const PLAY_GLYPH = "▶";
  const PAUSE_GLYPH = "⏸";

  const frameImage = byId("frame");
  const view = {
    banner: byId("banner"),
    bannerText: byId("banner-text"),
    reload: byId("reload"),
    frameMessage: byId("frame-message"),
    beatSelect: byId("beat-select"),
    inspector: byId("inspector-body"),
    previous: byId("previous"),
    play: byId("play"),
    next: byId("next"),
    time: byId("time"),
    status: byId("status"),
    undo: byId("undo"),
    redo: byId("redo"),
    revert: byId("revert"),
    save: byId("save"),
  };

  const listeners = [];
  let current = null;
  let currentKey = "";
  let selectedBeat = null;
  let playhead = 0;
  let playing = null;
  let shownFrameMs = null;
  let shownFrameKey = "";
  let isFrameLoading = false;
  let shownFrameURL = "";
  let refusal = null;
  let isServerAway = false;

  // --- the session's API ---

  // request sends method to path, a JSON body for a POST, and resolves with the JSON answer;
  // it rejects with the server's own error message when the answer is a failure.
  async function request(path, body) {
    const options = body === undefined ? {} : {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    };
    const response = await fetch(path, options);
    const answer = await response.json();
    if (!response.ok) {
      throw new Error(answer.error || response.statusText);
    }
    return answer;
  }

  // post runs one session action and takes in the state it answers, telling how it went.
  function post(path, done) {
    request(path, {}).then(
      (state) => {
        takeState(state);
        report(done, false);
      },
      (error) => report(error.message, true),
    );
  }

  // edit applies one edit. A refused edit stands on its beat, or the clip, until the next
  // accepted one; the Promise rejects with it as well, for a caller that wants to know.
  function edit(change) {
    return request(API.edit, change).then(
      (state) => {
        refusal = null;
        takeState(state);
        return state;
      },
      (error) => {
        refusal = { beat: change.beat, message: error.message };
        renderInspector();
        throw error;
      },
    );
  }

  // applyEdit is edit for the inspector's own fields: the refusal is already shown beside the
  // field, so nothing more is done with it.
  function applyEdit(beat, field, value) {
    edit({ beat, field, value }).catch(() => {});
  }

  // poll asks for the state every second, so outside writes and reloads show up.
  function poll() {
    request(API.state).then(
      (state) => {
        isServerAway = false;
        if (stateKey(state) !== currentKey) {
          takeState(state);
        } else {
          renderBanner();
        }
      },
      () => {
        isServerAway = true;
        renderBanner();
      },
    );
  }

  // stateKey changes whenever anything the page shows changes.
  function stateKey(state) {
    return [state.version, state.dirty, state.stale, state.can_undo, state.can_redo].join("|");
  }

  // takeState makes state the page's, redraws it and tells the listeners.
  function takeState(state) {
    current = state;
    currentKey = stateKey(state);
    if (selectedBeat !== null && !beatOf(selectedBeat)) {
      selectedBeat = null;
    }
    playhead = clampTime(playhead);
    renderAll();
    for (const listener of listeners) {
      try {
        listener(state);
      } catch (error) {
        console.error("tune: a state listener failed", error);
      }
    }
    pumpFrames();
  }

  // --- the clip's frames on the output clock ---

  function frameCount() {
    if (!current || current.fps <= 0) {
      return 0;
    }
    return Math.round((current.length_ms * current.fps) / MILLISECONDS_PER_SECOND);
  }

  // frameIndexAt is the index of the frame on screen at output time ms.
  function frameIndexAt(ms) {
    const index = Math.floor((ms * current.fps) / MILLISECONDS_PER_SECOND);
    return Math.min(Math.max(index, 0), Math.max(frameCount() - 1, 0));
  }

  // frameStartMs is the first whole millisecond at or after frame index's output time.
  function frameStartMs(index) {
    return Math.ceil((index * MILLISECONDS_PER_SECOND) / current.fps);
  }

  function lastFrameMs() {
    return frameStartMs(Math.max(frameCount() - 1, 0));
  }

  function clampTime(ms) {
    if (!current || frameCount() === 0) {
      return 0;
    }
    return Math.min(Math.max(ms, 0), lastFrameMs());
  }

  // pumpFrames asks for the frame at the playhead when it is not the one shown, one request at
  // a time: while one is loading the playhead moves on, and the frames it passed are dropped.
  function pumpFrames() {
    if (isFrameLoading || frameCount() === 0) {
      return;
    }
    const ms = frameStartMs(frameIndexAt(playhead));
    const key = current.version + ":" + ms;
    if (key === shownFrameKey) {
      return;
    }
    isFrameLoading = true;
    loadFrame(ms, key).finally(() => {
      isFrameLoading = false;
      pumpFrames();
    });
  }

  // loadFrame fetches the frame at ms and resolves once the preview shows it.
  async function loadFrame(ms, key) {
    shownFrameKey = key;
    try {
      const response = await fetch(API.frame + ms);
      if (!response.ok) {
        const answer = await response.json();
        throw new Error(answer.error || response.statusText);
      }
      await showFrame(await response.blob(), ms);
      view.frameMessage.hidden = true;
    } catch (error) {
      view.frameMessage.textContent = "No frame at " + formatTime(ms) + ": " + error.message;
      view.frameMessage.hidden = false;
    }
  }

  // showFrame puts the PNG blob in the preview and resolves once it has loaded.
  function showFrame(blob, ms) {
    const address = URL.createObjectURL(blob);
    return new Promise((resolve, reject) => {
      frameImage.onload = () => {
        if (shownFrameURL) {
          URL.revokeObjectURL(shownFrameURL);
        }
        shownFrameURL = address;
        shownFrameMs = ms;
        resolve();
      };
      frameImage.onerror = () => {
        URL.revokeObjectURL(address);
        reject(new Error("the frame did not decode"));
      };
      frameImage.src = address;
    });
  }

  // --- the transport ---

  function seek(ms) {
    playhead = clampTime(ms);
    if (playing) {
      playing = { wallStart: performance.now(), from: playhead };
    }
    renderTime();
    pumpFrames();
  }

  function play() {
    if (frameCount() === 0) {
      return;
    }
    if (playhead >= lastFrameMs()) {
      playhead = 0;
    }
    playing = { wallStart: performance.now(), from: playhead };
    renderPlay();
    requestAnimationFrame(tick);
  }

  function pause() {
    playing = null;
    renderPlay();
  }

  // tick moves the playhead with the wall clock while playing, stopping on the last frame.
  function tick() {
    if (!playing) {
      return;
    }
    const at = playing.from + Math.max(performance.now() - playing.wallStart, 0);
    if (at >= lastFrameMs()) {
      playhead = lastFrameMs();
      pause();
    } else {
      playhead = at;
      requestAnimationFrame(tick);
    }
    renderTime();
    pumpFrames();
  }

  function togglePlay() {
    if (playing) {
      pause();
    } else {
      play();
    }
  }

  function stepFrames(count) {
    if (frameCount() === 0) {
      return;
    }
    pause();
    seek(frameStartMs(frameIndexAt(playhead) + count));
  }

  // sectionStarts are the output times the clip's sections start at, in order, cuts left out.
  function sectionStarts() {
    const starts = current.sections.filter((section) => !section.cut).map((section) => section.start_ms);
    return [...new Set(starts)].sort((first, second) => first - second);
  }

  // jumpSection moves to the previous (-1) or next (+1) section start than the frame shown.
  function jumpSection(direction) {
    if (frameCount() === 0) {
      return;
    }
    const here = frameIndexAt(playhead);
    const starts = sectionStarts().filter((start) =>
      direction < 0 ? frameIndexAt(start) < here : frameIndexAt(start) > here,
    );
    if (starts.length > 0) {
      seek(direction < 0 ? starts[starts.length - 1] : starts[0]);
    }
  }

  // sectionAt is the section on screen at output time ms, or undefined past the clip.
  function sectionAt(ms) {
    return current.sections.find(
      (section) => !section.cut && ms >= section.start_ms && ms < section.start_ms + section.duration_ms,
    );
  }

  // --- drawing ---

  function renderAll() {
    renderBanner();
    renderTransport();
    renderBeatSelect();
    renderInspector();
  }

  function renderBanner() {
    const isStale = Boolean(current && current.stale);
    view.reload.hidden = !isStale;
    view.banner.hidden = !isStale && !isServerAway;
    if (isServerAway) {
      view.bannerText.textContent = "The photon tune server is not answering.";
    } else if (isStale) {
      view.bannerText.textContent =
        "The storyboard changed on disk while edits are unsaved. Save refuses until Reload, " +
        "which drops the unsaved edits.";
    }
  }

  function renderTransport() {
    const hasErrors = current.errors.length > 0;
    view.undo.disabled = !current.can_undo;
    view.redo.disabled = !current.can_redo;
    view.revert.disabled = !current.dirty;
    view.save.disabled = !current.dirty || current.stale || hasErrors;
    view.save.title = hasErrors ? "Save refuses while an error stands" : "Write the storyboard";
    renderPlay();
    renderTime();
  }

  function renderPlay() {
    view.play.textContent = playing ? PAUSE_GLYPH : PLAY_GLYPH;
  }

  function renderTime() {
    if (!current) {
      return;
    }
    const section = sectionAt(playhead);
    const beat = section ? "  beat " + section.beat : "";
    view.time.textContent = formatTime(playhead) + " / " + formatTime(current.length_ms) + beat;
  }

  function renderBeatSelect() {
    const options = [option("", "Clip frame" + errorMark(CLIP))];
    for (const beat of current.beats) {
      options.push(option(String(beat.id), "beat " + beat.id + " " + beat.title + errorMark(beat.id)));
    }
    view.beatSelect.replaceChildren(...options);
    view.beatSelect.value = selectedBeat === null ? "" : String(selectedBeat);
  }

  function renderInspector() {
    if (!current) {
      return;
    }
    const beat = selectedBeat === null ? null : beatOf(selectedBeat);
    view.inspector.replaceChildren(...(beat ? beatPanel(beat) : framePanel()));
  }

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
    return [
      element("h2", {}, "Clip frame"),
      countField("width (px)", "frame.width", current.width),
      countField("fps", "frame.fps", current.fps),
      ...READ_ONLY_FRAME_FIELDS.map((name) =>
        fieldRow(name, element("span", { className: "read-only" }, String(current[name]))),
      ),
      element("p", { className: "read-only" }, "Length " + formatTime(current.length_ms)),
      ...current.warnings.map((warning) => element("p", { className: "error" }, warning)),
      ...errorRows(CLIP),
    ];
  }

  function millisecondsField(label, beat, field, ms) {
    const input = element("input", { type: "number", min: "0", step: "1", value: String(ms) });
    input.addEventListener("change", () => applyEdit(beat, field, input.value.trim()));
    return fieldRow(label + " (ms)", input);
  }

  function countField(label, field, count) {
    const input = element("input", { type: "number", min: "1", step: "1", value: String(count) });
    input.addEventListener("change", () => applyEdit(CLIP, field, input.value.trim()));
    return fieldRow(label, input);
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
      const target = element("input", { type: "text", placeholder: "{text: 'ok', area: list}" });
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
    const section = current.sections.find((candidate) => candidate.beat === beat);
    if (!section) {
      return [];
    }
    const isTooFast = current.warnings.some((warning) => warning.startsWith("beat " + beat + " "));
    return [element("p", { className: isTooFast ? "report too-fast" : "report" }, section.report)];
  }

  // errorRows are the errors standing on beat (or the clip, for 0) and a refused edit of it.
  function errorRows(beat) {
    const messages = current.errors.filter((problem) => problem.beat === beat).map((problem) => problem.message);
    if (refusal && refusal.beat === beat) {
      messages.push(refusal.message);
    }
    return messages.map((message) => element("p", { className: "error" }, message));
  }

  function errorMark(beat) {
    const hasError = current.errors.some((problem) => problem.beat === beat);
    return hasError ? "  (error)" : "";
  }

  function needsRecord() {
    return element("span", { className: "needs-record" }, NEEDS_RECORD);
  }

  // report shows how the last action went in the transport bar.
  function report(message, isFailure) {
    view.status.textContent = message;
    view.status.classList.toggle("failed", isFailure);
  }

  // --- helpers ---

  function byId(id) {
    return document.getElementById(id);
  }

  // element builds a tag with properties set and children appended; strings become text.
  function element(tag, properties, ...children) {
    const node = Object.assign(document.createElement(tag), properties);
    node.append(...children);
    return node;
  }

  function option(value, label) {
    return element("option", { value }, label);
  }

  function fieldRow(label, control) {
    return element("label", { className: "field" }, element("span", {}, label), control);
  }

  function beatOf(id) {
    return current.beats.find((beat) => beat.id === id);
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

  // formatTime spells ms as m:ss.mmm.
  function formatTime(ms) {
    const whole = Math.max(Math.round(ms), 0);
    const minutes = Math.floor(whole / MILLISECONDS_PER_MINUTE);
    const seconds = Math.floor((whole % MILLISECONDS_PER_MINUTE) / MILLISECONDS_PER_SECOND);
    const milliseconds = whole % MILLISECONDS_PER_SECOND;
    return (
      minutes + ":" + String(seconds).padStart(2, "0") + "." + String(milliseconds).padStart(3, "0")
    );
  }

  function select(id) {
    selectedBeat = id === null || id === undefined ? null : Number(id);
    if (current) {
      renderBeatSelect();
      renderInspector();
    }
  }

  // --- wiring ---

  // isTyping says a key goes to a form field, which keeps its own keys.
  function isTyping(event) {
    const target = event.target;
    return target instanceof HTMLInputElement || target instanceof HTMLSelectElement;
  }

  // onKey: Space plays or pauses, the arrows step a frame, and with Shift a second.
  function onKey(event) {
    if (isTyping(event) || !current) {
      return;
    }
    if (event.key === " ") {
      togglePlay();
    } else if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
      const direction = event.key === "ArrowLeft" ? -1 : 1;
      if (event.shiftKey) {
        seek(playhead + direction * SECOND_STEP_MS);
      } else {
        stepFrames(direction);
      }
    } else {
      return;
    }
    event.preventDefault();
  }

  // onKeyUp keeps Space from also pressing the button that has the focus, which a browser does
  // when the key comes up.
  function onKeyUp(event) {
    if (event.key === " " && !isTyping(event)) {
      event.preventDefault();
    }
  }

  function wire() {
    view.play.addEventListener("click", togglePlay);
    view.previous.addEventListener("click", () => jumpSection(-1));
    view.next.addEventListener("click", () => jumpSection(1));
    view.undo.addEventListener("click", () => post(API.undo, "Undone"));
    view.redo.addEventListener("click", () => post(API.redo, "Redone"));
    view.revert.addEventListener("click", () => post(API.revert, "Reverted to the storyboard on disk"));
    view.save.addEventListener("click", () => post(API.save, "Saved"));
    view.reload.addEventListener("click", () => post(API.reload, "Reloaded from disk"));
    view.beatSelect.addEventListener("change", () =>
      select(view.beatSelect.value === "" ? null : view.beatSelect.value),
    );
    document.addEventListener("keydown", onKey);
    document.addEventListener("keyup", onKeyUp);
    setInterval(poll, POLL_INTERVAL_MS);
    poll();
  }

  window.tune = Object.freeze({
    state: () => current,
    selected: () => selectedBeat,
    select: select,
    time: () => Math.round(playhead),
    frameTime: () => shownFrameMs,
    seek: seek,
    pause: pause,
    edit: edit,
    onState: (listener) => {
      listeners.push(listener);
    },
    frame: frameImage,
  });

  wire();
})();
