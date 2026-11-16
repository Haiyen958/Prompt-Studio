package handlers_test

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"prompt-studio/web"
)

// Guards frontend DOM wiring and CSS Design System invariants across embedded web assets:
// every el("...") lookup in app.js/settings.js must match a real id="..." in its HTML page,
// CSS/JS/HTML must contain zero inline styles, zero !important, zero #id selectors, and
// zero hardcoded #hex / rgb() / rgba() colors outside 01-tokens.css and 07-themes.css.
func TestFrontendDOMIDsAndDesignSystemInvariants(t *testing.T) {
	readAsset := func(name string) string {
		t.Helper()
		b, err := fs.ReadFile(web.Assets, name)
		if err != nil {
			t.Fatalf("read asset %s: %v", name, err)
		}
		return string(b)
	}

	elCallRe := regexp.MustCompile(`\bel\(\s*"([A-Za-z0-9_-]+)"\s*\)`)
	idAttrRe := regexp.MustCompile(`\bid="([A-Za-z0-9_-]+)"`)

	checkPageIDs := func(jsFile, htmlFile string) {
		jsSrc := readAsset(jsFile)
		htmlSrc := readAsset(htmlFile)
		ids := map[string]bool{}
		for _, m := range idAttrRe.FindAllStringSubmatch(htmlSrc, -1) {
			ids[m[1]] = true
		}
		for _, m := range elCallRe.FindAllStringSubmatch(jsSrc, -1) {
			wantID := m[1]
			if !ids[wantID] {
				t.Errorf("%s calls el(%q), but #%s does not exist in %s", jsFile, wantID, wantID, htmlFile)
			}
		}
	}

	checkPageIDs("app.js", "index.html")
	checkPageIDs("settings.js", "settings.html")

	// Zero inline style mutations in JS & HTML
	for _, jsFile := range []string{"app.js", "settings.js", "ui.js", "api.js", "showcase.js"} {
		src := readAsset(jsFile)
		if strings.Contains(src, ".style.") || strings.Contains(src, `style="`) {
			t.Errorf("%s contains forbidden inline style usage", jsFile)
		}
	}
	for _, htmlFile := range []string{"index.html", "settings.html", "showcase.html"} {
		if strings.Contains(readAsset(htmlFile), `style="`) {
			t.Errorf("%s contains forbidden inline style attribute", htmlFile)
		}
	}

	// CSS hygiene: zero !important, zero #id selectors, and zero raw colors outside token files
	cssFiles, err := fs.Glob(web.Assets, "css/*.css")
	if err != nil || len(cssFiles) == 0 {
		t.Fatalf("glob css/*.css: %v", err)
	}
	commentRe := regexp.MustCompile(`(?s)/\*.*?\*/`)
	hexColorRe := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	rgbFuncRe := regexp.MustCompile(`\brgba?\s*\(`)
	tokenDefRe := regexp.MustCompile(`(--[A-Za-z0-9_-]+)\s*:`)

	isTokenFile := func(cf string) bool {
		return cf == "css/01-tokens.css" || cf == "css/07-themes.css"
	}

	for _, cf := range cssFiles {
		clean := commentRe.ReplaceAllString(readAsset(cf), "")
		if strings.Contains(clean, "!important") {
			t.Errorf("%s contains forbidden !important", cf)
		}
		withoutHex := hexColorRe.ReplaceAllString(clean, "")
		if strings.Contains(withoutHex, "#") {
			t.Errorf("%s appears to contain a forbidden #id selector", cf)
		}
		if !isTokenFile(cf) {
			if m := hexColorRe.FindString(clean); m != "" {
				t.Errorf("%s contains hardcoded hex color %q outside 01-tokens.css/07-themes.css", cf, m)
			}
			if m := rgbFuncRe.FindString(clean); m != "" {
				t.Errorf("%s contains hardcoded %q outside 01-tokens.css/07-themes.css", cf, m)
			}
		}
	}

	// Every theme override token in 07-themes.css must exist in 01-tokens.css (:root).
	rootTokens := map[string]bool{}
	for _, m := range tokenDefRe.FindAllStringSubmatch(commentRe.ReplaceAllString(readAsset("css/01-tokens.css"), ""), -1) {
		rootTokens[m[1]] = true
	}
	for _, m := range tokenDefRe.FindAllStringSubmatch(commentRe.ReplaceAllString(readAsset("css/07-themes.css"), ""), -1) {
		if !rootTokens[m[1]] {
			t.Errorf("07-themes.css overrides unknown token %q missing from 01-tokens.css", m[1])
		}
	}

	// P015: Verify all 6 curated accents are wired in settings.html and 07-themes.css (Dark + Light).
	settingsHTML := readAsset("settings.html")
	themesCSS := commentRe.ReplaceAllString(readAsset("css/07-themes.css"), "")
	for _, acc := range []string{"crimson", "indigo", "ocean", "emerald", "violet", "amber"} {
		if !strings.Contains(settingsHTML, `data-accent="`+acc+`"`) {
			t.Errorf("settings.html missing swatch button for data-accent=%q", acc)
		}
		if acc == "crimson" {
			continue // crimson is default in 01-tokens.css (:root) and html[data-theme="light"]
		}
		darkSel := `html:not([data-theme="light"])[data-accent="` + acc + `"]`
		lightSel := `html[data-theme="light"][data-accent="` + acc + `"]`
		if !strings.Contains(themesCSS, darkSel) {
			t.Errorf("07-themes.css missing dark accent block %s", darkSel)
		}
		if !strings.Contains(themesCSS, lightSel) {
			t.Errorf("07-themes.css missing light accent block %s", lightSel)
		}
	}
}
