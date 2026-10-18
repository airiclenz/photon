package raster

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"
)

// The ranges drawn procedurally rather than from a face, so every line and block meets its
// neighbour's edge exactly: box drawing (U+2500–257F) and block elements (U+2580–259F).
const (
	boxDrawingFirst    = 0x2500
	blockElementsFirst = 0x2580
	blockElementsLast  = 0x259F
)

// lightStrokeDivisor sets a light box-drawing line's thickness: an eighth of the cell width, at
// least one pixel.
const lightStrokeDivisor = 8

// fullCoverage is a mask pixel the glyph covers completely.
const fullCoverage = 0xff

// isProcedural reports whether ru is drawn by [proceduralGlyph] instead of from a face.
func isProcedural(ru rune) bool { return ru >= boxDrawingFirst && ru <= blockElementsLast }

// lightStroke is the thickness of a light box-drawing line in a cell cellWidth pixels wide; a
// heavy line is twice it, a double line two light lines a light line apart.
func lightStroke(cellWidth int) int {
	return max(1, int(math.Round(float64(cellWidth)/lightStrokeDivisor)))
}

// proceduralGlyph draws a box-drawing or block-element rune into a width × height mask; stroke is
// the light line thickness. A box-drawing rune no table covers leaves the mask empty.
func proceduralGlyph(ru rune, width, height, stroke int) *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, width, height))
	if ru >= blockElementsFirst {
		drawBlockElement(mask, ru)
		return mask
	}
	if a, ok := boxArmTable[ru]; ok {
		drawBoxArms(mask, a, stroke)
		return mask
	}
	if dash, ok := boxDashTable[ru]; ok {
		drawBoxDashes(mask, dash, stroke)
		return mask
	}
	switch {
	case ru >= boxArcFirst && ru <= boxArcLast:
		arc := boxArcDirections[ru-boxArcFirst]
		drawBoxArc(mask, arc[0], arc[1], stroke)
	case ru == boxDiagonalRising:
		drawDiagonal(mask, true, stroke)
	case ru == boxDiagonalFalling:
		drawDiagonal(mask, false, stroke)
	case ru == boxDiagonalCross:
		drawDiagonal(mask, true, stroke)
		drawDiagonal(mask, false, stroke)
	}
	return mask
}

// fillMask sets every pixel of r (clipped to the mask) to alpha.
func fillMask(mask *image.Alpha, r image.Rectangle, alpha uint8) {
	fill := &image.Uniform{C: color.Alpha{A: alpha}}
	draw.Draw(mask, r.Intersect(mask.Rect), fill, image.Point{}, draw.Src)
}

// Box-drawing line weights, as boxArmTable spells them.
const (
	weightNone   = '.'
	weightLight  = 'l'
	weightHeavy  = 'h'
	weightDouble = 'd'
)

// The four arms of a box-drawing glyph, in boxArms order.
const (
	armUp = iota
	armRight
	armDown
	armLeft
	armCount
)

// boxArms is the weight of each arm running from the cell's centre to an edge: up, right, down,
// left.
type boxArms [armCount]byte

// arms spells a boxArms from its four weight letters, up-right-down-left.
func arms(spelling string) boxArms {
	var a boxArms
	copy(a[:], spelling)
	return a
}

