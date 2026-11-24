package pureimage

import "strings"

var texts = map[string]map[string]string{
	"zh-CN": {
		"open":              "打开图片",
		"openFolder":        "打开文件夹",
		"prev":              "上一张",
		"next":              "下一张",
		"rotate":            "旋转",
		"flip":              "水平翻转",
		"fit":               "适配窗口",
		"zoomIn":            "放大",
		"zoomOut":           "缩小",
		"actual":            "原始大小",
		"slideshow":         "幻灯片",
		"details":           "详细信息",
		"pinToScreen":       "贴到屏幕",
		"delete":            "删除到回收站",
		"sort":              "排序",
		"settings":          "设置",
		"copyPath":          "复制文件路径",
		"reveal":            "在资源管理器中显示",
		"close":             "关闭",
		"recursive":         "包含子目录（递归）",
		"wheelZoom":         "滚轮缩放（关闭则滚轮切换）",
		"checkerboard":      "透明区域棋盘格",
		"showDetails":       "图片详细信息（左上角）",
		"spacePreview":      "空格预览（资源管理器选中按空格）",
		"fileAssoc":         "加入系统“打开方式”列表（全部图片格式）",
		"emptyHint":         "把图片或文件夹拖进窗口，或使用下面的按钮打开",
		"shortcutHint":      "← → 切换　R 旋转　H 翻转　S 幻灯片　P 贴图　Del 删除　F11 全屏　D/0 适配　1 原始大小",
		"sortNameAsc":       "名称升序",
		"sortNameDesc":      "名称降序",
		"sortModifiedAsc":   "修改时间升序",
		"sortModifiedDesc":  "修改时间降序",
		"sortSizeAsc":       "大小升序",
		"sortSizeDesc":      "大小降序",
		"sortReshuffle":     "重新随机",
		"defaultApps":       "打开系统默认应用设置",
		"slideshowInterval": "幻灯片间隔",
		"preload":           "预读数量",
		"preloadOff":        "关闭",
		"preloadOne":        "预读 1 张",
		"preloadTwo":        "预读 2 张",
		"preloadThree":      "预读 3 张",
		"animation":         "动画",
		"previewLevel":      "（预览版）",
		"language":          "语言",
		"languageZh":        "简体中文",
		"languageEn":        "English",
		"languageSystem":    "跟随系统",
		"sortName":          "名称",
		"sortModified":      "修改时间",
		"sortSize":          "大小",
		"sortRandom":        "随机",
		"sortDesc":          "降序",
		"dimensions":        "尺寸",
		"size":              "大小",
		"modified":          "修改",
		"camera":            "相机",
		"taken":             "拍摄",
		"exposure":          "曝光",
		"iso":               "ISO",
		"focal":             "焦距",
		"deleted":           "已删除",
		"copied":            "路径已复制",
		"pinned":            "已贴到屏幕",
		"fileAssocDone":     "已加入打开方式列表",
		"fileAssocFailed":   "加入打开方式失败",
		"cannotDecode":      "无法解码",
		"noImages":          "此目录没有支持的图片",
		"previewOn":         "空格预览已开启",
		"previewOff":        "空格预览已关闭",
	},
	"en-US": {
		"open":              "Open image",
		"openFolder":        "Open folder",
		"prev":              "Previous",
		"next":              "Next",
		"rotate":            "Rotate",
		"flip":              "Flip horizontally",
		"fit":               "Fit window",
		"zoomIn":            "Zoom in",
		"zoomOut":           "Zoom out",
		"actual":            "Actual size",
		"slideshow":         "Slideshow",
		"details":           "Details",
		"pinToScreen":       "Pin to screen",
		"delete":            "Move to Recycle Bin",
		"sort":              "Sort",
		"settings":          "Settings",
		"copyPath":          "Copy file path",
		"reveal":            "Show in Explorer",
		"close":             "Close",
		"recursive":         "Include subfolders (recursive)",
		"wheelZoom":         "Wheel zoom (off: wheel switches images)",
		"checkerboard":      "Checkerboard for transparency",
		"showDetails":       "Image details (top left)",
		"spacePreview":      "Space preview (selected in Explorer)",
		"fileAssoc":         "Add to system \"Open with\" (all image formats)",
		"emptyHint":         "Drop an image or folder here, or use the buttons below",
		"shortcutHint":      "← → switch   R rotate   H flip   S slideshow   P pin   Del delete   F11 full screen   D/0 fit   1 actual",
		"sortNameAsc":       "Name ascending",
		"sortNameDesc":      "Name descending",
		"sortModifiedAsc":   "Modified ascending",
		"sortModifiedDesc":  "Modified descending",
		"sortSizeAsc":       "Size ascending",
		"sortSizeDesc":      "Size descending",
		"sortReshuffle":     "Reshuffle",
		"defaultApps":       "Open Windows default apps",
		"slideshowInterval": "Slideshow interval",
		"preload":           "Preload count",
		"preloadOff":        "Off",
		"preloadOne":        "Preload 1",
		"preloadTwo":        "Preload 2",
		"preloadThree":      "Preload 3",
		"animation":         "Animation",
		"previewLevel":      " (preview)",
		"language":          "Language",
		"languageZh":        "简体中文",
		"languageEn":        "English",
		"languageSystem":    "Follow system",
		"sortName":          "Name",
		"sortModified":      "Modified",
		"sortSize":          "Size",
		"sortRandom":        "Random",
		"sortDesc":          "Descending",
		"dimensions":        "Dimensions",
		"size":              "Size",
		"modified":          "Modified",
		"camera":            "Camera",
		"taken":             "Taken",
		"exposure":          "Exposure",
		"iso":               "ISO",
		"focal":             "Focal length",
		"deleted":           "Deleted",
		"copied":            "Path copied",
		"pinned":            "Pinned to screen",
		"fileAssocDone":     "Added to \"Open with\"",
		"fileAssocFailed":   "Could not add file association",
		"cannotDecode":      "Cannot decode",
		"noImages":          "No supported images in this folder",
		"previewOn":         "Space preview on",
		"previewOff":        "Space preview off",
	},
}

func resolveLang(p string) string {
	if p == "system" {
		return "zh-CN"
	}
	if _, ok := texts[p]; ok {
		return p
	}
	return "zh-CN"
}

func tr(langPref, key string) string {
	lang := resolveLang(langPref)
	if m, ok := texts[lang]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	if m, ok := texts["zh-CN"]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	return key
}

func langLabel(langPref, code string) string {
	switch code {
	case "zh-CN":
		return tr(langPref, "languageZh")
	case "en-US":
		return tr(langPref, "languageEn")
	case "system":
		return tr(langPref, "languageSystem")
	}
	return strings.ToUpper(code)
}
