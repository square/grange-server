package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/v1/yaml"

	"github.com/xaviershay/grange"
)

var (
	port  int
	state grange.RangeState
)

func rootHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Handle error
	q, _ := url.QueryUnescape(r.URL.RawQuery)

	result, err := grange.EvalRange(q, &state)

	if err == nil {
		fmt.Fprint(w, strings.Join(result, "\n"))
	} else {
		http.Error(w, fmt.Sprintf("%s", err.Error()), 422)
	}
}

func init() {
	flag.IntVar(&port, "port", 8080, "HTTP Server Port")
	flag.Parse()
}

func main() {
	log.Printf("Hello!")

	loadState()
	httpAddr := fmt.Sprintf(":%v", port)

	log.Printf("Listening to %v", httpAddr)

	http.HandleFunc("/", rootHandler)
	log.Fatal(http.ListenAndServe(httpAddr, logRequests(http.DefaultServeMux)))
}

func trace(s string) (string, time.Time) {
	return s, time.Now()
}

func un(s string, startTime time.Time) {
	endTime := time.Now()
	log.Printf("%s in %.4fs", s, endTime.Sub(startTime).Seconds())
}

func logRequests(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.QueryUnescape(r.URL.RawQuery)
		log.Printf("%s %s", r.RemoteAddr, q)
		handler.ServeHTTP(w, r)
	})
}

func loadState() {
	defer un(trace("Loaded YAML"))

	state = grange.NewState()

	files, _ := ioutil.ReadDir("./clusters") // TODO: Configurable
	for _, f := range files {
		basename := f.Name()
		name := strings.TrimSuffix(basename, filepath.Ext(basename))

		dat, _ := ioutil.ReadFile("clusters/" + basename)
		var c grange.Cluster

		_ = yaml.Unmarshal(dat, &c)
		grange.AddCluster(state, name, c)
	}
}
