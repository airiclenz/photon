package pixels

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/take"
)

// kindKey, kindPatch and kindNone name what Encode returned for a frame in the cadence tests.
const (
	kindKey   = "key"
	kindPatch = "patch"
	kindNone  = "none"
)

func TestRoundTrip_EveryIndexInAnyOrder(t *testing.T) {
	// The frame is wide and tall enough for two changes patchGap apart.
	const width, height, frames = 96, 72, 60
	rng := rand.New(rand.NewPCG(7, 11))
	// The frame's bounds start off the origin, as a sub-image's do.
	frame := image.NewRGBA(image.Rect(3, 2, 3+width, 2+height))
	var encoder Encoder
	recorded := &take.Take{Cols: width, Rows: height, FPS: 10, Cell: take.CellSize{W: 1, H: 1}}
	var want []*image.RGBA
	split := 0
	for index := range frames {
		at := time.Duration(index) * 100 * time.Millisecond
		paintStep(frame, index, rng)
		entry, err := encoder.Encode(at, frame)
		if err != nil {
			t.Fatalf("Encode(frame %d): %v", index, err)
		}
		if entry != nil && len(entry.Patches) > 1 {
			split++
		}
		// A frame without an entry keeps its snapshot here, so the decoder's reading of one is
		// covered too.
		recorded.Snapshots = append(recorded.Snapshots, take.Snapshot{At: at, Image: entry})
		want = append(want, clone(frame))
	}
	if split == 0 {
		t.Fatal("no entry has more than one patch, so the round trip covers none")
	}

	var file bytes.Buffer
	if err := take.Write(&file, recorded); err != nil {
		t.Fatalf("Write: %v", err)
	}
	reread, err := take.Read(&file)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	forward := make([]int, frames)
	backward := make([]int, frames)
	for index := range frames {
		forward[index], backward[index] = index, frames-1-index
	}
	orders := map[string][]int{
		"in order":        forward,
		"in reverse":      backward,
		"in random order": rand.New(rand.NewPCG(3, 5)).Perm(frames),
	}
	for name, order := range orders {
		t.Run(name, func(t *testing.T) {
			decoder := NewDecoder(reread)
			for _, index := range order {
				got, err := decoder.Frame(index)
				if err != nil {
					t.Fatalf("Frame(%d): %v", index, err)
				}
				if !sameFrame(got, want[index]) {
					t.Fatalf("Frame(%d) differs from the frame encoded", index)
				}
			}
		})
	}
}

// paintStep changes frame for step index of the round-trip test: some steps change nothing, some
// paint one to three small rectangles (opaque or translucent) anywhere in the frame, and some
// repaint most of the frame.
func paintStep(frame *image.RGBA, index int, rng *rand.Rand) {
	bounds := frame.Bounds()
	switch {
	case index%7 == 3:
		return
	case index%11 == 5:
		fill(frame, image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y-2),
			randomColor(rng))
	default:
		for range 1 + index%3 {
			x := bounds.Min.X + rng.IntN(bounds.Dx()-4)
			y := bounds.Min.Y + rng.IntN(bounds.Dy()-4)
			fill(frame, image.Rect(x, y, x+1+rng.IntN(4), y+1+rng.IntN(4)), randomColor(rng))
		}
	}
}

// randomColor is an RGBA colour, opaque half the time and translucent otherwise, with the
// premultiplied channels at most its alpha.
func randomColor(rng *rand.Rand) color.RGBA {
	alpha := uint8(255)
	if rng.IntN(2) == 0 {
		alpha = uint8(rng.IntN(255))
	}
	channel := func() uint8 { return uint8(rng.IntN(int(alpha) + 1)) }
	return color.RGBA{R: channel(), G: channel(), B: channel(), A: alpha}
}