// boxArmTable covers every box-drawing rune made only of straight arms from the centre.
var boxArmTable = map[rune]boxArms{
	0x2500: arms(".l.l"), 0x2501: arms(".h.h"), 0x2502: arms("l.l."), 0x2503: arms("h.h."),
	0x250C: arms(".ll."), 0x250D: arms(".hl."), 0x250E: arms(".lh."), 0x250F: arms(".hh."),
	0x2510: arms("..ll"), 0x2511: arms("..lh"), 0x2512: arms("..hl"), 0x2513: arms("..hh"),
	0x2514: arms("ll.."), 0x2515: arms("lh.."), 0x2516: arms("hl.."), 0x2517: arms("hh.."),
	0x2518: arms("l..l"), 0x2519: arms("l..h"), 0x251A: arms("h..l"), 0x251B: arms("h..h"),
	0x251C: arms("lll."), 0x251D: arms("lhl."), 0x251E: arms("hll."), 0x251F: arms("llh."),
	0x2520: arms("hlh."), 0x2521: arms("hhl."), 0x2522: arms("lhh."), 0x2523: arms("hhh."),
	0x2524: arms("l.ll"), 0x2525: arms("l.lh"), 0x2526: arms("h.ll"), 0x2527: arms("l.hl"),
	0x2528: arms("h.hl"), 0x2529: arms("h.lh"), 0x252A: arms("l.hh"), 0x252B: arms("h.hh"),
	0x252C: arms(".lll"), 0x252D: arms(".llh"), 0x252E: arms(".hll"), 0x252F: arms(".hlh"),
	0x2530: arms(".lhl"), 0x2531: arms(".lhh"), 0x2532: arms(".hhl"), 0x2533: arms(".hhh"),
	0x2534: arms("ll.l"), 0x2535: arms("ll.h"), 0x2536: arms("lh.l"), 0x2537: arms("lh.h"),
	0x2538: arms("hl.l"), 0x2539: arms("hl.h"), 0x253A: arms("hh.l"), 0x253B: arms("hh.h"),
	0x253C: arms("llll"), 0x253D: arms("lllh"), 0x253E: arms("lhll"), 0x253F: arms("lhlh"),
	0x2540: arms("hlll"), 0x2541: arms("llhl"), 0x2542: arms("hlhl"), 0x2543: arms("hllh"),
	0x2544: arms("hhll"), 0x2545: arms("llhh"), 0x2546: arms("lhhl"), 0x2547: arms("hhlh"),
	0x2548: arms("lhhh"), 0x2549: arms("hlhh"), 0x254A: arms("hhhl"), 0x254B: arms("hhhh"),
	0x2550: arms(".d.d"), 0x2551: arms("d.d."), 0x2552: arms(".dl."), 0x2553: arms(".ld."),
	0x2554: arms(".dd."), 0x2555: arms("..ld"), 0x2556: arms("..dl"), 0x2557: arms("..dd"),
	0x2558: arms("ld.."), 0x2559: arms("dl.."), 0x255A: arms("dd.."), 0x255B: arms("l..d"),
	0x255C: arms("d..l"), 0x255D: arms("d..d"), 0x255E: arms("ldl."), 0x255F: arms("dld."),
	0x2560: arms("ddd."), 0x2561: arms("l.ld"), 0x2562: arms("d.dl"), 0x2563: arms("d.dd"),
	0x2564: arms(".dld"), 0x2565: arms(".ldl"), 0x2566: arms(".ddd"), 0x2567: arms("ld.d"),
	0x2568: arms("dl.l"), 0x2569: arms("dd.d"), 0x256A: arms("ldld"), 0x256B: arms("dldl"),
	0x256C: arms("dddd"),
	0x2574: arms("...l"), 0x2575: arms("l..."), 0x2576: arms(".l.."), 0x2577: arms("..l."),
	0x2578: arms("...h"), 0x2579: arms("h..."), 0x257A: arms(".h.."), 0x257B: arms("..h."),
	0x257C: arms(".h.l"), 0x257D: arms("l.h."), 0x257E: arms(".l.h"), 0x257F: arms("h.l."),
}

// interval is a half-open pixel interval [from, to) along one axis.
type interval struct{ from, to int }

// weightBands is where a line of weight crosses an axis whose light line starts at start: one
// band for light and heavy, two for double, none for no line.
func weightBands(weight byte, start, stroke int) []interval {
	switch weight {
	case weightLight:
		return []interval{{start, start + stroke}}
	case weightHeavy:
		heavyStart := start - stroke/2
		return []interval{{heavyStart, heavyStart + 2*stroke}}
	case weightDouble:
		return []interval{{start - stroke, start}, {start + stroke, start + 2*stroke}}
	}
	return nil
}

// drawBoxArms draws each arm from the edge in to the centre. A light or heavy arm reaches across
// the perpendicular arms' lines so the joint is solid; each line of a double arm stops where the
// joint needs it — at the near line of a perpendicular double, on the far line of an outer
// corner — so ╔ ╠ ╬ come out as two unbroken strokes.
func drawBoxArms(mask *image.Alpha, a boxArms, stroke int) {
	for arm := range armCount {
		if a[arm] != weightNone {
			drawBoxArm(mask, a, arm, stroke)
		}
	}
}

