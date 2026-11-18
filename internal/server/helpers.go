package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bosho/internal/i18n"
	"bosho/internal/money"
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

func ago(ts int64) string {
	d := now() - ts
	switch {
	case d < 60:
		return "just now"
	case d < 3600:
		return fmt.Sprintf("%d min ago", d/60)
	default:
		return fmt.Sprintf("%dh %dm ago", d/3600, (d%3600)/60)
	}
}

func back(w http.ResponseWriter, r *http.Request, to string) {
	http.Redirect(w, r, to, http.StatusSeeOther)
}

var templateFuncs = template.FuncMap{
	"money": money.Format,
	"taka":  money.Taka,
	"t":     i18n.T,
	"ago":   ago,
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
	"dishart": dishArt,
	// initial is the first letter of a name, for the round logo mark.
	"initial": func(s string) string {
		for _, r := range strings.TrimSpace(s) {
			return strings.ToUpper(string(r))
		}
		return "ব"
	},
	"rev": func(n int) []int { // n, n-1, …, 1
		s := make([]int, n)
		for i := range s {
			s[i] = n - i
		}
		return s
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