func TestEncoder_KeyFrameCadence(t *testing.T) {
	const width, height = 40, 20 // half the frame is 400 pixels
	type step struct {
		at    time.Duration
		size  image.Point     // the frame's new size; zero keeps it
		paint image.Rectangle // the pixels changed; empty changes none
		want  string
		rect  take.Box // the patch's rectangle, for a patch
	}
	full := take.Box{W: width, H: height}
	small := image.Rect(3, 4, 7, 6)
	tests := []struct {
		name  string
		steps []step
	}{
		{"the first frame is a key frame", []step{
			{at: 0, want: kindKey, rect: full},
		}},
		{"a small change is a patch of the rectangle that changed", []step{
			{at: 0, want: kindKey, rect: full},
			{at: 100 * time.Millisecond, paint: small, want: kindPatch, rect: take.Box{X: 3, Y: 4, W: 4, H: 2}},
		}},
		{"an unchanged frame gets no entry", []step{
			{at: 0, want: kindKey, rect: full},
			{at: 100 * time.Millisecond, want: kindNone},
		}},
		{"a change two seconds after the key frame is a key frame", []step{
			{at: 0, want: kindKey, rect: full},
			{at: 1999 * time.Millisecond, paint: small, want: kindPatch, rect: take.Box{X: 3, Y: 4, W: 4, H: 2}},
			{at: 2 * time.Second, paint: small, want: kindKey, rect: full},
			{at: 3 * time.Second, paint: small, want: kindPatch, rect: take.Box{X: 3, Y: 4, W: 4, H: 2}},
			{at: 4 * time.Second, paint: small, want: kindKey, rect: full},
		}},
		{"an unchanged frame past two seconds still gets no entry", []step{
			{at: 0, want: kindKey, rect: full},
			{at: 2 * time.Second, want: kindNone},
			{at: 2100 * time.Millisecond, paint: small, want: kindKey, rect: full},
		}},
		{"a change over more than half the frame is a key frame", []step{
			{at: 0, want: kindKey, rect: full},
			{at: 100 * time.Millisecond, paint: image.Rect(0, 0, 21, 20), want: kindKey, rect: full},
		}},
		{"a change over exactly half the frame is a patch", []step{
			{at: 0, want: kindKey, rect: full},
			{at: 100 * time.Millisecond, paint: image.Rect(10, 0, 30, 20), want: kindPatch, rect: take.Box{X: 10, W: 20, H: 20}},
		}},
		{"a new frame size is a key frame", []step{
			{at: 0, want: kindKey, rect: full},
			{at: 100 * time.Millisecond, size: image.Pt(30, 10), want: kindKey, rect: take.Box{W: 30, H: 10}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var encoder Encoder
			frame := image.NewRGBA(image.Rect(0, 0, width, height))
			for index, s := range tt.steps {
				if s.size != (image.Point{}) {
					frame = image.NewRGBA(image.Rectangle{Max: s.size})
				}
				fill(frame, s.paint, color.RGBA{R: uint8(40 * index), G: 200, B: 9, A: 255})
				entry, err := encoder.Encode(s.at, frame)
				if err != nil {
					t.Fatalf("step %d: Encode: %v", index, err)
				}
				got := kindOf(entry)
				if got != s.want {
					t.Fatalf("step %d: Encode gave a %s entry, want %s", index, got, s.want)
				}
				if entry != nil && !slices.Equal(rects(entry), []take.Box{s.rect}) {
					t.Errorf("step %d: entry rectangles %+v, want [%+v]", index, rects(entry), s.rect)
				}
			}
		})
	}
}

func TestEncoder_FarApartChangesAreSeparatePatches(t *testing.T) {
	const width, height = 200, 120 // half the frame is 12000 pixels
	tests := []struct {
		name  string
		paint []image.Rectangle
		want  string
		rects []take.Box
	}{
		{"changes far apart in rows and columns are one patch each",
			[]image.Rectangle{image.Rect(2, 2, 6, 5), image.Rect(180, 100, 190, 110)},
			kindPatch, []take.Box{{X: 2, Y: 2, W: 4, H: 3}, {X: 180, Y: 100, W: 10, H: 10}}},
		{"changes far apart in the same rows are one patch each",
			[]image.Rectangle{image.Rect(2, 10, 6, 14), image.Rect(100, 12, 104, 20)},
			kindPatch, []take.Box{{X: 2, Y: 10, W: 4, H: 4}, {X: 100, Y: 12, W: 4, H: 8}}},
		{"changes patchGap columns apart are one patch each",
			[]image.Rectangle{image.Rect(0, 0, 4, 4), image.Rect(4+patchGap, 0, 8+patchGap, 4)},
			kindPatch, []take.Box{{W: 4, H: 4}, {X: 4 + patchGap, W: 4, H: 4}}},
		{"changes patchGap rows apart are one patch each",
			[]image.Rectangle{image.Rect(0, 0, 4, 4), image.Rect(0, 4+patchGap, 4, 8+patchGap)},
			kindPatch, []take.Box{{W: 4, H: 4}, {Y: 4 + patchGap, W: 4, H: 4}}},
		{"changes closer than patchGap share one patch",
			[]image.Rectangle{image.Rect(0, 0, 4, 4), image.Rect(3+patchGap, 3+patchGap, 7+patchGap, 7+patchGap)},
			kindPatch, []take.Box{{W: 7 + patchGap, H: 7 + patchGap}}},
		{"far-apart changes over more than half the frame together are a key frame",
			[]image.Rectangle{image.Rect(0, 0, width, 40), image.Rect(0, 79, width, height)},
			kindKey, []take.Box{{W: width, H: height}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var encoder Encoder
			frame := image.NewRGBA(image.Rect(0, 0, width, height))
			fill(frame, frame.Rect, color.RGBA{R: 10, G: 20, B: 30, A: 255})
			if _, err := encoder.Encode(0, frame); err != nil {
				t.Fatalf("Encode(key): %v", err)
			}
			for _, rect := range tt.paint {
				fill(frame, rect, color.RGBA{R: 200, G: 120, B: 9, A: 255})
			}

			entry, err := encoder.Encode(100*time.Millisecond, frame)

			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if got := kindOf(entry); got != tt.want {
				t.Fatalf("Encode gave a %s entry, want %s", got, tt.want)
			}
			if !slices.Equal(rects(entry), tt.rects) {
				t.Errorf("entry rectangles %+v, want %+v", rects(entry), tt.rects)
			}
		})
	}
}

// rects returns the rectangles of entry's patches.
func rects(entry *take.ImageEntry) []take.Box {
	var boxes []take.Box
	for _, patch := range entry.Patches {
		boxes = append(boxes, patch.Rect)
	}
	return boxes
}

// kindOf names what entry is: a key frame, a patch or no entry.
func kindOf(entry *take.ImageEntry) string {
	switch {
	case entry == nil:
		return kindNone
	case entry.Key:
		return kindKey
	default:
		return kindPatch
	}
}

func TestDecoder_AppliesEveryPatchOfAnEntry(t *testing.T) {
	var encoder Encoder
	frame := image.NewRGBA(image.Rect(0, 0, 40, 20))
	fill(frame, frame.Rect, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	key, err := encoder.Encode(0, frame)
	if err != nil {
		t.Fatalf("Encode(key): %v", err)
	}
	// Two far-apart changes, encoded one frame apart, are joined into one entry of two patches.
	var patches []take.Patch
	for index, rect := range []image.Rectangle{image.Rect(2, 2, 5, 4), image.Rect(30, 14, 36, 18)} {
		fill(frame, rect, color.RGBA{R: 200, G: uint8(60 * index), B: 9, A: 255})
		entry, err := encoder.Encode(time.Duration(index+1)*100*time.Millisecond, frame)
		if err != nil || entry == nil || entry.Key {
			t.Fatalf("Encode(change %d) = %+v, %v, want a patch", index, entry, err)
		}
		patches = append(patches, entry.Patches...)
	}
	recorded := &take.Take{Cols: 40, Rows: 20, FPS: 10, Cell: take.CellSize{W: 1, H: 1},
		Snapshots: []take.Snapshot{
			{At: 0, Image: key},
			{At: 100 * time.Millisecond, Image: &take.ImageEntry{Patches: patches}},
		}}
	var file bytes.Buffer
	if err := take.Write(&file, recorded); err != nil {
		t.Fatalf("Write: %v", err)
	}
	reread, err := take.Read(&file)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := len(reread.Snapshots[1].Image.Patches); got != 2 {
		t.Fatalf("the reread entry has %d patches, want 2", got)
	}

	got, err := NewDecoder(reread).Frame(1)

	if err != nil {
		t.Fatalf("Frame(1): %v", err)
	}
	if !sameFrame(got, frame) {
		t.Fatal("Frame(1) differs from the frame both patches were encoded from")
	}
}

func TestEncoder_RefusesAnEmptyFrame(t *testing.T) {
	var encoder Encoder
	if _, err := encoder.Encode(0, image.NewRGBA(image.Rectangle{})); err == nil {
		t.Fatal("Encode(empty frame) succeeded, want an error")
	}
}

func TestDecoder_RefusesAMalformedLayer(t *testing.T) {
	var encoder Encoder
	frame := image.NewRGBA(image.Rect(0, 0, 4, 4))
	key, err := encoder.Encode(0, frame)
	if err != nil {
		t.Fatalf("Encode(key): %v", err)
	}
	fill(frame, image.Rect(1, 1, 3, 3), color.RGBA{R: 255, A: 255})
	patch, err := encoder.Encode(100*time.Millisecond, frame)
	if err != nil || patch == nil || patch.Key {
		t.Fatalf("Encode(patch) = %+v, %v, want a patch", patch, err)
	}
	withRect := func(entry *take.ImageEntry, rect take.Box) *take.ImageEntry {
		moved := *entry
		moved.Patches = []take.Patch{{Rect: rect, PNG: entry.Patches[0].PNG}}
		return &moved
	}
	withPatches := func(entry *take.ImageEntry, patches ...take.Patch) *take.ImageEntry {
		changed := *entry
		changed.Patches = patches
		return &changed
	}
	tests := []struct {
		name    string
		entries []*take.ImageEntry
	}{
		{"no key frame at or before the snapshot", []*take.ImageEntry{nil, patch}},
		{"a patch outside the frame", []*take.ImageEntry{key, withRect(patch, take.Box{X: 3, Y: 3, W: 2, H: 2})}},
		{"a patch the wrong size for its rectangle", []*take.ImageEntry{key, withRect(patch, take.Box{X: 1, Y: 1, W: 3, H: 2})}},
		{"a key frame off the origin", []*take.ImageEntry{withRect(key, take.Box{X: 1, W: 4, H: 4}), patch}},
		{"a second patch outside the frame", []*take.ImageEntry{key,
			withPatches(patch, patch.Patches[0], take.Patch{Rect: take.Box{X: 3, Y: 3, W: 2, H: 2}, PNG: patch.Patches[0].PNG})}},
		{"a key frame with no patch", []*take.ImageEntry{withPatches(key), patch}},
		{"a key frame with two patches", []*take.ImageEntry{withPatches(key, key.Patches[0], key.Patches[0]), patch}},
		{"bytes that are not a PNG", []*take.ImageEntry{{Key: true, Patches: []take.Patch{{Rect: take.Box{W: 4, H: 4}, PNG: []byte("not a png")}}}, patch}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorded := &take.Take{}
			for index, entry := range tt.entries {
				recorded.Snapshots = append(recorded.Snapshots,
					take.Snapshot{At: time.Duration(index) * time.Second, Image: entry})
			}
			_, err := NewDecoder(recorded).Frame(len(tt.entries) - 1)
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("Frame = %v, want an error wrapping ErrMalformed", err)
			}
		})
	}

	t.Run("a snapshot the take does not have", func(t *testing.T) {
		recorded := &take.Take{Snapshots: []take.Snapshot{{Image: key}}}
		for _, index := range []int{-1, 1} {
			if _, err := NewDecoder(recorded).Frame(index); err == nil {
				t.Errorf("Frame(%d) succeeded, want an error", index)
			}
		}
	})
}

