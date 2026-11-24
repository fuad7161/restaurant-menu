package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bosho/internal/database"
	"bosho/internal/i18n"
	"bosho/internal/money"
	"bosho/internal/storage"
	"bosho/web"
)

// M is the data handed to a template.
type M = map[string]any

func now() int64 { return time.Now().Unix() }

func randToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// clean trims, collapses whitespace and limits the length of user text.
func clean(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return s
}

func pathID(r *http.Request, k string) int64 {
	n, _ := strconv.ParseInt(r.PathValue(k), 10, 64)
	return n
}

func queryID(r *http.Request, k string) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get(k), 10, 64)
	return n
}

// initial is the first letter of a name, shown in the round mark when there is no logo.
func initial(s string) string {
	for _, r := range strings.TrimSpace(s) {
		return strings.ToUpper(string(r))
	}
	return "ব"
}

func ago(ts int64) string { return agoIn("en", ts) }

// agoIn is "5 min ago" in English or Bangla.
func agoIn(lang string, ts int64) string {
	d := now() - ts
	switch {
	case d < 60:
		return i18n.T(lang, "s_just_now")
	case d < 3600:
		return fmt.Sprintf(i18n.T(lang, "s_min_ago"), d/60)
	default:
		return fmt.Sprintf(i18n.T(lang, "s_h_ago"), d/3600, (d%3600)/60)
	}
}

// pct prints a percentage without trailing zeros: 5, 7.5.
func pct(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// assetVersions holds a short content hash of every static file. Links made with the asset
// template function carry it as ?v=, so browsers may cache a file for a year and still fetch the
// new one as soon as it changes.
var assetVersions = func() map[string]string {
	v := map[string]string{}
	fs.WalkDir(web.Static, "static", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, err := fs.ReadFile(web.Static, path); err == nil {
				h := sha256.Sum256(b)
				v[strings.TrimPrefix(path, "static/")] = hex.EncodeToString(h[:5])
			}
		}
		return nil
	})
	return v
}()

func asset(name string) string {
	if v, ok := assetVersions[name]; ok {
		return "/static/" + name + "?v=" + v
	}
	return "/static/" + name
}

// dishTags are the tags an owner can give a dish, in the order guests see them.
var dishTags = []string{"chef", "new", "veg", "spicy1", "spicy2", "spicy3"}

func hasTag(tags, t string) bool {
	for _, x := range strings.Split(tags, ",") {
		if x == t {
			return true
		}
	}
	return false
}

// itemTags lists the tag keys to show on a dish: Popular first, then the owner's tags.
func itemTags(it Item) []string {
	var out []string
	if it.Popular {
		out = append(out, "popular")
	}
	for _, t := range dishTags {
		if hasTag(it.Tags, t) {
			out = append(out, t)
		}
	}
	return out
}

func back(w http.ResponseWriter, r *http.Request, to string) {
	http.Redirect(w, r, to, http.StatusSeeOther)
}

var templateFuncs = template.FuncMap{
	"money": money.Format,
	"taka":  money.Taka,
	"t":     i18n.T,
	"ago":   ago,
	"agoIn": agoIn,
	"asset": asset,
	"pct":   pct,
	// small is the menu-card size of a dish photo; srcset offers both sizes when the photo has them.
	"small": storage.SmallName,
	"srcset": func(name string) string {
		if !storage.HasSizes(name) {
			return ""
		}
		return "/uploads/" + storage.SmallName(name) + " 480w, /uploads/" + name + " 1200w"
	},
	"lower": strings.ToLower,
	// title capitalises the first letter: "manager" → "Manager".
	"title": func(s string) string {
		if s == "" {
			return s
		}
		return strings.ToUpper(s[:1]) + s[1:]
	},
	"itemTags": itemTags,
	"hasTag":   hasTag,
	"dishTags": func() []string { return dishTags },
	// tf is t with fmt arguments: {{tf .Lang "s_table" .Label}}
	"tf": func(lang, key string, args ...any) string { return fmt.Sprintf(i18n.T(lang, key), args...) },
	// pick chooses the Bangla or English name, falling back to whichever exists.
	"pick": func(lang, bn, en string) string {
		if lang == "bn" && bn != "" {
			return bn
		}
		if en != "" {
			return en
		}
		return bn
	},
	"dict": func(v ...any) M {
		m := M{}
		for i := 0; i+1 < len(v); i += 2 {
			m[v[i].(string)] = v[i+1]
		}
		return m
	},
	"dishart":    dishArt,
	"theme":      themeCSS,
	"themecolor": themeColor,
	// initial is the first letter of a name, for the round logo mark.
	"initial": initial,
	"rev": func(n int) []int { // n, n-1, …, 1
		s := make([]int, n)
		for i := range s {
			s[i] = n - i
		}
		return s
	},
	"list": func(v ...int) []int { return v },
	// words is list for strings: {{range (words "a" "b")}}
	"words": func(v ...string) []string { return v },
	"div": func(a int64, b int) int64 {
		if b == 0 {
			return 0
		}
		return a / int64(b)
	},
	"seq": func(n int) []int {
		s := make([]int, n)
		for i := range s {
			s[i] = i + 1
		}
		return s
	},
}

// dishArt picks a bundled illustration for a dish without a photo, by matching
// words in the dish and category names (Bangla or English). Dish names are checked first.
var dishArtWords = []struct {
	art   string
	words []string
}{
	{"biryani", []string{"biryani", "biriyani", "kacchi", "tehari", "pulao", "polao", "khichuri", "rice", "বিরিয়ানি", "কাচ্চি", "তেহারি", "পোলাও", "খিচুড়ি", "ভাত"}},
	{"kebab", []string{"kebab", "kabab", "tikka", "grill", "bbq", "chaap", "chap", "naan", "কাবাব", "টিক্কা", "গ্রিল", "চাপ"}},
	{"fish", []string{"fish", "ilish", "hilsa", "rui", "prawn", "shrimp", "chingri", "মাছ", "ইলিশ", "রুই", "চিংড়ি"}},
	{"tea", []string{"tea", "chai", "coffee", "কফি"}},
	{"drink", []string{"drink", "borhani", "lassi", "lacchi", "juice", "water", "shake", "soda", "cola", "pani", "পানীয়", "বোরহানি", "লাচ্ছি", "জুস", "পানি"}},
	{"dessert", []string{"dessert", "sweet", "firni", "payesh", "doi", "mishti", "halwa", "ice cream", "kheer", "মিষ্টি", "ফিরনি", "পায়েস", "দই", "হালুয়া"}},
	{"curry", []string{"curry", "bhuna", "rezala", "korma", "dal", "daal", "beef", "mutton", "chicken", "tarkari", "ভুনা", "রেজালা", "কোরমা", "ডাল", "গরু", "খাসি", "মুরগি", "তরকারি"}},
}

func dishArt(names ...string) string {
	for _, n := range names {
		n = strings.ToLower(n)
		for _, a := range dishArtWords {
			for _, w := range a.words {
				if strings.Contains(n, w) {
					return a.art
				}
			}
		}
	}
	return "thali"
}

// inTx runs fn inside one transaction: everything fn writes is saved together, or nothing is.
func (a *App) inTx(fn func(q database.Querier) error) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// exec runs one write and returns only its error, for use inside inTx.
func exec(q database.Querier, query string, args ...any) error {
	_, err := q.Exec(query, args...)
	return err
}

// serverError answers a failed write with 500, so staff and guests see "Something went wrong"
// (app.js shows it as a message) instead of a screen that looks as if it worked.
func serverError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "error", err)
	http.Error(w, "Something went wrong. Please try again.", http.StatusInternalServerError)
}
