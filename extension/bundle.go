package extensionbundle

import "embed"

// Files is the reviewed extension bundle shipped inside coupangctl releases.
//
//go:embed action.js manifest.json page-reader.js popup.css popup.html popup.js service-worker.js
var Files embed.FS

var Filenames = []string{
	"manifest.json",
	"action.js",
	"page-reader.js",
	"popup.html",
	"popup.css",
	"popup.js",
	"service-worker.js",
}
