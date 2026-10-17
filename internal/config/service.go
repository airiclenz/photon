package config

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Service is a helper process photon starts before a take's launch and stops after it, such as
// a fake backend the filmed program talks to. Command is run by `sh -c` in the config's
// directory with Env added. Port is the port it serves: fixed, or 0 for photon to pick a free
// one; either way it reaches the service as PHOTON_PORT and every command as
// [PortEnvName](name). Ready is how photon tells it is up, and Timeout bounds that wait —
// [Load] sets an omitted one to [DefaultServiceTimeout].
type Service struct {
	Command string            `yaml:"command"`
	Env     map[string]string `yaml:"env"`
	Port    int               `yaml:"port"`
	Ready   Ready             `yaml:"ready"`
	Timeout time.Duration     `yaml:"timeout"`
}

// Ready is a service's readiness check, exactly one of: TCP, the service's port accepting a
// connection; or Line, a regex one line of the service's output matches.
type Ready struct {
	TCP  bool   `yaml:"tcp"`
	Line string `yaml:"line"`
}

// Profile is a named variant of a take's services, chosen with `photon record --profile`.
// Services maps the name of a service the config declares to what changes for it under this
// profile; the service's name, port and readiness stay as declared, so its PHOTON_PORT_<NAME>
// is the same under every profile. OnSuccess, when set, is a command run only after a take in
// which every beat passed.
type Profile struct {
	Services  map[string]ServiceOverride `yaml:"services"`
	OnSuccess string                     `yaml:"on_success"`
}

// ServiceOverride is what a profile changes for one service: a non-empty Command replaces the
// declared one, and Env entries are set over the declared env.
type ServiceOverride struct {
	Command string            `yaml:"command"`
	Env     map[string]string `yaml:"env"`
}

// DefaultServiceTimeout is how long photon waits for a service to become ready when the config
// gives no timeout.
const DefaultServiceTimeout = 10 * time.Second

// portEnvPrefix starts the name of the variable carrying a service's port.
const portEnvPrefix = "PHOTON_PORT_"

// maxPort is the highest TCP port.
const maxPort = 65535

// identifier is the spelling of a service or profile name. It is lowercase so that a service's
// port variable, its upper-cased name, is unambiguous.
var identifier = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// PortEnvName is the environment variable carrying the named service's port to the launch, to
// later services, to `run` expects and to on_success (not to reset, which runs before the services
// start): PHOTON_PORT_ followed by the name upper-cased, with each - turned into _.
func PortEnvName(service string) string {
	return portEnvPrefix + strings.ToUpper(strings.ReplaceAll(service, "-", "_"))
}

// validateServices checks every declared service, in name order, and that no two share a port
// variable or a fixed port.
func validateServices(services map[string]Service, report reporter) {
	byEnv := make(map[string]string, len(services))
	byPort := make(map[int]string, len(services))
	for _, name := range sortedKeys(services) {
		serviceReport := prefixed(report, fmt.Sprintf("services.%s: ", name))
		validateIdentifier(name, serviceReport)
		if other, taken := byEnv[PortEnvName(name)]; taken {
			serviceReport("name: shares %s with service %s", PortEnvName(name), other)
		}
		byEnv[PortEnvName(name)] = name

		service := services[name]
		service.validate(serviceReport)
		if other, taken := byPort[service.Port]; taken && service.Port != 0 {
			serviceReport("port: %d is also service %s's", service.Port, other)
		}
		byPort[service.Port] = name
	}
}

// validate checks the service's command, env, port, readiness check and timeout.
func (s Service) validate(report reporter) {
	validateCommand("command", s.Command, true, report)
	validateEnv("env", s.Env, report)
	if s.Port < 0 || s.Port > maxPort {
		report("port: want 0..%d, got %d", maxPort, s.Port)
	}
	if s.Ready.TCP == (s.Ready.Line != "") {
		report("ready: want exactly one of tcp or line")
	} else if s.Ready.Line != "" {
		validateRegex("ready.line", s.Ready.Line, report)
	}
	if s.Timeout < 0 {
		report("timeout: want ≥0, got %s", s.Timeout)
	}
}

// validateProfiles checks every profile, in name order: its name, that each service it
// overrides is declared, each override, and its on_success command.
func validateProfiles(profiles map[string]Profile, services map[string]Service, report reporter) {
	for _, name := range sortedKeys(profiles) {
		profileReport := prefixed(report, fmt.Sprintf("profiles.%s: ", name))
		validateIdentifier(name, profileReport)
		profile := profiles[name]
		for _, service := range sortedKeys(profile.Services) {
			overrideReport := prefixed(profileReport, fmt.Sprintf("services.%s: ", service))
			if _, declared := services[service]; !declared {
				overrideReport("not a declared service")
			}
			override := profile.Services[service]
			validateCommand("command", override.Command, false, overrideReport)
			validateEnv("env", override.Env, overrideReport)
		}
		validateCommand("on_success", profile.OnSuccess, false, profileReport)
	}
}

// validateIdentifier reports a service or profile name that is not spelled as [identifier].
func validateIdentifier(name string, report reporter) {
	if !identifier.MatchString(name) {
		report("name: want lowercase letters, digits, - and _ starting with a letter, got %q", name)
	}
}
