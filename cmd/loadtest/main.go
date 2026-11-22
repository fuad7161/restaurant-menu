// Command loadtest simulates a busy evening against a running Bosho: many guests whose screens
// refresh every few seconds and who order from time to time, plus a kitchen screen that accepts
// and finishes the orders. It prints how fast each kind of request was answered.
//
// It places real orders, so run it only against a test or demo restaurant, never a live one:
//
//	go run ./cmd/loadtest -url http://localhost:8081 -guests 60 -duration 2m
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

var (
	base     = flag.String("url", "http://localhost:8081", "address of the running app")
	tables   = flag.String("tables", "demo-table-1,demo-table-2,demo-table-3,demo-table-4", "table QR tokens, comma separated")
	guests   = flag.Int("guests", 60, "guests at the same time (each one its own group)")
	duration = flag.Duration("duration", time.Minute, "how long to run")
	phone    = flag.String("phone", "01700000000", "staff login for the kitchen screen")
	password = flag.String("password", "demo1234", "staff password")
)

type stats struct {
	mu    sync.Mutex
	times map[string][]time.Duration
	fails map[string]int
}

func (s *stats) add(kind string, d time.Duration, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.times[kind] = append(s.times[kind], d)
	if !ok {
		s.fails[kind]++
	}
}

var st = &stats{times: map[string][]time.Duration{}, fails: map[string]int{}}

type client struct{ http *http.Client }

func newClient() *client {
	jar, _ := cookiejar.New(nil)
	return &client{&http.Client{Jar: jar, Timeout: 30 * time.Second}}
}

// do sends one request, records its time under kind, and returns the body.
func (c *client) do(kind, method, path string, form url.Values, htmx bool) string {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, _ := http.NewRequest(method, *base+path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		st.add(kind, time.Since(start), false)
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	st.add(kind, time.Since(start), resp.StatusCode < 400)
	return string(b)
}

var itemRe = regexp.MustCompile(`name="item_id" value="(\d+)"`)

func guest(n int, token string, until time.Time, wg *sync.WaitGroup) {
	defer wg.Done()
	c := newClient()
	tp := "/t/" + token
	c.do("landing", "GET", tp, nil, false)
	menu := c.do("enter", "POST", tp+"/enter", url.Values{"name": {fmt.Sprintf("Load %d", n)}, "choice": {"new"}}, false)
	var items []string
	for _, m := range itemRe.FindAllStringSubmatch(menu, -1) {
		items = append(items, m[1])
	}
	next := time.Now().Add(time.Duration(rand.IntN(30)) * time.Second)
	tick := time.NewTicker(4 * time.Second)
	defer tick.Stop()
	for time.Now().Before(until) {
		<-tick.C
		// Each guest screen refreshes its bottom bar and its current screen.
		c.do("poll bar", "GET", tp+"/bar?tab=menu", nil, true)
		c.do("poll orders", "GET", tp+"/orders/live", nil, true)
		if time.Now().After(next) && len(items) > 0 {
			for k := 0; k < 1+rand.IntN(3); k++ {
				c.do("add to cart", "POST", tp+"/cart/add", url.Values{"item_id": {items[rand.IntN(len(items))]}, "from": {"card"}, "page": {"menu"}, "confirm": {"1"}}, true)
			}
			c.do("place order", "POST", tp+"/order", url.Values{}, false)
			next = time.Now().Add(time.Duration(45+rand.IntN(45)) * time.Second)
		}
	}
}

var actRe = regexp.MustCompile(`hx-post="/staff/orders/(\d+)/(accept|ready|served)"`)

func kitchen(until time.Time, wg *sync.WaitGroup) {
	defer wg.Done()
	c := newClient()
	c.do("staff login", "POST", "/login", url.Values{"phone": {*phone}, "password": {*password}}, false)
	for time.Now().Before(until) {
		board := c.do("poll kitchen", "GET", "/staff/kitchen/board", nil, true)
		// Work one ticket per refresh, like a busy cook.
		if m := actRe.FindStringSubmatch(board); m != nil {
			c.do("kitchen "+m[2], "POST", "/staff/orders/"+m[1]+"/"+m[2], url.Values{"minutes": {"15"}}, true)
		}
		time.Sleep(3 * time.Second)
	}
}

func main() {
	flag.Parse()
	toks := strings.Split(*tables, ",")
	until := time.Now().Add(*duration)
	log.Printf("%d guests on %d tables and 1 kitchen screen for %s against %s", *guests, len(toks), *duration, *base)
	var wg sync.WaitGroup
	wg.Add(1)
	go kitchen(until, &wg)
	for i := 0; i < *guests; i++ {
		wg.Add(1)
		go guest(i, toks[i%len(toks)], until, &wg)
		time.Sleep(50 * time.Millisecond) // guests arrive over a few seconds
	}
	wg.Wait()

	kinds := make([]string, 0, len(st.times))
	for k := range st.times {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	fmt.Printf("\n%-16s %7s %7s %8s %8s %8s\n", "request", "count", "failed", "median", "95%", "slowest")
	bad := 0
	for _, k := range kinds {
		t := st.times[k]
		slices.Sort(t)
		p := func(q float64) time.Duration { return t[min(len(t)-1, int(q*float64(len(t))))] }
		fmt.Printf("%-16s %7d %7d %8s %8s %8s\n", k, len(t), st.fails[k], p(.5).Round(time.Millisecond), p(.95).Round(time.Millisecond), t[len(t)-1].Round(time.Millisecond))
		bad += st.fails[k]
	}
	if bad > 0 {
		os.Exit(1)
	}
}
