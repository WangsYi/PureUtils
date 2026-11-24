package config

import "encoding/json"

// synthesizeLegacySection constructs a read-only plugin section payload from
// legacy v1 top-level settings.json keys without altering files on disk.
func synthesizeLegacySection(pluginID string, top map[string]json.RawMessage) (json.RawMessage, bool) {
	if len(top) == 0 {
		return nil, false
	}
	switch pluginID {
	case PluginPureShot:
		obj := map[string]json.RawMessage{}
		if v, ok := top["capture"]; ok && !isJSONNull(v) {
			obj["capture"] = v
		}
		if v, ok := top["pins"]; ok && !isJSONNull(v) {
			obj["pins"] = v
		}
		if len(obj) == 0 {
			return nil, false
		}
		b, err := json.Marshal(obj)
		if err != nil {
			return nil, false
		}
		return json.RawMessage(b), true

	case PluginPureClip:
		obj := map[string]json.RawMessage{}
		if v, ok := top["history"]; ok && !isJSONNull(v) {
			obj["history"] = v
		}
		if v, ok := pickNestedKey(top["appearance"], "previewHeight"); ok {
			obj["previewHeight"] = v
		}
		if v, ok := pickNestedKey(top["advanced"], "selfWriteSuppressMs"); ok {
			obj["selfWriteSuppressMs"] = v
		}
		if v, ok := pickNestedKey(top["advanced"], "debounceMs"); ok {
			obj["debounceMs"] = v
		}
		if len(obj) == 0 {
			return nil, false
		}
		b, err := json.Marshal(obj)
		if err != nil {
			return nil, false
		}
		return json.RawMessage(b), true

	case PluginPureImage:
		v, ok := top["viewer"]
		if !ok || isJSONNull(v) {
			return nil, false
		}
		return append(json.RawMessage(nil), v...), true

	case PluginPureZip:
		v, ok := top["zip"]
		if !ok || isJSONNull(v) {
			return nil, false
		}
		return append(json.RawMessage(nil), v...), true

	default:
		return nil, false
	}
}

func pickNestedKey(objRaw json.RawMessage, key string) (json.RawMessage, bool) {
	if isJSONNull(objRaw) {
		return nil, false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(objRaw, &m); err != nil {
		return nil, false
	}
	v, ok := m[key]
	if !ok || isJSONNull(v) {
		return nil, false
	}
	return append(json.RawMessage(nil), v...), true
}
