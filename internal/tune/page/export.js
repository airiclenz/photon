// photon tune's GIF export: the transport bar's Export GIF button. It asks the session to render
// the saved storyboard's GIF to its configured output, shows the render's progress on the button
// while it runs, and reports in the transport bar's status the GIF written or why it failed.
//
// It reads the export through app.js's window.tunePage: app.js hands renderExport every state it
// takes and every state its poll answers, the export's progress included, which moves the export
// alone and never the state's version, so the poll's redraw and the frame cache stay as they were.
"use strict";

(() => {
  const page = window.tunePage;
  const API_EXPORT = "/api/export";
  const PERCENT = 100;
  const LABEL = "Export GIF";

  const button = document.getElementById("export");

  // isAwaiting says the page waits for an export to finish: one it started, or one it saw running.
  let isAwaiting = false;

  // startExport asks the session to render the GIF and takes in the state it answers.
  function startExport() {
    page.request(API_EXPORT, {}).then(
      (state) => {
        isAwaiting = true;
        page.reportStatus("Exporting the GIF", false);
        page.takeState(state);
      },
      (error) => page.reportStatus(error.message, true),
    );
  }

  // renderExport draws state's export on the button and, once an awaited export has finished,
  // reports in the status the GIF it wrote or why it failed.
  function renderExport(state) {
    const job = state.export;
    button.disabled = job.running || state.dirty || state.stale || state.errors.length > 0;
    button.title = exportTitle(state);
    button.textContent = job.running ? "Exporting " + percent(job) + "%" : LABEL;
    if (job.running) {
      isAwaiting = true;
      return;
    }
    if (isAwaiting) {
      isAwaiting = false;
      reportOutcome(job);
    }
  }

  // percent is how much of job's render has streamed, in whole percent.
  function percent(job) {
    return job.total > 0 ? Math.floor((job.done * PERCENT) / job.total) : 0;
  }

  // exportTitle says what the button does, or why it is disabled.
  function exportTitle(state) {
    if (state.export.running) {
      return "An export is running";
    }
    if (state.dirty) {
      return "Export renders the storyboard on disk: save the edits first";
    }
    if (state.stale) {
      return "The storyboard changed on disk: reload it first";
    }
    if (state.errors.length > 0) {
      return "Export refuses while an error stands";
    }
    return "Render the GIF to the storyboard's output";
  }

  // reportOutcome reports how job ended: the GIF it wrote, or why it failed.
  function reportOutcome(job) {
    if (job.error) {
      page.reportStatus("Export failed: " + job.error, true);
    } else {
      page.reportStatus("Exported " + job.out, false);
    }
  }

  button.addEventListener("click", startExport);
  Object.assign(page, { renderExport });
})();