// The synthetic browser take the size test records: a 1280×800 page filmed for a minute at 24
// frames a second, scrolled every 5 s, with text typed into a box and a side panel swapped every
// 15 s, and in one variant a spinner far from the typing box turning every frame.
const (
	pageWidth      = 1280
	pageHeight     = 800
	pageFPS        = 24
	pageSeconds    = 60
	pageBudget     = 25_000_000 // bytes: the most a one-minute browser take may take on disk
	pageLength     = 1600       // rows of the scrolling page, which wraps
	pageScrollStep = 160
	pageScrollFor  = 5 * pageFPS // frames between two scrolls
	pageSwapFor    = 15 * pageFPS
	pageTypeFor    = 5 // frames between two typed characters
	pageLineHeight = 22
	glyphWidth     = 8
	glyphHeight    = 14
	glyphCount     = 64
	spinnerDots    = 8
	spinnerRadius  = 13
	spinnerDot     = 6
)

// The page's layout: the scrolling text column, the side panel and the box text is typed into.
var (
	pageTextColumn = image.Rect(40, 0, 880, pageLength)
	pagePanel      = image.Rect(920, 80, 1240, 680)
	pageTypingBox  = image.Rect(40, 716, 880, 784)
	pageSpinner    = image.Rect(1200, 16, 1240, 56)
)

