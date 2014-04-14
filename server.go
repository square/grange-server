package main

import (
    "fmt"
    "log"
    "flag"
    "net/http"
    "net/url"
    "io/ioutil"
    "strings"
    "path/filepath"

    "gopkg.in/v1/yaml"

    "github.com/xaviershay/grange"
)

var (
    port int
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
    state = grange.NewState()

    files, _ := ioutil.ReadDir("./clusters") // TODO: Configurable
    for _, f := range files {
      basename := f.Name()
      name := strings.TrimSuffix(basename, filepath.Ext(basename))

      dat, _ := ioutil.ReadFile("clusters/" + basename)
      var c grange.Cluster;

      _ = yaml.Unmarshal(dat, &c)
      grange.AddCluster(state, name, c)
    }

    httpAddr := fmt.Sprintf(":%v", port)

    log.Printf("Listening to %v", httpAddr)

    http.HandleFunc("/", rootHandler)
    log.Fatal(http.ListenAndServe(httpAddr, nil))
}
