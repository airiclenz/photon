// Package schedule lays a take's beats onto the clip's clock. Each storyboard beat becomes a
// [Section] of its `duration`: a beat that fits plays at 1× and then freezes its last frame for
// the rest of its duration; a longer one plays 1× over its `hold` and the rest sped up to fit. A
// `cut` beat is dropped from the clip. A section faster than 6× earns a warning, and a take whose
// beats disagree with the storyboard's or ran in another order is an error.
//
// The board comes from [storyboard.Parse] (or [storyboard.Load]), whose validation already
// refuses a beat timing that cannot be scheduled and a board with no kept beat, so the
// schedule trusts those values and does not check them again.
//
// A [Schedule] maps between the clip's output clock and the take's source clock in both
// directions ([Schedule.Source], [Schedule.Output]) and samples the output clock at a frame rate
// ([Schedule.Frames]), so the render knows which take instant every frame shows and where a
// recorded click lands in the clip.
//
// It imports storyboard and take.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - schedule.go: [New], the [Schedule], its [Section]s and sampled [Frame]s.
package schedule