func TestEncode_OneMinuteBrowserTakeFitsTheBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("encodes a minute of 1280×800 frames")
	}
	for _, tt := range []struct {
		name    string
		spinner bool
	}{
		{"typing", false},
		{"typing plus a turning spinner", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			encodeOneMinuteTake(t, tt.spinner)
		})
	}
}

// encodeOneMinuteTake encodes the synthetic browser take, with the spinner when spinner is set,
// and fails t when it is over the budget on disk or its last frame does not decode back.
func encodeOneMinuteTake(t *testing.T, spinner bool) {
	t.Helper()
	rng := rand.New(rand.NewPCG(1, 2))
	glyphs := makeGlyphs(rng)
	page := makePage(rng, glyphs)
	panels := [2]*image.RGBA{makePanel(rng, glyphs, 0xe8f0ff), makePanel(rng, glyphs, 0xfff4e0)}
	typed := make([]int, 0, pageSeconds*pageFPS/pageTypeFor)

	frame := image.NewRGBA(image.Rect(0, 0, pageWidth, pageHeight))
	var encoder Encoder
	recorded := &take.Take{
		Cols: pageWidth, Rows: pageHeight, FPS: pageFPS, Cell: take.CellSize{W: 1, H: 1},
		Started: time.Date(2026, time.November, 6, 9, 30, 0, 0, time.UTC),
	}
	keys := 0
	for index := range pageSeconds * pageFPS {
		if index%pageTypeFor == 0 {
			typed = append(typed, rng.IntN(glyphCount))
		}
		drawPageFrame(frame, page, panels[(index/pageSwapFor)%2], glyphs, typed,
			(index/pageScrollFor)*pageScrollStep)
		if spinner {
			drawSpinner(frame, index)
		}
		at := time.Duration(index) * time.Second / pageFPS
		entry, err := encoder.Encode(at, frame)
		if err != nil {
			t.Fatalf("Encode(frame %d): %v", index, err)
		}
		if entry == nil {
			continue
		}
		if entry.Key {
			keys++
		}
		recorded.Snapshots = append(recorded.Snapshots, take.Snapshot{At: at, Image: entry})
	}

	path := filepath.Join(t.TempDir(), "take.gz")
	if err := take.Save(path, recorded); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	t.Logf("%d snapshots, %d key frames, %d bytes on disk", len(recorded.Snapshots), keys,
		info.Size())
	if info.Size() > pageBudget {
		t.Errorf("the one-minute take is %d bytes, over the %d-byte budget", info.Size(), pageBudget)
	}

	last, err := NewDecoder(recorded).Frame(len(recorded.Snapshots) - 1)
	if err != nil {
		t.Fatalf("Frame(last): %v", err)
	}
	if !sameFrame(last, frame) {
		t.Error("the last snapshot's frame differs from the last frame encoded")
	}
}

