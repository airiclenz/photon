// photon tune's page: the preview, the inspector and the transport bar over the session's API.
// It holds no timing logic of its own: every value it shows comes from /api/state, every change
// is an edit the server applies and validates, and every frame is one the server composes.
//
// window.tune is the contract the timeline lanes, the target picker and the preview fit build
// on:
//   state()      the last /api/state JSON, or null before the first arrives
//   selected()   the id of the beat the inspector shows, or null for the clip's frame or an entry
//   select(id)   show beat id in the inspector, or the clip's frame for null
//   selectedEntry()    the zoom or caption the inspector shows, as {kind, index}, or null:
//                      kind "zoom" or "caption", index its place in state().zooms or .captions
//   selectEntry(entry) show entry, {kind, index}, in the inspector
//   time()       the playhead, in output milliseconds
//   frameTime()  the output milliseconds of the frame the preview <img> shows, or null
//   seek(ms)     move the playhead to ms, clamped to the clip
//   pause()      stop playing
//   edit(edit)   apply {owner: {kind, index}, field, value}: owner a beat by its id, the clip
//                with index 0, or a zoom or caption by its index, value typed per field; a
//                Promise of the new state, rejected when refused
//   onState(fn)  call fn(state) with every new state
//   frame        the preview <img> element
//
// This file holds the session's API, the selection and the wiring. The page's own scripts that
// load after it — kinds.js (the entry kinds and their lookups), frames.js (the preview's draft
// and exact frames), transport.js (the frame clock, the transport and its status), controls.js
// (the inspector's field controls and error rows), panels.js (the beat and frame panels) and
// entries.js (the zoom and caption panels) — share its state through window.tunePage, an internal
// namespace no part of the contract: app.js puts in it the state they read and the helpers they build with, and each of them adds what it does. Of
// the other scripts picker.js adds the standing pick's proposal, and timeline.js marks a scrub
// through transport.js's beginScrub and endScrub and reports a refused lane add or bar drag
// through its reportStatus; fit.js, the preview fit, reads window.tune alone. app.js calls those only once every script has run, so the wiring, and
// with it the first /api/state request, waits for DOMContentLoaded.
"use strict";

