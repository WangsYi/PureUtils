package host

import (
	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
)

// PluginFactory describes a compile-time bundled built-in plugin.
type PluginFactory struct {
	ID       string
	ModuleID core.ModuleID
	Name     string
	Desc     string
	New      func() purekit.Plugin
}

// CompiledModules reports which built-in modules are included in this binary.
func CompiledModules() core.ModuleConfig {
	return core.ModuleConfig{
		Capture: pureshotCompiled,
		History: pureclipCompiled,
		Viewer:  pureimageCompiled,
		Zip:     purezipCompiled,
		Video:   purevideoCompiled,
		Monitor: puremonitorCompiled,
	}
}

// ViewerCompiled reports whether PureImage was compiled into this binary.
func ViewerCompiled() bool {
	return pureimageCompiled
}

// BuiltinPlugins returns factories for all built-in plugins compiled into this build
// in deterministic registration order.
func BuiltinPlugins() []PluginFactory {
	var out []PluginFactory
	if f, ok := builtinPureShot(); ok {
		out = append(out, f)
	}
	if f, ok := builtinPureClip(); ok {
		out = append(out, f)
	}
	if f, ok := builtinPureImage(); ok {
		out = append(out, f)
	}
	if f, ok := builtinPureVideo(); ok {
		out = append(out, f)
	}
	if f, ok := builtinPureZip(); ok {
		out = append(out, f)
	}
	if f, ok := builtinPureMonitor(); ok {
		out = append(out, f)
	}
	return out
}