// glyph is one character's coverage, 0 (paper) to 255 (ink), row by row.
type glyph [glyphHeight][glyphWidth]uint8

// makeGlyphs returns glyphCount made-up characters with anti-aliased edges.
func makeGlyphs(rng *rand.Rand) []glyph {
	levels := []uint8{0, 0, 0, 90, 170, 255, 255}
	glyphs := make([]glyph, glyphCount)
	for index := range glyphs {
		for y := 2; y < glyphHeight-2; y++ {
			for x := 1; x < glyphWidth-1; x++ {
				glyphs[index][y][x] = levels[rng.IntN(len(levels))]
			}
		}
	}
	return glyphs
}

// makePage returns the scrolling page: lines of words over a white page, with a rule every
// twentieth line.
func makePage(rng *rand.Rand, glyphs []glyph) *image.RGBA {
	page := image.NewRGBA(image.Rect(0, 0, pageWidth, pageLength))
	fill(page, page.Rect, color.RGBA{R: 252, G: 252, B: 250, A: 255})
	ink := color.RGBA{R: 30, G: 34, B: 40, A: 255}
	for line := 0; (line+1)*pageLineHeight <= pageLength; line++ {
		y := line * pageLineHeight
		if line%20 == 0 {
			fill(page, image.Rect(0, y+pageLineHeight/2, pageWidth, y+pageLineHeight/2+1), ink)
			continue
		}
		writeWords(page, rng, glyphs, image.Rect(pageTextColumn.Min.X, y+4,
			pageTextColumn.Max.X-rng.IntN(300), y+4+glyphHeight), ink)
	}
	return page
}

