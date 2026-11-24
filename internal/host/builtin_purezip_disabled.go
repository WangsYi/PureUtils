//go:build nopurezip

package host

const purezipCompiled = false

func builtinPureZip() (PluginFactory, bool) {
	return PluginFactory{}, false
}
