package tune

import "github.com/airiclenz/photon/internal/storyboard"

// Edit is one change the page asks for, a [storyboard.Edit], whose doc gives its wire shape and
// every field's value; [Session.Apply] hands it to [storyboard.Apply].
type Edit = storyboard.Edit
