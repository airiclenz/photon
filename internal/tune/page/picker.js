// photon tune's target picker: ⌖ arms pick mode on the paused preview, and a drag across the
// cells of one row asks the server for the target that names them.
// It builds on app.js's window.tune: frame for the preview <img>, pause() to hold the frame
// still, frameTime() for the output time of the frame the <img> shows, selectedEntry() and
// state() for the pan move a pan target goes on, onState(fn) to offer the pan target only while
// a pan exists, and edit(edit) to set it. It adds pickProposal() and
// reportPick(message, isFailure) to window.tunePage, the page's internal namespace, where
// entries.js's via waypoints read the standing proposal's target. Nothing here maps pixels to
// cells or resolves a target: the drag's two corners go to /api/pick in output-frame pixels, and
// the server answers the resolver-checked proposal: its target as a camera.to edit takes it, its
// box in output-frame pixels and the YAML snippets that write it.
"use strict";

(() => {
  const tune = window.tune;
  const PICK_URL = "/api/pick";
  const SNIPPET_ZOOM = "zoom";
  const FIELD_CAMERA_TO = "camera.to";
  const ENTRY_CAMERA = "camera";
  const KIND_PAN = "pan";
  const ESCAPE_KEY = "Escape";
  const ARMED_HINT = "Drag across the cells of one row on the preview; Esc cancels.";

  const frameImage = tune.frame;
  const view = {
    stage: document.getElementById("stage"),
    arm: document.getElementById("pick"),
    status: document.getElementById("pick-status"),
    drag: document.getElementById("pick-drag"),
    box: document.getElementById("pick-box"),
    result: document.getElementById("pick-result"),
    target: document.getElementById("pick-target"),
    setPan: document.getElementById("pick-pan"),
    copies: document.querySelectorAll("#pick-result [data-snippet]"),
  };

  let isArmed = false;
  let drag = null;
  let proposal = null;
  let proposalMs = null;

  // --- pick mode ---

  function arm() {
    tune.pause();
    isArmed = true;
    view.stage.classList.add("picking");
    view.arm.setAttribute("aria-pressed", "true");
    report(ARMED_HINT, false);
  }

  function disarm() {
    isArmed = false;
    drag = null;
    view.drag.hidden = true;
    view.stage.classList.remove("picking");
    view.arm.setAttribute("aria-pressed", "false");
  }

  function toggleArmed() {
    if (isArmed) {
      disarm();
      report("", false);
    } else {
      arm();
    }
  }

  // --- the drag, in the <img>'s displayed pixels ---

  function onPointerDown(event) {
    if (!isArmed || event.button !== 0) {
      return;
    }
    if (tune.frameTime() === null) {
      report("No frame is shown to pick from.", true);
      return;
    }
    event.preventDefault();
    tune.pause();
    view.stage.setPointerCapture(event.pointerId);
    const at = displayedPoint(event);
    drag = { from: at, to: at };
    drawDrag();
  }

  function onPointerMove(event) {
    if (!drag) {
      return;
    }
    drag.to = displayedPoint(event);
    drawDrag();
  }

  function onPointerUp(event) {
    if (!drag) {
      return;
    }
    drag.to = displayedPoint(event);
    const picked = drag;
    disarm();
    pick(picked);
  }

  // displayedPoint is the pointer on the <img>, in its displayed pixels, clamped to it.
  function displayedPoint(event) {
    const bounds = frameImage.getBoundingClientRect();
    return {
      x: Math.min(Math.max(event.clientX - bounds.left, 0), bounds.width),
      y: Math.min(Math.max(event.clientY - bounds.top, 0), bounds.height),
    };
  }

  function drawDrag() {
    const left = Math.min(drag.from.x, drag.to.x);
    const top = Math.min(drag.from.y, drag.to.y);
    place(view.drag, left, top, Math.abs(drag.to.x - drag.from.x), Math.abs(drag.to.y - drag.from.y));
  }

  // --- the pick ---

  // pick posts the drag's corners in output-frame pixels, converted from displayed pixels by the
  // <img>'s natural/displayed size ratio, with the output time of the frame it shows.
  async function pick(picked) {
    const ms = tune.frameTime();
    const ratio = naturalPerDisplayed();
    if (ms === null || ratio === null) {
      report("No frame is shown to pick from.", true);
      return;
    }
    const body = { t: ms, from: outputPoint(picked.from, ratio), to: outputPoint(picked.to, ratio) };
    report("Picking…", false);
    try {
      const response = await fetch(PICK_URL, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      const answer = await response.json();
      if (!response.ok) {
        throw new Error(answer.error || response.statusText);
      }
      showProposal(answer, ms);
      report("", false);
    } catch (error) {
      clearProposal();
      report(error.message, true);
    }
  }

  // naturalPerDisplayed is how many output-frame pixels one displayed pixel of the <img> spans,
  // or null while it shows nothing.
  function naturalPerDisplayed() {
    const displayed = frameImage.getBoundingClientRect().width;
    if (displayed <= 0 || frameImage.naturalWidth <= 0) {
      return null;
    }
    return frameImage.naturalWidth / displayed;
  }

  function outputPoint(point, ratio) {
    return { x: Math.floor(point.x * ratio), y: Math.floor(point.y * ratio) };
  }

  // --- the proposal ---

  function showProposal(answer, ms) {
    proposal = answer;
    proposalMs = ms;
    view.target.textContent = answer.snippets[SNIPPET_ZOOM];
    view.result.hidden = false;
    drawBox();
  }

  function clearProposal() {
    proposal = null;
    proposalMs = null;
    view.result.hidden = true;
    view.box.hidden = true;
  }

  // drawBox draws the proposal's rect, in output-frame pixels, scaled to the displayed <img> by
  // its displayed/natural size ratio; it is hidden once the preview shows another frame.
  function drawBox() {
    const ratio = naturalPerDisplayed();
    if (!proposal || ratio === null || tune.frameTime() !== proposalMs) {
      view.box.hidden = true;
      return;
    }
    const rect = proposal.rect;
    place(view.box, rect.x / ratio, rect.y / ratio, rect.w / ratio, rect.h / ratio);
  }

  // setAsPanTarget sets the proposal as where the selected pan move goes, in place of its target
  // or cell; the server refuses it on a pan along a via path.
  function setAsPanTarget() {
    const move = selectedPan();
    if (!proposal || !move) {
      report("Select a pan move first.", true);
      return;
    }
    const change = {
      owner: { kind: ENTRY_CAMERA, index: move.index },
      field: FIELD_CAMERA_TO,
      value: proposal.target,
    };
    tune.edit(change).then(
      () => report("Pan target set on camera " + move.index, false),
      (error) => report(error.message, true),
    );
  }

  // selectedPan is the pan move the inspector shows, or undefined when it shows anything else.
  function selectedPan() {
    const entry = tune.selectedEntry();
    const state = tune.state();
    if (!entry || !state || entry.kind !== ENTRY_CAMERA) {
      return undefined;
    }
    return state.camera.find((move) => move.index === entry.index && move.kind === KIND_PAN);
  }

  // offerPan shows the pan target button only while the storyboard has a pan move to aim.
  function offerPan(state) {
    view.setPan.hidden = !state.camera.some((move) => move.kind === KIND_PAN);
  }

  function copySnippet(key) {
    if (!proposal) {
      return;
    }
    navigator.clipboard.writeText(proposal.snippets[key]).then(
      () => report("Copied the " + key + " YAML", false),
      (error) => report("Copy failed: " + error.message, true),
    );
  }

  // --- helpers ---

  // place puts node over the <img> at left, top with width and height, all in displayed pixels.
  function place(node, left, top, width, height) {
    node.style.left = frameImage.offsetLeft + left + "px";
    node.style.top = frameImage.offsetTop + top + "px";
    node.style.width = width + "px";
    node.style.height = height + "px";
    node.hidden = false;
  }

  function report(message, isFailure) {
    view.status.textContent = message;
    view.status.classList.toggle("failed", isFailure);
  }

  function onKey(event) {
    if (isArmed && event.key === ESCAPE_KEY) {
      disarm();
      report("", false);
      event.preventDefault();
    }
  }

  function wire() {
    view.arm.addEventListener("click", toggleArmed);
    view.stage.addEventListener("pointerdown", onPointerDown);
    view.stage.addEventListener("pointermove", onPointerMove);
    view.stage.addEventListener("pointerup", onPointerUp);
    view.stage.addEventListener("pointercancel", disarm);
    // app.js takes the new frame's time in its own load handler, which may run after this one.
    frameImage.addEventListener("load", () => requestAnimationFrame(drawBox));
    window.addEventListener("resize", drawBox);
    // fit.js resizes the <img> on more than a window resize.
    new ResizeObserver(drawBox).observe(frameImage);
    document.addEventListener("keydown", onKey);
    view.setPan.addEventListener("click", setAsPanTarget);
    tune.onState(offerPan);
    for (const button of view.copies) {
      button.addEventListener("click", () => copySnippet(button.dataset.snippet));
    }
  }

  // pickProposal is the standing proposal's target, as a camera.to or waypoint edit takes it, or
  // null while none stands.
  function pickProposal() {
    return proposal ? proposal.target : null;
  }

  Object.assign(window.tunePage, { pickProposal, reportPick: report });
  wire();
})();
