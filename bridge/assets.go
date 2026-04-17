package main

import "strings"

const maxAssetURL = 256

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func normAsset(s string) string {
	if isURL(s) && len(s) > maxAssetURL {
		return ""
	}
	return s
}

func transformAssets(a *Activity) {
	if a == nil || a.Assets == nil {
		return
	}
	a.Assets.LargeImage = normAsset(a.Assets.LargeImage)
	a.Assets.SmallImage = normAsset(a.Assets.SmallImage)
}