// makePanel returns a side panel of words on a tinted background.
func makePanel(rng *rand.Rand, glyphs []glyph, tint uint32) *image.RGBA {
	panel := image.NewRGBA(image.Rectangle{Max: pagePanel.Size()})
	fill(panel, panel.Rect, color.RGBA{R: uint8(tint >> 16), G: uint8(tint >> 8), B: uint8(tint), A: 255})
	ink := color.RGBA{R: 20, G: 60, B: 110, A: 255}
	for y := 12; y+glyphHeight < panel.Rect.Dy(); y += pageLineHeight {
		writeWords(panel, rng, glyphs, image.Rect(12, y, panel.Rect.Dx()-12-rng.IntN(120), y+glyphHeight), ink)
	}
	return panel
}

// writeWords fills one line rect of img with words of random glyphs.
func writeWords(img *image.RGBA, rng *rand.Rand, glyphs []glyph, rect image.Rectangle, ink color.RGBA) {
	x := rect.Min.X
	for x+glyphWidth <= rect.Max.X {
		for range 2 + rng.IntN(8) {
			if x+glyphWidth > rect.Max.X {
				break
			}
			drawGlyph(img, glyphs[rng.IntN(len(glyphs))], image.Pt(x, rect.Min.Y), ink)
			x += glyphWidth
		}
		x += glyphWidth
	}
}

