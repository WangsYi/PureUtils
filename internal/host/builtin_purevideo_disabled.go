//go:build nopurevideo

package host

const purevideoCompiled = false

func builtinPureVideo() (PluginFactory, bool) {
	return PluginFactory{}, false
}
