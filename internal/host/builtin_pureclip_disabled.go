//go:build nopureclip

package host

const pureclipCompiled = false

func builtinPureClip() (PluginFactory, bool) {
	return PluginFactory{}, false
}
