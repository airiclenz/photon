package compose

import (
	"math"
	"testing"
)

// pathSamples is how many equal steps of progress the path tests walk.
const pathSamples = 400

// winding is a path from its start through two waypoints that turn it each way, to its end.
var windingStart, windingVia, windingEnd = pointF{X: 100, Y: 500},
	[]pointF{{X: 300, Y: 150}, {X: 600, Y: 400}},
	pointF{X: 900, Y: 100}

// newWindingPath is the path through windingStart, windingVia and windingEnd.
func newWindingPath() path {
	return newPath(windingStart, append(append([]pointF{}, windingVia...), windingEnd))
}

// walkPath is the path's point at each of pathSamples+1 equal steps of progress from 0 to 1.
func walkPath(route path) []pointF {
	points := make([]pointF, 0, pathSamples+1)
	for step := range pathSamples + 1 {
		points = append(points, route.at(float64(step)/pathSamples))
	}
	return points
}

func TestPathEndpointsAreExact(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		route path
		end   pointF
	}{
		{name: "through waypoints", route: newWindingPath(), end: windingEnd},
		{name: "two points", route: newPath(windingStart, []pointF{windingEnd}), end: windingEnd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.route.at(0); got != windingStart {
				t.Errorf("at 0: want %+v, got %+v", windingStart, got)
			}
			if got := tc.route.at(1); got != tc.end {
				t.Errorf("at 1: want %+v, got %+v", tc.end, got)
			}
		})
	}
}

func TestPathPassesThroughEachWaypoint(t *testing.T) {
	t.Parallel()
	route := newWindingPath()
	const denseSamples = 10 * pathSamples

	for _, waypoint := range windingVia {
		nearest := math.Inf(1)
		for step := range denseSamples + 1 {
			nearest = math.Min(nearest, distance(route.at(float64(step)/denseSamples), waypoint))
		}

		if nearest > 1 {
			t.Errorf("waypoint %+v: nearest point of the path is %.2f px away, want within 1", waypoint, nearest)
		}
	}
}

func TestPathTurnsWithoutACorner(t *testing.T) {
	t.Parallel()
	const maxTurnDegrees = 10
	points := walkPath(newWindingPath())

	for index := 2; index < len(points); index++ {
		before := math.Atan2(points[index-1].Y-points[index-2].Y, points[index-1].X-points[index-2].X)
		after := math.Atan2(points[index].Y-points[index-1].Y, points[index].X-points[index-1].X)
		turn := math.Abs(math.Remainder(after-before, 2*math.Pi)) * 180 / math.Pi

		if turn >= maxTurnDegrees {
			t.Fatalf("sample %d: heading turns %.2f°, want under %d°", index, turn, maxTurnDegrees)
		}
	}
}

func TestPathStepsEqualLengths(t *testing.T) {
	t.Parallel()
	const tolerance = 0.02
	points := walkPath(newWindingPath())
	steps := make([]float64, 0, pathSamples)
	total := 0.0
	for index := 1; index < len(points); index++ {
		steps = append(steps, distance(points[index-1], points[index]))
		total += steps[len(steps)-1]
	}
	mean := total / pathSamples

	for index, step := range steps {
		if math.Abs(step-mean) > tolerance*mean {
			t.Fatalf("step %d covers %.3f px, want within %g%% of the mean %.3f", index, step, tolerance*100, mean)
		}
	}
}

func TestPathOfTwoPointsIsTheLine(t *testing.T) {
	t.Parallel()
	route := newPath(windingStart, []pointF{windingEnd})

	for _, share := range []float64{0.1, 0.25, 0.5, 0.9} {
		if got, want := route.at(share), lerp(windingStart, windingEnd, share); got != want {
			t.Errorf("at %g: want %+v, got %+v", share, want, got)
		}
	}
}

func TestPathDropsCoincidentPoints(t *testing.T) {
	t.Parallel()
	middle := pointF{X: 400, Y: 200}
	for _, tc := range []struct {
		name      string
		further   []pointF
		wantStill bool
	}{
		{name: "repeated waypoints", further: []pointF{windingStart, middle, middle, windingEnd, windingEnd}},
		{name: "one distinct point", further: []pointF{windingStart}, wantStill: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for step, point := range walkPath(newPath(windingStart, tc.further)) {
				if math.IsNaN(point.X) || math.IsNaN(point.Y) {
					t.Fatalf("sample %d: want a point, got %+v", step, point)
				}
				if tc.wantStill && point != windingStart {
					t.Fatalf("sample %d: want the path to hold at %+v, got %+v", step, windingStart, point)
				}
			}
		})
	}
}
