package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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
	Info("Hello friends, server starting...")

	loadState()
	httpAddr := fmt.Sprintf(":%v", port)

	if parse {
		Info("Not starting server because of -parse option")
	} else {
		Info("Listening to %v", httpAddr)

		http.HandleFunc("/", rootHandler)
		Fatal("Server crashed: %s",
			http.ListenAndServe(httpAddr, logRequests(http.DefaultServeMux)))
		os.Exit(1)
	}
}

func logRequests(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.QueryUnescape(r.URL.RawQuery)
		Info("QUERY %s %s", r.RemoteAddr, q)
		handler.ServeHTTP(w, r)
	})
}

func loadState() {
	Info("Start YAML load")

	state = grange.NewState()
	dir := "clusters" // TODO: Configurable

	files, _ := ioutil.ReadDir(dir)
	for _, f := range files {
		basename := f.Name()
		ext := filepath.Ext(basename)
		if ext != ".yaml" {
			continue
		}
		name := strings.TrimSuffix(basename, ext)
		fullpath := dir + "/" + basename
		Debug("Loading %%%s from %s", name, fullpath)

		dat, _ := ioutil.ReadFile(fullpath) // TODO: Cross-platform file join?

		m := make(map[string]interface{})
		_ = yaml.Unmarshal(dat, &m)
		c := yamlToCluster(name, m)
		if len(c) == 0 {
			Warn("%%%s is empty, discarding", name)
		} else {
			grange.AddCluster(state, name, c)
		}
	}
	Info("Finish YAML load")
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
					Warn("Discarding invalid value '%v' in %%%s:%s",
						x, clusterName, key)
				}
			}
			c[key] = result
		default:
			Warn("Discarding invalid key %%%s:%s", clusterName, key)
		}
	}
	return c
}