(() => {
  const API = {
    state: "/api/state",
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
  const CLIP = 0;

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
  let selectedEntry = null;
  let refusal = null;
  let isServerAway = false;

  // page is window.tunePage, the internal namespace the page's own scripts share.
  const page = {
    CLIP,
    view,
    frame: frameImage,
    state: () => current,
    refusal: () => refusal,
    applyEdit,
    isEntry,
    element,
    option,
    formatTime,
  };
  window.tunePage = page;

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
        page.reportStatus(done, false);
      },
      (error) => page.reportStatus(error.message, true),
    );
  }

  // edit applies one edit. A refused edit stands on its beat, the clip, or its zoom or caption
  // until the next accepted one, which also clears the transport bar's status; the Promise rejects
  // with it as well, for a caller that wants to know.
  function edit(change) {
    return request(API.edit, change).then(
      (state) => {
        refusal = null;
        takeState(state);
        page.reportStatus("", false);
        return state;
      },
      (error) => {
        refusal = { owner: ownerKey(change), message: error.message };
        renderInspector();
        throw error;
      },
    );
  }

  // applyEdit is edit for the inspector's own fields: the refusal is already shown beside the
  // field, so nothing more is done with it. owner is the beat id, 0 for the clip, or the
  // {kind, index} entry of a zoom's or caption's field.
  function applyEdit(owner, field, value) {
    edit({ owner: page.problemOwner(owner), field, value }).catch(() => {});
  }

  // ownerKey names what change stands on: its entry's beat select value for a zoom's or caption's
  // field, else its beat's id, or the clip's 0.
  function ownerKey(change) {
    const { kind, index } = change.owner;
    if (page.isEntryKind(kind)) {
      return page.entryValue(kind, index);
    }
    return String(index);
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
    if (selectedEntry !== null && !page.entryOf(selectedEntry)) {
      selectedEntry = null;
    }
    page.clampPlayhead();
    renderAll();
    for (const listener of listeners) {
      try {
        listener(state);
      } catch (error) {
        console.error("tune: a state listener failed", error);
      }
    }
    page.pumpFrames();
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
    page.renderPlay();
    page.renderTime();
  }

  function renderBeatSelect() {
    const options = [option("", "Clip frame" + page.errorMark(CLIP))];
    for (const beat of current.beats) {
      options.push(option(String(beat.id), "beat " + beat.id + " " + beat.title + page.errorMark(beat.id)));
    }
    options.push(...page.entryOptions());
    view.beatSelect.replaceChildren(...options);
    view.beatSelect.value = selectionValue();
  }

  // selectionValue is the beat select's value for what the inspector shows.
  function selectionValue() {
    if (selectedEntry !== null) {
      return page.entryValue(selectedEntry.kind, selectedEntry.index);
    }
    return selectedBeat === null ? "" : String(selectedBeat);
  }

  function renderInspector() {
    if (!current) {
      return;
    }
    const entry = selectedEntry === null ? null : page.entryOf(selectedEntry);
    if (entry) {
      view.inspector.replaceChildren(...page.entryPanel(selectedEntry.kind, entry));
      return;
    }
    const beat = selectedBeat === null ? null : beatOf(selectedBeat);
    view.inspector.replaceChildren(...(beat ? page.beatPanel(beat) : page.framePanel()));
  }

  // isEntry says owner is a zoom's or caption's {kind, index} rather than a beat id.
  function isEntry(owner) {
    return typeof owner === "object" && owner !== null;
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

  function beatOf(id) {
    return current.beats.find((beat) => beat.id === id);
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
    selectedEntry = null;
    renderSelection();
  }

  // selectEntry shows entry, {kind, index}, in the inspector in place of a beat.
  function selectEntry(entry) {
    selectedEntry = Object.freeze({ kind: entry.kind, index: Number(entry.index) });
    selectedBeat = null;
    renderSelection();
  }

  // selectValue selects what a beat select value names: the clip's frame, a beat or an entry.
  function selectValue(value) {
    const entry = page.entryOfValue(value);
    if (entry) {
      selectEntry(entry);
    } else {
      select(value === "" ? null : value);
    }
  }

  function renderSelection() {
    if (current) {
      renderBeatSelect();
      renderInspector();
    }
  }

  // --- wiring ---

  function wire() {
    view.play.addEventListener("click", page.togglePlay);
    view.previous.addEventListener("click", () => page.jumpSection(-1));
    view.next.addEventListener("click", () => page.jumpSection(1));
    view.undo.addEventListener("click", () => post(API.undo, "Undone"));
    view.redo.addEventListener("click", () => post(API.redo, "Redone"));
    view.revert.addEventListener("click", () => post(API.revert, "Reverted to the storyboard on disk"));
    view.save.addEventListener("click", () => post(API.save, "Saved"));
    view.reload.addEventListener("click", () => post(API.reload, "Reloaded from disk"));
    view.beatSelect.addEventListener("change", () => selectValue(view.beatSelect.value));
    document.addEventListener("keydown", page.onKey);
    document.addEventListener("keyup", page.onKeyUp);
    setInterval(poll, POLL_INTERVAL_MS);
    poll();
  }

  window.tune = Object.freeze({
    state: () => current,
    selected: () => selectedBeat,
    select: select,
    selectedEntry: () => selectedEntry,
    selectEntry: selectEntry,
    time: () => page.time(),
    frameTime: () => page.frameTime(),
    seek: (ms) => page.seek(ms),
    pause: () => page.pause(),
    edit: edit,
    onState: (listener) => {
      listeners.push(listener);
    },
    frame: frameImage,
  });

  document.addEventListener("DOMContentLoaded", wire);
})();