// drawBoxArm draws one arm of a: each of its lines from the arm's edge to where the joint at the
// centre stops it.
func drawBoxArm(mask *image.Alpha, a boxArms, arm, stroke int) {
	width, height := mask.Rect.Dx(), mask.Rect.Dy()
	centreX, centreY := (width-stroke)/2, (height-stroke)/2
	weight := a[arm]
	horizontal := arm == armRight || arm == armLeft
	toward := 1
	if arm == armLeft || arm == armUp {
		toward = -1
	}
	alongStart, alongSize, acrossStart := centreY, height, centreX
	negativeSide, positiveSide := a[armLeft], a[armRight]
	if horizontal {
		alongStart, alongSize, acrossStart = centreX, width, centreY
		negativeSide, positiveSide = a[armUp], a[armDown]
	}
	opposite := a[(arm+2)%armCount]

	for i, across := range weightBands(weight, acrossStart, stroke) {
		var reach interval
		if weight == weightDouble {
			near, far := negativeSide, positiveSide
			if i == 1 {
				near, far = positiveSide, negativeSide
			}
			reach = doubleLineReach(near, far, opposite, alongStart, stroke, toward)
		} else {
			reach = solidLineReach(negativeSide, positiveSide, alongStart, stroke)
		}
		along := interval{reach.from, alongSize}
		if toward < 0 {
			along = interval{0, reach.to}
		}
		rect := image.Rect(along.from, across.from, along.to, across.to)
		if !horizontal {
			rect = image.Rect(across.from, along.from, across.to, along.to)
		}
		fillMask(mask, rect, fullCoverage)
	}
}

// solidLineReach is the stretch of the along axis the perpendicular arms' lines cover, which a
// light or heavy arm reaches into; with no perpendicular arm it is the centre's light band.
func solidLineReach(negativeSide, positiveSide byte, alongStart, stroke int) interval {
	reach := interval{alongStart, alongStart + stroke}
	first := true
	for _, weight := range []byte{negativeSide, positiveSide} {
		for _, band := range weightBands(weight, alongStart, stroke) {
			if first {
				reach, first = band, false
				continue
			}
			reach = interval{min(reach.from, band.from), max(reach.to, band.to)}
		}
	}
	return reach
}

// doubleLineReach is where one line of a double arm stops. near is the perpendicular arm on the
// line's own side, far the one across from it, opposite the arm continuing straight on; toward
// is +1 for an arm running to the right or bottom edge. Only the side facing the arm's edge of
// the returned interval is used.
func doubleLineReach(near, far, opposite byte, alongStart, stroke, toward int) interval {
	nearBands := weightBands(near, alongStart, stroke)
	farBands := weightBands(far, alongStart, stroke)
	outer := interval{alongStart - stroke, alongStart + 2*stroke}
	switch {
	case near == weightDouble:
		// Stop on the perpendicular's line nearest this arm's edge — the inner corner.
		if toward > 0 {
			return nearBands[1]
		}
		return nearBands[0]
	case near != weightNone:
		return nearBands[0]
	case opposite != weightNone:
		return outer
	case far == weightDouble:
		return outer
	case far != weightNone:
		return farBands[0]
	}
	return interval{alongStart, alongStart + stroke}
}

// boxDash is a dashed line: its orientation, weight and how many dashes fill the cell.
type boxDash struct {
	horizontal bool
	weight     byte
	count      int
}

// boxDashTable covers the dashed box-drawing runes.
var boxDashTable = map[rune]boxDash{
	0x2504: {true, weightLight, 3}, 0x2505: {true, weightHeavy, 3},
	0x2506: {false, weightLight, 3}, 0x2507: {false, weightHeavy, 3},
	0x2508: {true, weightLight, 4}, 0x2509: {true, weightHeavy, 4},
	0x250A: {false, weightLight, 4}, 0x250B: {false, weightHeavy, 4},
	0x254C: {true, weightLight, 2}, 0x254D: {true, weightHeavy, 2},
	0x254E: {false, weightLight, 2}, 0x254F: {false, weightHeavy, 2},
}

// dashGapDivisor leaves a third of each dash's slot as the gap.
const dashGapDivisor = 3

// drawBoxDashes splits the cell's length into count equal slots and draws a dash centred in each,
// a third of the slot left as the gap.
func drawBoxDashes(mask *image.Alpha, dash boxDash, stroke int) {
	width, height := mask.Rect.Dx(), mask.Rect.Dy()
	length, across := height, weightBands(dash.weight, (width-stroke)/2, stroke)[0]
	if dash.horizontal {
		length, across = width, weightBands(dash.weight, (height-stroke)/2, stroke)[0]
	}
	for i := range dash.count {
		from, to := i*length/dash.count, (i+1)*length/dash.count
		gap := max(1, (to-from)/dashGapDivisor)
		from, to = from+gap/2, to-(gap-gap/2)
		rect := image.Rect(across.from, from, across.to, to)
		if dash.horizontal {
			rect = image.Rect(from, across.from, to, across.to)
		}
		fillMask(mask, rect, fullCoverage)
	}
}

