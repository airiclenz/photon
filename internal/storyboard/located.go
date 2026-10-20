package storyboard

import (
	"errors"
	"fmt"
)

// OwnerKind names what part of a storyboard a problem belongs to.
type OwnerKind string

// The spellings of OwnerKind.
const (
	OwnerClip     OwnerKind = "clip"
	OwnerBeat     OwnerKind = "beat"
	OwnerCamera   OwnerKind = "camera"
	OwnerCaption  OwnerKind = "caption"
	OwnerPosition OwnerKind = "position"
	OwnerZoom     OwnerKind = "zoom"
)

// Owner is the part of a storyboard a problem belongs to: a beat by its id, a camera move, a
// camera position, a camera zoom or a caption by its index in its list, or the clip as a whole
// with Index 0.
type Owner struct {
	Kind  OwnerKind `json:"kind"`
	Index int       `json:"index"`
}

// prefix is the text a problem's message starts with to name its owner: `beat N: `,
// `camera[i]: `, `camera.positions[i]: `, `camera.zooms[i]: ` or `captions[i]: `, and nothing for
// the clip.
func (o Owner) prefix() string {
	switch o.Kind {
	case OwnerBeat:
		return fmt.Sprintf("beat %d: ", o.Index)
	case OwnerCamera:
		return fmt.Sprintf("camera[%d]: ", o.Index)
	case OwnerPosition:
		return fmt.Sprintf("camera.positions[%d]: ", o.Index)
	case OwnerZoom:
		return fmt.Sprintf("camera.zooms[%d]: ", o.Index)
	case OwnerCaption:
		return fmt.Sprintf("captions[%d]: ", o.Index)
	default:
		return ""
	}
}

// LocatedError is a problem that belongs to one beat, camera move, position, zoom or caption. Its
// message is the owner's prefix before Err's, and it unwraps to Err alone, so [errors.Is] and
// [errors.As] reach what Err wraps.
type LocatedError struct {
	Owner
	Err error
}

// Error renders the owner's prefix before Err's message.
func (e *LocatedError) Error() string {
	return e.prefix() + e.Err.Error()
}

// Unwrap returns Err.
func (e *LocatedError) Unwrap() error { return e.Err }

// AtBeat files err, which must not be nil, under the beat with id.
func AtBeat(id int, err error) error {
	return &LocatedError{Owner: Owner{Kind: OwnerBeat, Index: id}, Err: err}
}

// AtCamera files err, which must not be nil, under the camera move at index.
func AtCamera(index int, err error) error {
	return &LocatedError{Owner: Owner{Kind: OwnerCamera, Index: index}, Err: err}
}

// AtPosition files err, which must not be nil, under the camera position at index.
func AtPosition(index int, err error) error {
	return &LocatedError{Owner: Owner{Kind: OwnerPosition, Index: index}, Err: err}
}

// AtZoom files err, which must not be nil, under the camera zoom at index.
func AtZoom(index int, err error) error {
	return &LocatedError{Owner: Owner{Kind: OwnerZoom, Index: index}, Err: err}
}

// AtCaption files err, which must not be nil, under the caption at index.
func AtCaption(index int, err error) error {
	return &LocatedError{Owner: Owner{Kind: OwnerCaption, Index: index}, Err: err}
}

// OwnerOf returns the owner of the outermost [LocatedError] in err's chain, as [errors.As] finds
// it, or the clip when err holds none. Give it one problem: on a joined error such as a
// [ValidationError] it answers for the first located problem only.
func OwnerOf(err error) Owner {
	var located *LocatedError
	if errors.As(err, &located) {
		return located.Owner
	}
	return Owner{Kind: OwnerClip}
}
