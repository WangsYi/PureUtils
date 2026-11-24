//go:build nopureimage || nofreeimage

package host

const pureimageCompiled = false

func builtinPureImage() (PluginFactory, bool) {
	return PluginFactory{}, false
}
