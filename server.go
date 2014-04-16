package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gopkg.in/v1/yaml"

	"github.com/xaviershay/grange"
)

var (
	currentConfig serverConfig
	port          int
	parse         bool
	state         grange.RangeState
	configPath    string
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
	configPath = "grange.yaml" // TODO: Read from command line

	loadConfig(configPath)
	loadState()
	httpAddr := fmt.Sprintf(":%v", port)

	go handleSignals()

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

// Dynamically reloadable server configuration.
type serverConfig struct {
	loglevel string
}

func handleSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)
	for sig := range c {
		switch sig {
		case syscall.SIGHUP:
			Info("Reloading config in response to HUP")
			loadConfig(configPath)
		}
	}
}

func loadConfig(path string) {
	dat, err := ioutil.ReadFile(path) // TODO: error handling
	if err != nil {
		panic(err)
	}
	var config map[string]interface{} // TODO: Why can't deserialize directly into config struct?
	_ = yaml.Unmarshal(dat, &config)

	// TODO: Validate config

	currentConfig.loglevel = config["loglevel"].(string)
	setLogLevel(currentConfig.loglevel)
}

func logRequests(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.QueryUnescape(r.URL.RawQuery)

		// Useful if a query is crashing. Default log line is post-process though
		// so that timing information is front and center.
		Debug("PREQUERY %s %s", r.RemoteAddr, q)

		now := time.Now()
		handler.ServeHTTP(w, r)

		Info("QUERY %s %.3f \"%s\"", r.RemoteAddr, time.Now().Sub(now).Seconds(), q)
	})
}

func loadState() {
	Info("Loading state from YAML")

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
			if name == "GROUPS" {
				grange.SetGroups(&state, c)
			} else {
				grange.AddCluster(&state, name, c)
			}
		}
	}
}

// Converts a generic YAML map to a cluster by extracting all the correctly
// typed strings and discarding invalid values.
func yamlToCluster(clusterName string, yaml map[string]interface{}) grange.Cluster {
	c := grange.Cluster{}

	for key, value := range yaml {
		switch value.(type) {
		case nil:
			c[key] = []string{}
		case string:
			c[key] = []string{value.(string)}
		case int:
			c[key] = []string{fmt.Sprintf("%d", value.(int))}
		case []interface{}:
			result := []string{}

			for _, x := range value.([]interface{}) {
				switch x.(type) {
				case string:
					result = append(result, fmt.Sprintf("%s", x))
				case int:
					result = append(result, fmt.Sprintf("%d", x))
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
