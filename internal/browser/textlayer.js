// The page's text layer and selector boxes, read at one moment. This file is one function
// expression: photon calls it with the list of selectors to box and reads back
// {nodes, selectors}, all positions in CSS pixels from the viewport's top left.
//
// nodes holds every visible text node under <body>, in document order, as its graphemes: each
// grapheme's text and its rect {x, y, w, h}, or a null rect when it is not rendered (collapsed
// whitespace) or lies wholly outside the viewport. A text node whose element is not
// visibility: visible, or which renders nothing, is left out.
//
// selectors holds, in the order asked, either {boxes} — the rect of every element the selector
// matches that is visible, has an area and touches the viewport, in document order — or {error},
// the page's message when it refuses the selector.
(selectors) => {
  const viewportWidth = window.innerWidth;
  const viewportHeight = window.innerHeight;
  const segmenter = new Intl.Segmenter(undefined, {granularity: 'grapheme'});
  const range = document.createRange();

  const touchesViewport = (rect) =>
    rect.right > 0 && rect.bottom > 0 && rect.left < viewportWidth && rect.top < viewportHeight;

  const rectOf = (rect) => ({x: rect.left, y: rect.top, w: rect.width, h: rect.height});

  const isVisible = (element) =>
    element !== null && getComputedStyle(element).visibility === 'visible';

  const rendersAnything = (node) => {
    range.selectNodeContents(node);
    return range.getClientRects().length > 0;
  };

  const glyphsOf = (node) => {
    const glyphs = [];
    for (const {segment, index} of segmenter.segment(node.data)) {
      range.setStart(node, index);
      range.setEnd(node, index + segment.length);
      const box = range.getBoundingClientRect();
      const isShown = range.getClientRects().length > 0 && touchesViewport(box);
      glyphs.push({t: segment, r: isShown ? rectOf(box) : null});
    }
    return glyphs;
  };

  const nodes = [];
  if (document.body !== null) {
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node !== null; node = walker.nextNode()) {
      if (isVisible(node.parentElement) && rendersAnything(node)) {
        nodes.push(glyphsOf(node));
      }
    }
  }

  // Each selector is queried in its own try, so one the page refuses names itself and leaves
  // the others boxed.
  const boxesOf = (selector) => {
    try {
      const boxes = [];
      for (const element of document.querySelectorAll(selector)) {
        const box = element.getBoundingClientRect();
        if (box.width > 0 && box.height > 0 && isVisible(element) && touchesViewport(box)) {
          boxes.push(rectOf(box));
        }
      }
      return {boxes};
    } catch (error) {
      return {error: String(error && error.message ? error.message : error)};
    }
  };

  return {nodes, selectors: selectors.map(boxesOf)};
}
