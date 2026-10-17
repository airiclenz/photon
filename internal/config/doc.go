// Package config holds a project's Config — its `photon.yaml`, shared by every storyboard of the
// project — and the two ways to get one: [Find] locates the file (the `--config` flag, else the
// nearest photon.yaml at or above the storyboard's directory) and [Load] reads it. Load refuses
// an unknown field, reports every rule the file breaks at once in a [ValidationError], resolves
// the config's relative paths against the file's own directory and fills the defaults a project
// may leave out.
//
// The config is how a project plugs into photon: the commands that build and reset its rig, the
// one it films, the services a take needs and the profiles that swap them, how a take ends, the
// areas its screen is divided into, the theme the raster draws with and the fonts it uses. Every
// command is one string, run later by `sh -c`; nothing here runs one.
//
// It imports only keys, to check that the end keys name keys photon can send. Whether an area a
// storyboard names is declared here is checked where the two are loaded together, not here.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - config.go: the [Config], its [Launch], [End], [Theme] and [Color], [Load], the defaults,
//     [ValidationError] and the top-level rules.
//   - area.go: an [Area], its [Anchor] edges and [Pick], and their rules.
//   - service.go: a [Service] and its [Ready] check, a [Profile] and its [ServiceOverride],
//     [PortEnvName], and their rules.
//   - find.go: [Find], [FileName] and [ErrNotFound].
package config
