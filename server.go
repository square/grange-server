package main

import (
	"encoding/json"
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
	state         *grange.RangeState
	configPath    string
)

func queryHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	// TODO: Handle error?
	q, _ := url.QueryUnescape(r.URL.RawQuery)

	// Useful if a query is crashing. Default log line is post-process though
	// so that timing information is front and center.
	Debug("PREQUERY %s %s", r.RemoteAddr, q)

	result, err := grange.EvalRange(q, state)

	if err == nil {
		for x := range result.Iter() {
			fmt.Fprint(w, x, "\n")
		}
	} else {
		http.Error(w, fmt.Sprintf("%s", err.Error()), 422)
	}

	Info("QUERY %s %.3f \"%s\"", r.RemoteAddr, time.Now().Sub(now).Seconds(), q)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"status": "ok",
	}
	str, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(str)
	w.Write([]byte("\n")) // Be nice to curl
}

func init() {
	flag.IntVar(&port, "port", 8080, "HTTP Server Port")
	flag.BoolVar(&parse, "parse", false, "do not start server. Non-zero exit code on parse warnings.")
	flag.Parse()
}

func main() {
	Info("Hello friends, server starting with PID %d", os.Getpid())
	configPath = "grange.yaml" // TODO: Read from command line

	warnings := loadConfig(configPath)
	httpAddr := fmt.Sprintf(":%v", port)

	if warnings < 0 {
		os.Exit(1)
	}

	go handleSignals()

	if parse {
		Info("Not starting server because of -parse option")
		if warnings > 0 {
			os.Exit(1)
		}
	} else {
		Info("Listening to %v", httpAddr)

		http.HandleFunc("/_status", statusHandler)
		http.HandleFunc("/", queryHandler)
		Fatal("Server crashed: %s",
			http.ListenAndServe(httpAddr, http.DefaultServeMux))
		os.Exit(1)
	}
}

// Dynamically reloadable server configuration.
type serverConfig struct {
	loglevel string
	yamlpath string
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

// Returns number of warnings emited while loading, or negative number for
// fatal error.
func loadConfig(path string) int {
	dat, err := ioutil.ReadFile(path) // TODO: error handling
	if err != nil {
		Fatal("Could not read config file: %s", path)
		Fatal(err.Error())
		return -1
	}
	var config map[string]interface{} // TODO: Why can't deserialize directly into config struct?
	_ = yaml.Unmarshal(dat, &config)

	// TODO: Validate config

	currentConfig.loglevel = config["loglevel"].(string)
	currentConfig.yamlpath = config["yamlpath"].(string)
	setLogLevel(currentConfig.loglevel)

	newState, warnings := loadState()
	state = newState
	return warnings
}

func loadState() (*grange.RangeState, int) {
	state := grange.NewState()
	dir := currentConfig.yamlpath
	warnings := 0

	Info("Loading state from YAML in path: %s", dir)

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
		c, w := yamlToCluster(name, m)
		warnings += w
		if len(c) == 0 {
			Warn("%%%s is empty, discarding", name)
			warnings++
		} else {
			if name == "GROUPS" {
				grange.SetGroups(&state, c)
			} else {
				grange.AddCluster(&state, name, c)
			}
		}
	}

	return &state, warnings
}

// Converts a generic YAML map to a cluster by extracting all the correctly
// typed strings and discarding invalid values.
func yamlToCluster(clusterName string, yaml map[string]interface{}) (grange.Cluster, int) {
	c := grange.Cluster{}

	warnings := 0

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
					warnings++
				}
			}
			c[key] = result
		default:
			Warn("Discarding invalid key %%%s:%s", clusterName, key)
			warnings++
		}
	}
	return c, warnings
}