// The rounded corners ╭╮╯╰ and the diagonals ╱╲╳.
const (
	boxArcFirst        = 0x256D
	boxArcLast         = 0x2570
	boxDiagonalRising  = 0x2571
	boxDiagonalFalling = 0x2572
	boxDiagonalCross   = 0x2573
)

// boxArcDirections gives each rounded corner the edges it joins: horizontal (+1 right, −1 left)
// and vertical (+1 down, −1 up).
var boxArcDirections = [...][2]float64{
	{1, 1},   // ╭
	{-1, 1},  // ╮
	{-1, -1}, // ╯
	{1, -1},  // ╰
}

// arcSegments is how many straight pieces approximate a rounded corner's quarter circle.
const arcSegments = 16

// arcPoint is one point on a rounded corner's centre line and the unit normal the stroke is
// offset along there.
type arcPoint struct{ x, y, normalX, normalY float64 }

// drawBoxArc strokes a rounded corner in a light line: straight in from the vertical edge along
// the light line's centre, a quarter circle, and straight out to the horizontal edge, so it meets
// ─ and │ in the neighbouring cells exactly.
func drawBoxArc(mask *image.Alpha, horizontal, vertical float64, stroke int) {
	width, height := float64(mask.Rect.Dx()), float64(mask.Rect.Dy())
	half := float64(stroke) / 2
	lineX := float64((mask.Rect.Dx()-stroke)/2) + half
	lineY := float64((mask.Rect.Dy()-stroke)/2) + half
	// One stroke short of the tightest edge, so the corner leaves the cell on a straight run that
	// meets the neighbouring line flush.
	radius := max(half, min(lineX, width-lineX, lineY, height-lineY)-float64(stroke))
	centreX, centreY := lineX+horizontal*radius, lineY+vertical*radius
	edgeX, edgeY := width, height
	if horizontal < 0 {
		edgeX = 0
	}
	if vertical < 0 {
		edgeY = 0
	}

	path := []arcPoint{{lineX, edgeY, -horizontal, 0}}
	for i := range arcSegments + 1 {
		angle := float64(i) / arcSegments * math.Pi / 2
		dx, dy := -horizontal*math.Cos(angle), -vertical*math.Sin(angle)
		path = append(path, arcPoint{centreX + radius*dx, centreY + radius*dy, dx, dy})
	}
	path = append(path, arcPoint{edgeX, lineY, 0, -vertical})

	outline := make([][2]float64, 0, 2*len(path))
	for _, p := range path {
		outline = append(outline, [2]float64{p.x + p.normalX*half, p.y + p.normalY*half})
	}
	for i := len(path) - 1; i >= 0; i-- {
		p := path[i]
		outline = append(outline, [2]float64{p.x - p.normalX*half, p.y - p.normalY*half})
	}
	fillPolygon(mask, outline)
}

// drawDiagonal strokes a light line corner to corner: bottom-left to top-right when rising,
// top-left to bottom-right otherwise.
func drawDiagonal(mask *image.Alpha, rising bool, stroke int) {
	width, height := float64(mask.Rect.Dx()), float64(mask.Rect.Dy())
	fromX, fromY, toX, toY := 0.0, 0.0, width, height
	if rising {
		fromY, toY = height, 0
	}
	length := math.Hypot(toX-fromX, toY-fromY)
	half := float64(stroke) / 2
	normalX, normalY := -(toY-fromY)/length*half, (toX-fromX)/length*half
	fillPolygon(mask, [][2]float64{
		{fromX + normalX, fromY + normalY}, {toX + normalX, toY + normalY},
		{toX - normalX, toY - normalY}, {fromX - normalX, fromY - normalY},
	})
}

// fillPolygon adds an anti-aliased closed polygon's coverage to the mask.
func fillPolygon(mask *image.Alpha, points [][2]float64) {
	z := vector.NewRasterizer(mask.Rect.Dx(), mask.Rect.Dy())
	z.MoveTo(float32(points[0][0]), float32(points[0][1]))
	for _, p := range points[1:] {
		z.LineTo(float32(p[0]), float32(p[1]))
	}
	z.ClosePath()
	z.Draw(mask, mask.Rect, image.Opaque, image.Point{})
}
