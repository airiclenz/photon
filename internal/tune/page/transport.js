// photon tune's frame clock and transport: the playhead on the clip's output clock, the frames
// the preview shows at it, playing, stepping and jumping between section starts, and the keys
// that drive them.
//
// It loads after app.js and adds its functions to window.tunePage, the page's internal namespace,
// where app.js's window.tune contract (time, frameTime, seek, pause) and its wiring reach them.
// It reads the state through the namespace too and computes no timing of its own: the fps,
// length and section starts all come from /api/state, and every frame is one the server composes.
"use strict";

(() => {
  const page = window.tunePage;
  const FRAME_API = "/api/frame?t=";
  const MILLISECONDS_PER_SECOND = 1000;
  const SECOND_STEP_MS = 1000;
  const PLAY_GLYPH = "▶";
  const PAUSE_GLYPH = "⏸";

  const view = page.view;
  const frameImage = page.frame;

  let playhead = 0;
  let playing = null;
  let shownFrameMs = null;
  let shownFrameKey = "";
  let isFrameLoading = false;
  let shownFrameURL = "";

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

  // pumpFrames asks for the frame at the playhead when it is not the one shown, one request at
  // a time: while one is loading the playhead moves on, and the frames it passed are dropped.
  function pumpFrames() {
    if (isFrameLoading || frameCount() === 0) {
      return;
    }
    const ms = frameStartMs(frameIndexAt(playhead));
    const key = page.state().version + ":" + ms;
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
      const response = await fetch(FRAME_API + ms);
      if (!response.ok) {
        const answer = await response.json();
        throw new Error(answer.error || response.statusText);
      }
      await showFrame(await response.blob(), ms);
      view.frameMessage.hidden = true;
    } catch (error) {
      view.frameMessage.textContent = "No frame at " + page.formatTime(ms) + ": " + error.message;
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

  Object.assign(page, {
    time: () => Math.round(playhead),
    frameTime: () => shownFrameMs,
    clampPlayhead,
    pumpFrames,
    seek,
    pause,
    togglePlay,
    jumpSection,
    renderPlay,
    renderTime,
    onKey,
    onKeyUp,
  });
})();
