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
	parse bool
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
	flag.BoolVar(&parse, "parse", false, "do not start server")
	flag.Parse()
}

func main() {
	log.Printf("Hello friends, server starting...")

	loadState()
	httpAddr := fmt.Sprintf(":%v", port)

	if parse {
		log.Printf("Not starting server because of -parse option")
	} else {
		log.Printf("Listening to %v", httpAddr)

		http.HandleFunc("/", rootHandler)
		log.Fatal(http.ListenAndServe(httpAddr, logRequests(http.DefaultServeMux)))
	}
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
		ext := filepath.Ext(basename)
		if ext != ".yaml" {
			continue
		}
		name := strings.TrimSuffix(basename, ext)

		dat, _ := ioutil.ReadFile("clusters/" + basename)

		m := make(map[string]interface{})
		_ = yaml.Unmarshal(dat, &m)
		c := yamlToCluster(name, m)
		if len(c) == 0 {
			log.Printf("%%%s is empty, discarding", name)
		} else {
			grange.AddCluster(state, name, c)
		}
	}
}

// Converts a generic YAML map to a cluster by extracting all the correctly
// typed strings and discarding invalid values.
func yamlToCluster(clusterName string, yaml map[string]interface{}) grange.Cluster {
	c := grange.Cluster{}

	for key, value := range yaml {
		switch value.(type) {
		case string:
			c[key] = []string{value.(string)}
		case []interface{}:
			result := []string{}

			for _, x := range value.([]interface{}) {
				switch x.(type) {
				case string:
					result = append(result, fmt.Sprintf("%s", x))
				default:
					log.Printf("Discarding invalid value '%v' in %%%s:%s",
						x, clusterName, key)
				}
			}
			c[key] = result
		default:
			log.Printf("Discarding invalid key %%%s:%s", clusterName, key)
		}
	}
	return c
}