// drawPageFrame draws the page scrolled down by scroll rows, the side panel, and the typing box
// holding the glyphs typed so far.
func drawPageFrame(frame, page, panel *image.RGBA, glyphs []glyph, typed []int, scroll int) {
	for y := range pageHeight {
		copy(row(frame, 0, y, pageWidth), row(page, 0, (y+scroll)%pageLength, pageWidth))
	}
	for y := range pagePanel.Dy() {
		copy(row(frame, pagePanel.Min.X, pagePanel.Min.Y+y, pagePanel.Dx()),
			row(panel, 0, y, pagePanel.Dx()))
	}
	fill(frame, pageTypingBox, color.RGBA{R: 240, G: 240, B: 236, A: 255})
	perLine := (pageTypingBox.Dx() - 2*glyphWidth) / glyphWidth
	for index, which := range typed {
		at := image.Pt(pageTypingBox.Min.X+glyphWidth+(index%perLine)*glyphWidth,
			pageTypingBox.Min.Y+4+(index/perLine)*(glyphHeight+6))
		drawGlyph(frame, glyphs[which], at, color.RGBA{A: 255})
	}
}

// drawSpinner draws the spinner at step of its turn: a ring of dots, the leading one darkest and
// each one behind it lighter.
func drawSpinner(frame *image.RGBA, step int) {
	fill(frame, pageSpinner, color.RGBA{R: 252, G: 252, B: 250, A: 255})
	center := image.Pt((pageSpinner.Min.X+pageSpinner.Max.X)/2, (pageSpinner.Min.Y+pageSpinner.Max.Y)/2)
	for dot := range spinnerDots {
		sin, cos := math.Sincos(2 * math.Pi * float64(dot) / spinnerDots)
		at := center.Add(image.Pt(int(spinnerRadius*cos)-spinnerDot/2, int(spinnerRadius*sin)-spinnerDot/2))
		shade := uint8(200 * ((step - dot + spinnerDots) % spinnerDots) / spinnerDots)
		fill(frame, image.Rectangle{Min: at, Max: at.Add(image.Pt(spinnerDot, spinnerDot))},
			color.RGBA{R: shade, G: shade, B: shade + 40, A: 255})
	}
}

// drawGlyph blends g in ink over img with its top-left corner at at.
func drawGlyph(img *image.RGBA, g glyph, at image.Point, ink color.RGBA) {
	for y := range glyphHeight {
		for x := range glyphWidth {
			cover := uint32(g[y][x])
			if cover == 0 {
				continue
			}
			paper := img.RGBAAt(at.X+x, at.Y+y)
			blend := func(p, i uint8) uint8 { return uint8((uint32(p)*(255-cover) + uint32(i)*cover) / 255) }
			img.SetRGBA(at.X+x, at.Y+y, color.RGBA{
				R: blend(paper.R, ink.R), G: blend(paper.G, ink.G), B: blend(paper.B, ink.B), A: 255,
			})
		}
	}
}

// fill paints rect of img in c.
func fill(img *image.RGBA, rect image.Rectangle, c color.RGBA) {
	rect = rect.Intersect(img.Rect)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// clone returns a copy of img with its bounds moved to start at (0, 0).
func clone(img *image.RGBA) *image.RGBA {
	size := img.Rect.Size()
	copied := image.NewRGBA(image.Rectangle{Max: size})
	for y := range size.Y {
		copy(row(copied, 0, y, size.X), row(img, 0, y, size.X))
	}
	return copied
}

// sameFrame reports whether a and b are the same size and hold the same bytes, wherever their
// bounds start.
func sameFrame(a, b *image.RGBA) bool {
	size := a.Rect.Size()
	if size != b.Rect.Size() {
		return false
	}
	for y := range size.Y {
		if !bytes.Equal(row(a, 0, y, size.X), row(b, 0, y, size.X)) {
			return false
		}
	}
	return true
}
