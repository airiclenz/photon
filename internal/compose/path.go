package compose

import (
	"math"
	"sort"
)

// pathSamplesPerSpan is how many steps of the curve between two neighbouring points the
// arc-length table measures: enough that equal steps of progress cover equal lengths to well
// under a percent.
const pathSamplesPerSpan = 64

// centripetalAlpha is the knot exponent of a centripetal Catmull-Rom curve: each knot interval
// is the square root of the distance between its points, which keeps the curve from looping or
// overshooting where its points bunch up.
const centripetalAlpha = 0.5

// path is the route of a pan: a centripetal Catmull-Rom curve through a start point, any
// waypoints and an end, walked by arc length, so equal steps of progress cover equal distances
// and the curve turns without a corner at each waypoint. A path of two distinct points is the
// straight line between them; a path of one holds still. A path is built once and only read
// after, so it is safe for concurrent use.
type path struct {
	// points are the points the curve passes through, with consecutive coincident ones dropped.
	points []pointF

	// lengths holds the arc length from the start to each sample of the curve:
	// pathSamplesPerSpan samples per span between neighbouring points, plus the end.
	lengths []float64
}

// newPath is the path from start through each of further in order. A point equal to the one
// before it is dropped: its knot interval would be zero, and the curve's blend would divide 0
// by 0.
func newPath(start pointF, further []pointF) path {
	points := []pointF{start}
	for _, point := range further {
		if point != points[len(points)-1] {
			points = append(points, point)
		}
	}
	p := path{points: points}
	if len(points) > 2 {
		p.lengths = p.measure()
	}
	return p
}

// measure is the table of arc lengths at every sample of a curve of at least three points.
func (p path) measure() []float64 {
	spans := len(p.points) - 1
	lengths := make([]float64, 0, spans*pathSamplesPerSpan+1)
	lengths = append(lengths, 0)
	previous := p.points[0]
	for span := range spans {
		for step := 1; step <= pathSamplesPerSpan; step++ {
			sample := p.spanAt(span, float64(step)/pathSamplesPerSpan)
			lengths = append(lengths, lengths[len(lengths)-1]+distance(previous, sample))
			previous = sample
		}
	}
	return lengths
}

// at is the point a share e ∈ [0, 1] of the way along the path by arc length; e outside that
// range is clamped. The ends are exact: 0 is the start and 1 the last point.
func (p path) at(e float64) pointF {
	e = clamp01(e)
	last := len(p.points) - 1
	switch {
	case last == 0 || e == 0:
		return p.points[0]
	case e == 1:
		return p.points[last]
	case last == 1:
		return lerp(p.points[0], p.points[1], e)
	}

	target := e * p.lengths[len(p.lengths)-1]
	next := sort.SearchFloat64s(p.lengths, target)
	sample := next - 1
	share := 0.0
	if gap := p.lengths[next] - p.lengths[sample]; gap > 0 {
		share = (target - p.lengths[sample]) / gap
	}
	span, step := sample/pathSamplesPerSpan, sample%pathSamplesPerSpan
	return p.spanAt(span, (float64(step)+share)/pathSamplesPerSpan)
}

// spanAt is the curve's point a share u ∈ [0, 1] of the way through the knot interval of span,
// the stretch from points[span] to points[span+1]. The curve's first and last spans lean on a
// phantom point mirrored through the path's end, so the path leaves its start and reaches its
// end heading straight for its neighbour.
func (p path) spanAt(span int, u float64) pointF {
	from, to := p.points[span], p.points[span+1]
	before := reflect(from, to)
	if span > 0 {
		before = p.points[span-1]
	}
	after := reflect(to, from)
	if span+2 < len(p.points) {
		after = p.points[span+2]
	}
	return centripetalPoint([4]pointF{before, from, to, after}, u)
}

// centripetalPoint is the point a share u of the way through the middle knot interval of the
// centripetal Catmull-Rom curve over four points, by the Barry and Goldman pyramid of linear
// blends. No two neighbouring points may coincide.
func centripetalPoint(points [4]pointF, u float64) pointF {
	var knots [4]float64
	for index := 1; index < len(knots); index++ {
		knots[index] = knots[index-1] + math.Pow(distance(points[index-1], points[index]), centripetalAlpha)
	}
	t := knots[1] + u*(knots[2]-knots[1])
	blend := func(a, b pointF, start, end float64) pointF { return lerp(a, b, (t-start)/(end-start)) }

	first := blend(points[0], points[1], knots[0], knots[1])
	second := blend(points[1], points[2], knots[1], knots[2])
	third := blend(points[2], points[3], knots[2], knots[3])
	left := blend(first, second, knots[0], knots[2])
	right := blend(second, third, knots[1], knots[3])
	return blend(left, right, knots[1], knots[2])
}

// reflect is point mirrored through centre.
func reflect(centre, point pointF) pointF {
	return pointF{X: 2*centre.X - point.X, Y: 2*centre.Y - point.Y}
}

// distance is the straight-line distance between two points.
func distance(a, b pointF) float64 { return math.Hypot(b.X-a.X, b.Y-a.Y) }
