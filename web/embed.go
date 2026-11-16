package web

import "embed"

//go:embed index.html settings.html showcase.html style.css css/*.css api.js ui.js app.js settings.js showcase.js
var Assets embed.FS
