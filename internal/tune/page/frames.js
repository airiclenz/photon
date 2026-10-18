// photon tune's preview frames: which frame the preview asks the server for at the playhead, and
// putting it on screen. While the preview moves — playing, or a scrub's pointer down — it asks for
// draft frames (/api/frame?t=<ms>&draft=1), which the server draws fast from a scale-1 raster;
// once the playhead settles it asks for the exact frame `photon render` composes, showing a draft
// first unless the exact frame is already at hand. An exact frame shown is never replaced by a
// draft of the same state version and time. One request is in flight at a time: while one loads
// the playhead moves on, and the frames it passed are dropped.
//
// It loads after app.js and before transport.js, and adds pumpFrames, settleFrames and frameTime
// to window.tunePage, the page's internal namespace, where app.js's takeState and window.tune
// contract and transport.js's clock reach them. The clock is transport.js's: it reads the
// playhead, frameCount, frameIndexAt, frameStartMs, isPlaying and isScrubbing from the namespace
// when a request goes out, never at load, since transport.js adds them after this file runs.
"use strict";

(() => {
  const page = window.tunePage;
  const FRAME_API = "/api/frame?t=";
  const DRAFT_QUERY = "&draft=1";
  const QUALITY_DRAFT = "draft";
  const QUALITY_EXACT = "exact";
  const KEY_SEPARATOR = ":";

  const view = page.view;
  const frameImage = page.frame;

  let shownFrameMs = null;
  // loadedFrameKey is the version:quality:ms key of the frame last asked for, shown or failed, so
  // a failed frame is not asked for again until the playhead, the quality or the state moves.
  let loadedFrameKey = "";
  let isFrameLoading = false;
  let shownFrameURL = "";
  // isDraftFirst asks the next settled request to show a draft before the exact frame.
  let isDraftFirst = false;

  // frameKey names the frame of quality at output time ms of the current state.
  function frameKey(quality, ms) {
    return page.state().version + KEY_SEPARATOR + quality + KEY_SEPARATOR + ms;
  }

  // isMoving says the playhead is moving: playing, or a scrub's pointer is down.
  function isMoving() {
    return page.isPlaying() || page.isScrubbing();
  }

  // wantedQuality is the quality of the frame to ask for at ms, or null when the frame shown
  // already is the one wanted there.
  function wantedQuality(ms) {
    const exactKey = frameKey(QUALITY_EXACT, ms);
    const isDraftShown = loadedFrameKey === frameKey(QUALITY_DRAFT, ms);
    const isDraftFirstAsked = isDraftFirst;
    isDraftFirst = false;
    if (loadedFrameKey === exactKey) {
      return null;
    }
    if (isMoving()) {
      return isDraftShown ? null : QUALITY_DRAFT;
    }
    return isDraftFirstAsked && !isDraftShown ? QUALITY_DRAFT : QUALITY_EXACT;
  }

  // pumpFrames asks for the frame wanted at the playhead when it is not the one shown: a draft
  // while the preview moves, the exact frame once it has settled.
  function pumpFrames() {
    if (isFrameLoading || page.frameCount() === 0) {
      return;
    }
    const ms = page.frameStartMs(page.frameIndexAt(page.playhead()));
    const quality = wantedQuality(ms);
    if (quality === null) {
      return;
    }
    isFrameLoading = true;
    loadFrame(ms, quality).finally(() => {
      isFrameLoading = false;
      pumpFrames();
    });
  }

  // settleFrames settles the preview on the exact frame at the playhead, showing a draft first
  // unless the exact frame is at hand. Its request goes out once the caller's turn is over, so a
  // caller that pauses and then seeks asks for the frame at the playhead it seeks to.
  function settleFrames() {
    isDraftFirst = true;
    Promise.resolve().then(pumpFrames);
  }

  // loadFrame fetches the frame of quality at ms and resolves once the preview shows it.
  async function loadFrame(ms, quality) {
    loadedFrameKey = frameKey(quality, ms);
    const address = FRAME_API + ms + (quality === QUALITY_DRAFT ? DRAFT_QUERY : "");
    try {
      const response = await fetch(address);
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

  Object.assign(page, {
    frameTime: () => shownFrameMs,
    pumpFrames,
    settleFrames,
  });
})();
