package extensionbundle

import "embed"

// Files is the reviewed extension bundle shipped inside coupangctl releases.
//
//go:embed action.js icon16.png icon48.png icon128.png manifest.json page-reader.js popup.css popup.html popup.js service-worker.js
var Files embed.FS

var Filenames = []string{
	"manifest.json",
	"action.js",
	"icon16.png",
	"icon48.png",
	"icon128.png",
	"page-reader.js",
	"popup.html",
	"popup.css",
	"popup.js",
	"service-worker.js",
}
