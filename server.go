package main

import (
	"gopkg.in/gcfg.v1"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/xaviershay/grange"
	"github.com/xaviershay/statsd" // For https://github.com/quipo/statsd/pull/9
	"gopkg.in/v1/yaml"
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
)

var (
	currentConfig serverConfig
	port          string
	parse         bool
	help          bool
	state         *grange.State
	configPath    string
	stats         subStatsd
)

// Because I'm too lazy to implement the entire noop Statsd interface
type subStatsd interface {
	Incr(string, int64) error
	Close() error
}

type noopStatsd struct{}

func (x *noopStatsd) Incr(y string, z int64) error { return nil }
func (x *noopStatsd) Close() error                 { return nil }

func queryHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	// Setup CORS Headers for OPTIONS request
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET")

	if r.Method == "OPTIONS" {
		stats.Incr("20X", 1)
		return
	}

	q, err := url.QueryUnescape(r.URL.RawQuery)
	if err != nil {
		stats.Incr("40X", 1)
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
		stats.Incr("20X", 1)
	} else {
		stats.Incr("40X", 1)
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
		fmt.Println("example: grange-server -port=8888 grange.gcfg")
		fmt.Println()

		flag.PrintDefaults()

		fmt.Println()
	}
	flag.StringVar(&port, "port", "8080", "HTTP Server Port")
	flag.BoolVar(&parse, "parse", false, "Do not start server. Non-zero exit code on parse warnings.")
	flag.BoolVar(&help, "help", false, "Show this message.")
	flag.Parse()
	stats = &noopStatsd{}
}

func main() {
	defer cleanupStatsd()

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

	if parse {
		warnings := loadConfig(configPath)
		Info("Not starting server because of -parse option")
		if warnings > 0 {
			os.Exit(1)
		}
	} else {
		configChannel := make(chan string)
		doneChannel := make(chan bool)

		go handleSignals(configChannel)
		go configLoop(configChannel, doneChannel)

		configChannel <- configPath

		// Wait for at least one config to be loaded before serving traffic.
		<-doneChannel

		// No longer care about listening to this channel
		go sink(doneChannel)

		var httpAddr string
		if strings.Contains(port, ":") {
			httpAddr = port
		} else {
			httpAddr = fmt.Sprintf(":%v", port)
		}

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
	yamlpath []string
}

func sink(channel chan bool) {
	for _ = range channel {
	}
}

func configLoop(configChannel chan string, doneChannel chan bool) {
	for path := range configChannel {
		loadConfig(path)
		doneChannel <- true
	}
}

func handleSignals(configChannel chan string) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)
	for sig := range c {
		switch sig {
		case syscall.SIGHUP:
			Info("Scheduling config reload in response to HUP")
			configChannel <- configPath
		}
	}
}

// Returns number of warnings emited while loading, or negative number for
// fatal error.
func loadConfig(path string) int {
	if len(path) > 0 {
		cfg := struct {
			Rangeserver struct {
				Loglevel string
				Yamlpath []string
			}

			Statsd struct {
				Host     string
				Prefix   string
				Interval float32
			}
		}{}

		err := gcfg.ReadFileInto(&cfg, path)
		if err != nil {
			Warn("Failed to parse gcfg data: %s", err)
		}

		if cfg.Rangeserver.Loglevel != "" {
			Debug("Setting loglevel from config: %s", cfg.Rangeserver.Loglevel)
			currentConfig.loglevel = cfg.Rangeserver.Loglevel
		} else {
			Debug("No loglevel found in config: %s")
		}

		if len(cfg.Rangeserver.Yamlpath) > 0 {
			for _, path := range cfg.Rangeserver.Yamlpath {
				Debug("Adding yamlpath from config: %s", path)
			}
			currentConfig.yamlpath = cfg.Rangeserver.Yamlpath
		} else {
			Debug("No yamlpath found in config: %s")
		}

		host := cfg.Statsd.Host
		if host != "" {
			Info("Connecting to statsd: %s", host)
			cleanupStatsd()
			statsdclient := statsd.NewStatsdClient(host, cfg.Statsd.Prefix)
			statsdclient.CreateSocket()
			if cfg.Statsd.Interval > 0 {
				bufferedStats := statsd.NewStatsdBuffer(time.Second*time.Duration(cfg.Statsd.Interval), statsdclient)
				bufferedStats.Logger = &GrangeLogger{Prefix: "statsd"}
				stats = bufferedStats
			} else {
				stats = statsdclient
			}
		}
		setLogLevel(currentConfig.loglevel)
	} else {
		// No config file, use defaults
		currentConfig.loglevel = "INFO"
		currentConfig.yamlpath = []string{"clusters"}
		setLogLevel(currentConfig.loglevel)
	}

	newState, warnings := loadState()
	for _, err := range newState.PrimeCache() {
		Warn(err.Error())
	}
	Info("Switching in new state with primed cache")
	state = newState
	return warnings
}

func loadState() (*grange.State, int) {
	state := grange.NewState()
	state.SetDefaultCluster("GROUPS")
	warnings := 0

	for _, dir := range currentConfig.yamlpath {

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
				state.AddCluster(name, c)
			}
		}
	}

	return &state, warnings
}

func cleanupStatsd() {
	stats.Close()
	stats = &noopStatsd{}
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
			c[key] = []string{fmt.Sprintf("%t", value.(bool))}
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
