//go:build nopureshot

package host

const pureshotCompiled = false

func builtinPureShot() (PluginFactory, bool) {
	return PluginFactory{}, false
}
