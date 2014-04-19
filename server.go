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
	"path"
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
	help          bool
	state         *grange.State
	configPath    string
)

func queryHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	q, err := url.QueryUnescape(r.URL.RawQuery)
	if err != nil {
		http.Error(w, fmt.Sprintf("Could not unescape: %s", r.URL.RawQuery), 422)
		return
	}

	// Useful if a query is crashing. Default log line is post-process though
	// so that timing information is front and center.
	Debug("PREQUERY %s %s", r.RemoteAddr, q)

	result, err := state.Query(q)

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
	Debug("STATUS /_status")

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
	flag.Usage = func() {
		fmt.Println("  usage: grange-server [opts] [CONFIGFILE]")
		fmt.Println("example: grange-server -port=8888 grange.yaml")
		fmt.Println()

		flag.PrintDefaults()

		fmt.Println()
	}
	flag.IntVar(&port, "port", 8080, "HTTP Server Port")
	flag.BoolVar(&parse, "parse", false, "Do not start server. Non-zero exit code on parse warnings.")
	flag.BoolVar(&help, "help", false, "Show this message.")
	flag.Parse()
}

func main() {
	if help {
		flag.Usage()
		os.Exit(1)
	}

	Info("Hello friends, server starting with PID %d", os.Getpid())
	switch flag.NArg() {
	case 0:
		Info("No config file in arguments, using default config")
		configPath = ""
	case 1:
		configPath = flag.Arg(0)
		Info("Using config file: %s", configPath)
	default:
		flag.Usage()
		os.Exit(1)
	}

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
	if len(path) > 0 {
		dat, err := ioutil.ReadFile(path)
		if err != nil {
			Fatal("Could not read config file: %s", path)
			Fatal(err.Error())
			return -1
		}
		var config map[string]interface{}
		_ = yaml.Unmarshal(dat, &config)

		if config["loglevel"] != nil {
			Debug("Setting loglevel from config: %s", config["loglevel"])
			currentConfig.loglevel = config["loglevel"].(string)
		} else {
			Debug("No loglevel found in config: %s")
		}

		if config["yamlpath"] != nil {
			Debug("Setting yamlpath from config: %s", config["yamlpath"])
			currentConfig.yamlpath = config["yamlpath"].(string)
		} else {
			Debug("No yamlpath found in config: %s")
		}
		setLogLevel(currentConfig.loglevel)
	} else {
		// No config file, use defaults
		currentConfig.loglevel = "INFO"
		currentConfig.yamlpath = "clusters"
		setLogLevel(currentConfig.loglevel)
	}

	newState, warnings := loadState()
	newState.PrimeCache()
	state = newState
	return warnings
}

func loadState() (*grange.State, int) {
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
		fullpath := path.Join(dir, basename)
		Debug("Loading %%%s from %s", name, fullpath)

		dat, _ := ioutil.ReadFile(fullpath)

		m := make(map[string]interface{})
		_ = yaml.Unmarshal(dat, &m)
		c, w := yamlToCluster(name, m)
		warnings += w
		if len(c) == 0 {
			Warn("%%%s is empty, discarding", name)
			warnings++
		} else {
			if name == "GROUPS" {
        state.SetGroups(c)
			} else {
        state.AddCluster(name, c)
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
		case bool:
			c[key] = []string{fmt.Sprintf("%s", value.(bool))}
		case []interface{}:
			result := []string{}

			for _, x := range value.([]interface{}) {
				switch x.(type) {
				case string:
					result = append(result, fmt.Sprintf("%s", x))
				case int:
					result = append(result, fmt.Sprintf("%d", x))
				case bool:
					result = append(result, fmt.Sprintf("%s", x))
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
