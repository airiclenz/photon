// photon tune's frame clock and transport: the playhead on the clip's output clock, playing,
// scrubbing, stepping and jumping between section starts, the keys that drive them, the bar's
// status, which tells how the last action went, and the build of the photon serving the page, at
// the bar's right end. frames.js shows the frames at the playhead: this file asks it for drafts
// while the preview moves (pumpFrames) and to settle on the exact frame once it stops
// (settleFrames).
//
// It loads after app.js and frames.js and adds its functions to window.tunePage, the page's
// internal namespace, where app.js's window.tune contract (time, seek, pause), its wiring and its
// actions reach them; frames.js reads the clock there (playhead, frameCount, frameIndexAt,
// frameStartMs, isPlaying, isScrubbing), timeline.js marks a scrub's pointer down and up through
// beginScrub and endScrub, and reports a refused lane add or bar drag through reportStatus there
// too. It reads the state through the namespace too and computes no timing of its own: the fps,
// length and section starts all come from /api/state, and every frame is one the server composes.
"use strict";

(() => {
  const page = window.tunePage;
  const MILLISECONDS_PER_SECOND = 1000;
  const SECOND_STEP_MS = 1000;
  const PLAY_GLYPH = "▶";
  const PAUSE_GLYPH = "⏸";

  const view = page.view;
  const buildText = document.getElementById("build");

  let playhead = 0;
  let playing = null;
  let isScrubbing = false;

  // --- the clip's frames on the output clock ---

  function frameCount() {
    const state = page.state();
    if (!state || state.fps <= 0) {
      return 0;
    }
    return Math.round((state.length_ms * state.fps) / MILLISECONDS_PER_SECOND);
  }

  // frameIndexAt is the index of the frame on screen at output time ms.
  function frameIndexAt(ms) {
    const index = Math.floor((ms * page.state().fps) / MILLISECONDS_PER_SECOND);
    return Math.min(Math.max(index, 0), Math.max(frameCount() - 1, 0));
  }

  // frameStartMs is the first whole millisecond at or after frame index's output time.
  function frameStartMs(index) {
    return Math.ceil((index * MILLISECONDS_PER_SECOND) / page.state().fps);
  }

  function lastFrameMs() {
    return frameStartMs(Math.max(frameCount() - 1, 0));
  }

  function clampTime(ms) {
    if (!page.state() || frameCount() === 0) {
      return 0;
    }
    return Math.min(Math.max(ms, 0), lastFrameMs());
  }

  // clampPlayhead keeps the playhead inside the clip a new state describes.
  function clampPlayhead() {
    playhead = clampTime(playhead);
  }

  // --- the transport ---

  // seek moves the playhead to ms: while the preview moves the frames follow it as drafts, and a
  // jump while it is still settles it on the exact frame there.
  function seek(ms) {
    playhead = clampTime(ms);
    if (playing) {
      playing = { wallStart: performance.now(), from: playhead };
    }
    renderTime();
    if (playing || isScrubbing) {
      page.pumpFrames();
    } else {
      page.settleFrames();
    }
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

  // pause stops playing. Stopping a play settles the preview on the exact frame, except during a
  // scrub, whose release settles it.
  function pause() {
    const wasPlaying = playing !== null;
    playing = null;
    renderPlay();
    if (wasPlaying && !isScrubbing) {
      page.settleFrames();
    }
  }

  // beginScrub marks a scrub's pointer down: until endScrub the preview shows draft frames.
  function beginScrub() {
    isScrubbing = true;
  }

  // endScrub marks the scrub's pointer up and settles the preview on the exact frame.
  function endScrub() {
    isScrubbing = false;
    page.settleFrames();
  }

  // tick moves the playhead with the wall clock while playing, stopping on the last frame.
  function tick() {
    if (!playing) {
      return;
    }
    const at = playing.from + Math.max(performance.now() - playing.wallStart, 0);
    if (at >= lastFrameMs()) {
      playhead = lastFrameMs();
      renderTime();
      pause();
      return;
    }
    playhead = at;
    requestAnimationFrame(tick);
    renderTime();
    page.pumpFrames();
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
    const starts = page.state().sections.filter((section) => !section.cut).map((section) => section.start_ms);
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
    return page.state().sections.find(
      (section) => !section.cut && ms >= section.start_ms && ms < section.start_ms + section.duration_ms,
    );
  }

  // --- drawing ---

  function renderPlay() {
    view.play.textContent = playing ? PAUSE_GLYPH : PLAY_GLYPH;
  }

  function renderTime() {
    const state = page.state();
    if (!state) {
      return;
    }
    const section = sectionAt(playhead);
    const beat = section ? "  beat " + section.beat : "";
    view.time.textContent = page.formatTime(playhead) + " / " + page.formatTime(state.length_ms) + beat;
    if (buildText.textContent !== state.build) {
      buildText.textContent = state.build;
    }
  }

  // --- the keys ---

  // isTyping says a key goes to a form field, which keeps its own keys.
  function isTyping(event) {
    const target = event.target;
    return (
      target instanceof HTMLInputElement ||
      target instanceof HTMLSelectElement ||
      target instanceof HTMLTextAreaElement
    );
  }

  // onKey: Space plays or pauses, the arrows step a frame, and with Shift a second.
  function onKey(event) {
    if (isTyping(event) || !page.state()) {
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

  // --- the status ---

  // reportStatus shows message in the transport bar's status, styled as a failure when
  // isFailure; an empty message clears it.
  function reportStatus(message, isFailure) {
    view.status.textContent = message;
    view.status.classList.toggle("failed", isFailure);
  }

  Object.assign(page, {
    time: () => Math.round(playhead),
    playhead: () => playhead,
    isPlaying: () => playing !== null,
    isScrubbing: () => isScrubbing,
    frameCount,
    frameIndexAt,
    frameStartMs,
    clampPlayhead,
    seek,
    pause,
    beginScrub,
    endScrub,
    togglePlay,
    jumpSection,
    renderPlay,
    renderTime,
    onKey,
    onKeyUp,
    reportStatus,
  });
})();
