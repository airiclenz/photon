// Package caption draws a storyboard's captions over the shipped frame. A caption sits in output
// space: it is drawn after the camera's crop and the scale to the shipped width, so a zoom or a
// camera position never moves or scales it.
//
// [New] does all the work once, from a storyboard [storyboard.Parse] returned, whose captions carry
// their start and end on the clip's output clock and their colours already resolved. It draws each
// caption's text from the caption faces at an em of its font size × the design scale (2) × the
// compositor's unit — the shipped em of the terminal's text at the same font size, so a caption at
// frame.font_size matches the terminal text's size — and sizes a box around it, padded in design
// pixels. The box is placed a margin in from the frame's edges by place and align and moved by
// offset, or set at position, then clamped inside the frame; a box larger than the frame is an
// error. Lines align within the box as the caption's align says, left when it has a position. The
// text is rasterized to a coverage mask there and then, because an opentype face is not safe for
// concurrent use; [Layer.Draw] then only composites, the box at its opacity × the fade and the text
// at the fade, which rises linearly over the fade-in and falls linearly over the fade-out. A
// [Layer] never changes after New, so one is safe for concurrent use.
//
// It imports storyboard and fonts. golang.org/x/image/font measures and draws the text.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - caption.go: [New], the [Layer] and [Layer.Draw], each caption's fade over the start and
//     stop it carries, and its colours as opaque RGBA.
//   - layout.go: measuring a caption's lines, sizing, placing and clamping its box, and
//     rasterizing its text.
package caption
