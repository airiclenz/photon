// photon tune's preview frames: which frames the preview asks the server for around the playhead,
// the frames it holds at hand, and putting one on screen. While the preview moves — playing, or a
// scrub's pointer down — it asks for draft frames (/api/frame?t=<ms>&draft=1), which the server
// draws fast from a scale-1 raster; once the playhead settles it asks for the exact frame
// `photon render` composes, showing a draft first unless the exact frame is already at hand. An
// exact frame shown is never replaced by a draft of the same state version and time.
//
// While playing it keeps up to PREFETCH_REQUESTS requests in flight for the frames on the clip's
// fps grid from the playhead's on, and the frames that arrive wait in a cache of at most
// CACHE_ENTRIES, keyed version:quality:ms, until the clock reaches them; a frame the playhead has
// passed is dropped, and an answer that arrives for one is ignored. Otherwise one request is in
// flight at a time, counting only those sent while not playing: while one loads the playhead moves
// on, and the frames it passed are dropped. A frame at hand is shown without a request. The cache
// owns the object URLs of the frames it holds and revokes one when it evicts the frame, or when
// the state version changes, unless the preview shows that URL or is loading it; the preview
// revokes the URL it replaces once no cache entry holds it.
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
  // PREFETCH_REQUESTS is how many frame requests a play keeps in flight.
  const PREFETCH_REQUESTS = 4;
  // PREFETCH_FRAMES is how many frames, the playhead's first, a play fetches ahead.
  const PREFETCH_FRAMES = 8;
  // CACHE_ENTRIES bounds the frames held at hand.
  const CACHE_ENTRIES = 48;

  const view = page.view;
  const frameImage = page.frame;

  // cache holds the frames at hand of cacheVersion by key, least recently used first, each as
  // {key, ms, url}.
  const cache = new Map();
  let cacheVersion = null;
  // inFlight are the keys of the frame requests whose answers have not arrived.
  const inFlight = new Set();
  // stillRequests counts the requests in flight that were sent while not playing.
  let stillRequests = 0;
  // failedKeys are the frames whose requests failed while the playhead stood on the frame at
  // failedAtMs, not asked for again until the playhead, the quality or the state moves.
  const failedKeys = new Set();
  let failedAtMs = null;
  // displayedKey is the key of the frame last put in the preview, loaded, loading or undecodable.
  let displayedKey = "";
  let pendingURL = "";
  let shownURL = "";
  let shownFrameMs = null;
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

  // playheadFrameMs is the output time of the frame at the playhead.
  function playheadFrameMs() {
    return page.frameStartMs(page.frameIndexAt(page.playhead()));
  }

  // --- the frames at hand ---

  // releaseURL revokes url unless the preview shows it, is loading it or a cache entry holds it.
  function releaseURL(url) {
    if (!url || url === shownURL || url === pendingURL) {
      return;
    }
    for (const entry of cache.values()) {
      if (entry.url === url) {
        return;
      }
    }
    URL.revokeObjectURL(url);
  }

  function evict(key) {
    const entry = cache.get(key);
    cache.delete(key);
    releaseURL(entry.url);
  }

  // syncVersion empties the cache when the state version has moved on.
  function syncVersion() {
    const version = page.state().version;
    if (version === cacheVersion) {
      return;
    }
    for (const key of [...cache.keys()]) {
      evict(key);
    }
    failedKeys.clear();
    cacheVersion = version;
  }

  // cached is the frame of quality at ms at hand, marked as just used, or undefined.
  function cached(quality, ms) {
    const key = frameKey(quality, ms);
    const entry = cache.get(key);
    if (entry) {
      cache.delete(key);
      cache.set(key, entry);
    }
    return entry;
  }

  // store puts the frame blob of key at ms at hand, evicting the least recently used past the
  // bound.
  function store(key, ms, blob) {
    const entry = { key, ms, url: URL.createObjectURL(blob) };
    cache.set(key, entry);
    while (cache.size > CACHE_ENTRIES) {
      evict(cache.keys().next().value);
    }
    return entry;
  }

  // dropPassed evicts the frames before the one at ms.
  function dropPassed(ms) {
    for (const [key, entry] of [...cache]) {
      if (entry.ms < ms) {
        evict(key);
      }
    }
  }

  // isAtHand says a frame at ms of either quality is shown, cached, asked for or failed.
  function isAtHand(ms) {
    return [QUALITY_DRAFT, QUALITY_EXACT].some((quality) => {
      const key = frameKey(quality, ms);
      return displayedKey === key || cache.has(key) || inFlight.has(key) || failedKeys.has(key);
    });
  }

  // --- choosing the frames ---

  // pumpFrames asks for the frames wanted around the playhead and shows the one at it when it is
  // at hand: drafts while the preview moves, the exact frame once it has settled.
  function pumpFrames() {
    if (page.frameCount() === 0) {
      return;
    }
    syncVersion();
    const ms = playheadFrameMs();
    if (ms !== failedAtMs) {
      failedKeys.clear();
      failedAtMs = ms;
    }
    if (isMoving()) {
      pumpMoving(ms);
    } else {
      pumpStill(ms);
    }
  }

  // pumpMoving shows the frame at ms when it is at hand, then fetches the frames ahead while
  // playing, or the draft at ms during a scrub.
  function pumpMoving(ms) {
    isDraftFirst = false;
    const isDisplayed =
      displayedKey === frameKey(QUALITY_EXACT, ms) || displayedKey === frameKey(QUALITY_DRAFT, ms);
    const entry = isDisplayed ? undefined : cached(QUALITY_EXACT, ms) || cached(QUALITY_DRAFT, ms);
    if (entry) {
      show(entry);
    }
    if (page.isPlaying()) {
      prefetch(ms);
    } else if (stillRequests === 0 && !isAtHand(ms)) {
      request(QUALITY_DRAFT, ms, false);
    }
  }

  // prefetch drops the frames before ms and asks for the drafts from ms on that are not at hand,
  // nearest first, while fewer than PREFETCH_REQUESTS requests are in flight.
  function prefetch(ms) {
    dropPassed(ms);
    const first = page.frameIndexAt(ms);
    const end = Math.min(first + PREFETCH_FRAMES, page.frameCount());
    for (let index = first; index < end && inFlight.size < PREFETCH_REQUESTS; index++) {
      const at = page.frameStartMs(index);
      if (!isAtHand(at)) {
        request(QUALITY_DRAFT, at, true);
      }
    }
  }

  // pumpStill settles the preview on the exact frame at ms, showing a draft first when asked.
  function pumpStill(ms) {
    if (stillRequests > 0) {
      return;
    }
    const isDraftFirstAsked = isDraftFirst;
    isDraftFirst = false;
    const exactKey = frameKey(QUALITY_EXACT, ms);
    if (displayedKey === exactKey || failedKeys.has(exactKey)) {
      return;
    }
    const exact = cached(QUALITY_EXACT, ms);
    if (exact) {
      show(exact);
      return;
    }
    const draftKey = frameKey(QUALITY_DRAFT, ms);
    if (isDraftFirstAsked && displayedKey !== draftKey) {
      const draft = cached(QUALITY_DRAFT, ms);
      if (draft) {
        show(draft);
      } else if (!inFlight.has(draftKey) && !failedKeys.has(draftKey)) {
        request(QUALITY_DRAFT, ms, false);
        return;
      }
    }
    request(QUALITY_EXACT, ms, false);
  }

  // settleFrames settles the preview on the exact frame at the playhead, showing a draft first
  // unless the exact frame is at hand. Its request goes out once the caller's turn is over, so a
  // caller that pauses and then seeks asks for the frame at the playhead it seeks to.
  function settleFrames() {
    isDraftFirst = true;
    Promise.resolve().then(pumpFrames);
  }

  // --- fetching and showing ---

  // request fetches the frame of quality at ms. Its answer is ignored once the state version has
  // moved on, and a prefetched one too once the playhead has passed its frame; a frame asked for
  // while not playing is shown when it arrives, a prefetched one when the clock reaches it.
  async function request(quality, ms, isPrefetch) {
    const key = frameKey(quality, ms);
    const version = page.state().version;
    inFlight.add(key);
    stillRequests += isPrefetch ? 0 : 1;
    let blob = null;
    let failure = null;
    try {
      blob = await fetchFrame(quality, ms);
    } catch (error) {
      failure = error;
    }
    inFlight.delete(key);
    stillRequests -= isPrefetch ? 0 : 1;
    syncVersion();
    const isCurrent = version === page.state().version && !(isPrefetch && ms < playheadFrameMs());
    if (isCurrent && failure) {
      failedKeys.add(key);
      reportMissing(ms, failure);
    } else if (isCurrent) {
      const entry = store(key, ms, blob);
      if (!isPrefetch) {
        show(entry);
      }
    }
    pumpFrames();
  }

  // fetchFrame resolves to the PNG blob of the frame of quality at ms.
  async function fetchFrame(quality, ms) {
    const response = await fetch(FRAME_API + ms + (quality === QUALITY_DRAFT ? DRAFT_QUERY : ""));
    if (!response.ok) {
      const answer = await response.json();
      throw new Error(answer.error || response.statusText);
    }
    return response.blob();
  }

  function reportMissing(ms, error) {
    view.frameMessage.textContent = "No frame at " + page.formatTime(ms) + ": " + error.message;
    view.frameMessage.hidden = false;
  }

  // show puts the cached frame entry in the preview; frameTime() names it once it has loaded.
  function show(entry) {
    const replaced = pendingURL;
    pendingURL = entry.url;
    displayedKey = entry.key;
    releaseURL(replaced);
    frameImage.onload = () => {
      const previous = shownURL;
      shownURL = entry.url;
      pendingURL = "";
      shownFrameMs = entry.ms;
      releaseURL(previous);
      view.frameMessage.hidden = true;
    };
    frameImage.onerror = () => {
      pendingURL = "";
      if (cache.get(entry.key) === entry) {
        cache.delete(entry.key);
      }
      releaseURL(entry.url);
      reportMissing(entry.ms, new Error("the frame did not decode"));
    };
    frameImage.src = entry.url;
  }

  Object.assign(page, {
    frameTime: () => shownFrameMs,
    pumpFrames,
    settleFrames,
  });
})();
