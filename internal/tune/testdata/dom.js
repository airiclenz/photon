// A minimal DOM for driving photon tune's page scripts under goja in the Go tests: elements made
// on demand by id, events a test dispatches by hand, and timers, animation frames and image loads
// queued until the test runs them. Nothing here lays anything out: an element's box is the
// layout the test sets on it, the window's height is the innerHeight it sets, and a resize
// observer hears of an element's resize only when the test says it resized. fetch answers through testHost.serve, which the Go harness binds to
// the tune handler, and testDOM is what the harness drives the page with.
"use strict";

(() => {
  const host = globalThis.testHost;
  const BLOB_PREFIX = "blob:tune/";
  const TEXT_NODE = 3;
  const ELEMENT_NODE = 1;
  const HTTP_OK_FIRST = 200;
  const HTTP_OK_LAST = 299;
  const WINDOW_HEIGHT = 768;

  // tasks run one at a time by testDOM.runTask: timeouts and image loads, in queue order.
  const tasks = [];
  // frames run together by testDOM.runFrame: the animation frame callbacks queued before it.
  const frames = [];
  // intervals run only when testDOM.runIntervals asks, so a poll never runs on its own.
  const intervals = new Map();
  const blobs = new Map();
  // observers are the live resize observers, each with the elements it observes.
  const observers = new Set();
  const byId = new Map();
  const listenersOf = new WeakMap();
  let nextHandle = 1;
  let nextBlob = 1;
  // heldAnswers deliver the answers of the fetches held under heldPrefix, in the order they went
  // out; null holds none.
  const heldAnswers = [];
  let heldPrefix = null;
  let clock = 0;

  // --- events ---

  class Event {
    constructor(type, init = {}) {
      Object.assign(this, { bubbles: true, button: 0, clientX: 0, clientY: 0, pointerId: 1 }, init);
      this.type = type;
      this.target = null;
      this.currentTarget = null;
      this.defaultPrevented = false;
      this.isPropagationStopped = false;
    }

    preventDefault() {
      this.defaultPrevented = true;
    }

    stopPropagation() {
      this.isPropagationStopped = true;
    }
  }

  class EventTarget {
    addEventListener(type, listener) {
      const all = listenersOf.get(this) || new Map();
      listenersOf.set(this, all);
      const ofType = all.get(type) || [];
      all.set(type, ofType);
      if (!ofType.includes(listener)) {
        ofType.push(listener);
      }
    }

    removeEventListener(type, listener) {
      const ofType = (listenersOf.get(this) || new Map()).get(type) || [];
      const at = ofType.indexOf(listener);
      if (at >= 0) {
        ofType.splice(at, 1);
      }
    }

    // dispatchEvent calls the listeners on this node, then on each parent while the event
    // bubbles; a listener that throws fails the dispatch, so the test sees it.
    dispatchEvent(event) {
      event.target = event.target || this;
      for (let node = this; node; node = event.bubbles ? node.parentNode : null) {
        event.currentTarget = node;
        const ofType = (listenersOf.get(node) || new Map()).get(event.type) || [];
        for (const listener of [...ofType]) {
          listener.call(node, event);
        }
        if (event.isPropagationStopped) {
          break;
        }
      }
      return !event.defaultPrevented;
    }
  }

  // --- nodes ---

  class Text {
    constructor(data) {
      this.nodeType = TEXT_NODE;
      this.data = String(data);
      this.parentNode = null;
    }

    get textContent() {
      return this.data;
    }
  }

  class ClassList {
    constructor() {
      this.names = new Set();
    }

    add(...names) {
      names.forEach((name) => this.names.add(name));
    }

    remove(...names) {
      names.forEach((name) => this.names.delete(name));
    }

    contains(name) {
      return this.names.has(name);
    }

    // toggle adds name, or removes it, as force says; without force it flips it.
    toggle(name, force) {
      const isOn = force === undefined ? !this.names.has(name) : Boolean(force);
      if (isOn) {
        this.names.add(name);
      } else {
        this.names.delete(name);
      }
      return isOn;
    }

    get value() {
      return [...this.names].join(" ");
    }
  }

  class Element extends EventTarget {
    constructor(tag) {
      super();
      this.nodeType = ELEMENT_NODE;
      this.tagName = tag.toUpperCase();
      this.id = "";
      this.childNodes = [];
      this.parentNode = null;
      this.attributes = new Map();
      this.dataset = {};
      this.style = {};
      this.classList = new ClassList();
      this.hidden = false;
      this.disabled = false;
      this.title = "";
      // layout is the box the test lays the element out at, in displayed pixels.
      this.layout = { left: 0, top: 0, width: 0, height: 0 };
    }

    get className() {
      return this.classList.value;
    }

    set className(names) {
      this.classList = new ClassList();
      this.classList.add(...String(names).split(/\s+/).filter(Boolean));
    }

    get children() {
      return this.childNodes.filter((node) => node.nodeType === ELEMENT_NODE);
    }

    get textContent() {
      return this.childNodes.map((node) => node.textContent).join("");
    }

    set textContent(text) {
      this.replaceChildren(String(text));
    }

    append(...nodes) {
      for (const each of nodes) {
        const node = typeof each === "object" && each !== null ? each : new Text(each);
        if (node.parentNode) {
          node.parentNode.removeChild(node);
        }
        node.parentNode = this;
        this.childNodes.push(node);
      }
    }

    replaceChildren(...nodes) {
      for (const node of this.childNodes) {
        node.parentNode = null;
      }
      this.childNodes = [];
      this.append(...nodes);
    }

    removeChild(node) {
      const at = this.childNodes.indexOf(node);
      if (at >= 0) {
        this.childNodes.splice(at, 1);
        node.parentNode = null;
      }
      return node;
    }

    remove() {
      if (this.parentNode) {
        this.parentNode.removeChild(this);
      }
    }

    setAttribute(name, value) {
      const text = String(value);
      this.attributes.set(name, text);
      if (name === "id") {
        this.id = text;
      } else if (name.startsWith("data-")) {
        this.dataset[datasetKey(name)] = text;
      }
    }

    getAttribute(name) {
      if (name.startsWith("data-")) {
        const value = this.dataset[datasetKey(name)];
        return value === undefined ? null : value;
      }
      return this.attributes.has(name) ? this.attributes.get(name) : null;
    }

    hasAttribute(name) {
      return this.getAttribute(name) !== null;
    }

    querySelectorAll(selector) {
      return queryAll(this, selector);
    }

    getBoundingClientRect() {
      const { left, top, width, height } = this.layout;
      return { left, top, width, height, x: left, y: top, right: left + width, bottom: top + height };
    }

    get clientWidth() {
      return this.layout.width;
    }

    get clientHeight() {
      return this.layout.height;
    }

    get offsetLeft() {
      return this.layout.left;
    }

    get offsetTop() {
      return this.layout.top;
    }

    setPointerCapture() {}

    releasePointerCapture() {}

    focus() {}

    blur() {}
  }

  class HTMLElement extends Element {}
  class HTMLInputElement extends HTMLElement {}
  class HTMLSelectElement extends HTMLElement {}
  class HTMLTextAreaElement extends HTMLElement {}

  // HTMLImageElement loads what its src names in a queued task: a blob URL of a decodable image
  // fires onload and load with naturalWidth and naturalHeight set to the image's, anything else
  // onerror. A src set again before the task runs drops the earlier load, as a browser does.
  class HTMLImageElement extends HTMLElement {
    constructor(tag) {
      super(tag);
      this.naturalWidth = 0;
      this.naturalHeight = 0;
      this.onload = null;
      this.onerror = null;
      this.source = "";
    }

    get src() {
      return this.source;
    }

    set src(address) {
      const source = String(address);
      this.source = source;
      const blob = blobs.get(source);
      queueTask(() => {
        if (this.source !== source) {
          return;
        }
        if (!blob || !(blob.width > 0)) {
          this.fire("error", this.onerror);
          return;
        }
        this.naturalWidth = blob.width;
        this.naturalHeight = blob.height;
        this.fire("load", this.onload);
      });
    }

    fire(type, handler) {
      const event = new Event(type, { bubbles: false });
      if (typeof handler === "function") {
        handler.call(this, event);
      }
      this.dispatchEvent(event);
    }
  }

  const elementClasses = {
    INPUT: HTMLInputElement,
    SELECT: HTMLSelectElement,
    TEXTAREA: HTMLTextAreaElement,
    IMG: HTMLImageElement,
  };

  function createElement(tag) {
    const Kind = elementClasses[tag.toUpperCase()] || HTMLElement;
    return new Kind(tag);
  }

  // datasetKey is the dataset key of a data- attribute: data-drag-from is dragFrom.
  function datasetKey(name) {
    return name.slice("data-".length).replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
  }

  // --- selectors: descendant chains of #id, .class, [attribute] and tag ---

  function queryAll(root, selector) {
    const chain = selector.trim().split(/\s+/).map(compoundOf);
    return descendants(root).filter((node) => matchesChain(node, chain, root));
  }

  function descendants(root) {
    const found = [];
    for (const child of root.children) {
      found.push(child, ...descendants(child));
    }
    return found;
  }

  // matchesChain says node matches the chain's last compound and its ancestors under root the
  // rest, in order.
  function matchesChain(node, chain, root) {
    if (!matchesCompound(node, chain[chain.length - 1])) {
      return false;
    }
    let rest = chain.length - 2;
    for (let up = node.parentNode; rest >= 0 && up && up !== root; up = up.parentNode) {
      if (matchesCompound(up, chain[rest])) {
        rest--;
      }
    }
    return rest < 0;
  }

  function compoundOf(text) {
    const parts = text.match(/#[\w-]+|\.[\w-]+|\[[\w-]+\]|^[\w-]+/g);
    if (!parts || parts.join("") !== text) {
      throw new Error("dom.js: unsupported selector " + text);
    }
    return parts;
  }

  function matchesCompound(node, parts) {
    return parts.every((part) => {
      if (part.startsWith("#")) {
        return node.id === part.slice(1);
      }
      if (part.startsWith(".")) {
        return node.classList.contains(part.slice(1));
      }
      if (part.startsWith("[")) {
        return node.hasAttribute(part.slice(1, -1));
      }
      return node.tagName === part.toUpperCase();
    });
  }

  // --- the document and the window ---

  const body = createElement("body");

  class Document extends EventTarget {
    get body() {
      return body;
    }

    createElement(tag) {
      return createElement(tag);
    }

    // getElementById is the element of id, made on demand as a div under the body unless
    // testDOM.declare named its tag first.
    getElementById(id) {
      if (!byId.has(id)) {
        declare(id, "div");
      }
      return byId.get(id);
    }

    querySelectorAll(selector) {
      return queryAll(body, selector);
    }
  }

  const document = new Document();

  function declare(id, tag) {
    const node = createElement(tag);
    node.id = id;
    body.append(node);
    byId.set(id, node);
  }

  const windowEvents = new EventTarget();

  // --- the queues ---

  function queueTask(callback) {
    tasks.push(callback);
  }

  function setTimeout(callback, ms, ...args) {
    const handle = nextHandle++;
    tasks.push(Object.assign(() => callback(...args), { handle }));
    return handle;
  }

  function clearTimeout(handle) {
    const at = tasks.findIndex((task) => task.handle === handle);
    if (at >= 0) {
      tasks.splice(at, 1);
    }
  }

  function setInterval(callback, ms, ...args) {
    const handle = nextHandle++;
    intervals.set(handle, () => callback(...args));
    return handle;
  }

  function clearInterval(handle) {
    intervals.delete(handle);
  }

  function requestAnimationFrame(callback) {
    const handle = nextHandle++;
    frames.push({ handle, callback });
    return handle;
  }

  function cancelAnimationFrame(handle) {
    const at = frames.findIndex((frame) => frame.handle === handle);
    if (at >= 0) {
      frames.splice(at, 1);
    }
  }

  // ResizeObserver calls back when testDOM.resized names an element it observes.
  class ResizeObserver {
    constructor(callback) {
      this.callback = callback;
      this.observed = new Set();
      observers.add(this);
    }

    observe(node) {
      this.observed.add(node);
    }

    unobserve(node) {
      this.observed.delete(node);
    }

    disconnect() {
      this.observed.clear();
      observers.delete(this);
    }
  }

  // --- fetch, blobs and the clipboard ---

  // Response is a fetch's answer from the harness: status, statusText, and the body as text for
  // json() or, for an image, its size for blob().
  class Response {
    constructor(answer) {
      this.status = answer.status;
      this.statusText = answer.statusText;
      this.ok = answer.status >= HTTP_OK_FIRST && answer.status <= HTTP_OK_LAST;
      this.answer = answer;
    }

    json() {
      return Promise.resolve().then(() => JSON.parse(this.answer.body));
    }

    blob() {
      const { contentType, imageWidth, imageHeight, size } = this.answer;
      return Promise.resolve({ type: contentType, size, width: imageWidth, height: imageHeight });
    }
  }

  // fetch serves the request at once; its answer arrives at once too, unless its resource starts
  // with the prefix testDOM.holdFetches holds, when it waits for testDOM.releaseFetches.
  function fetch(resource, options = {}) {
    const headers = options.headers || {};
    const method = options.method || "GET";
    const address = String(resource);
    const answer = host.serve(method, address, headers["Content-Type"] || "", options.body || "");
    if (heldPrefix === null || !address.startsWith(heldPrefix)) {
      return Promise.resolve(new Response(answer));
    }
    return new Promise((resolve) => {
      heldAnswers.push(() => resolve(new Response(answer)));
    });
  }

  const URL = {
    createObjectURL(blob) {
      const address = BLOB_PREFIX + nextBlob++;
      blobs.set(address, blob);
      return address;
    },
    revokeObjectURL(address) {
      blobs.delete(address);
    },
  };

  const clipboard = {
    written: [],
    writeText(text) {
      clipboard.written.push(String(text));
      return Promise.resolve();
    },
  };

  Object.assign(globalThis, {
    window: globalThis,
    document,
    Event,
    HTMLElement,
    HTMLInputElement,
    HTMLSelectElement,
    HTMLTextAreaElement,
    HTMLImageElement,
    ResizeObserver,
    URL,
    fetch,
    setTimeout,
    clearTimeout,
    setInterval,
    clearInterval,
    requestAnimationFrame,
    cancelAnimationFrame,
    navigator: { clipboard },
    performance: { now: () => clock },
    innerHeight: WINDOW_HEIGHT,
    addEventListener: (type, listener) => windowEvents.addEventListener(type, listener),
    removeEventListener: (type, listener) => windowEvents.removeEventListener(type, listener),
    dispatchEvent: (event) => windowEvents.dispatchEvent(event),
  });

  // testDOM is how the harness drives the page: it declares the page's elements, dispatches
  // events on them by id ("document" for the document, "window" for the window), tells the resize
  // observers of an element's resize, and runs the queues.
  globalThis.testDOM = Object.freeze({
    declare,
    dispatch(id, type, init) {
      const targets = { document, window: globalThis };
      const target = targets[id] || document.getElementById(id);
      return target.dispatchEvent(new Event(type, init || {}));
    },
    // resized calls back every resize observer that observes the element of id.
    resized(id) {
      const node = document.getElementById(id);
      for (const observer of [...observers]) {
        if (observer.observed.has(node)) {
          observer.callback([{ target: node }], observer);
        }
      }
    },
    // runTask runs the first queued task and says whether there was one.
    runTask() {
      const task = tasks.shift();
      if (task) {
        task();
      }
      return Boolean(task);
    },
    // runFrame runs the animation frame callbacks queued before it; those they queue wait.
    runFrame() {
      for (const frame of frames.splice(0)) {
        frame.callback(clock);
      }
    },
    runIntervals() {
      for (const callback of [...intervals.values()]) {
        callback();
      }
    },
    hasTasks: () => tasks.length > 0,
    // holdFetches holds the answers of the fetches whose resource starts with prefix from now on.
    holdFetches(prefix) {
      heldPrefix = String(prefix);
    },
    // releaseFetches delivers the first count held answers, all of them without a count; the
    // fetches that go out afterwards are held still.
    releaseFetches(count) {
      const released = heldAnswers.splice(0, count === undefined ? heldAnswers.length : count);
      for (const deliver of released) {
        deliver();
      }
    },
    heldFetches: () => heldAnswers.length,
    advance(ms) {
      clock += ms;
    },
  });
})();
