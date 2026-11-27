//go:build nopuremonitor

package host

const puremonitorCompiled = false

func builtinPureMonitor() (PluginFactory, bool) {
	return PluginFactory{}, false
}

// RunMonitorHelper returns false when PureMonitor is excluded at compile time.
func RunMonitorHelper(_ []string) (bool, int) {
	return false, 0
}
