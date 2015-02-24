grange-server
=============

A read-only range server frontend to
[grange](https://github.com/xaviershay/grange). See there for documentation of
concepts and query language.

Cluster and group data is specified in YAML files.


Installation
-----

1. Setup your go environment if needed

Example

    mkdir $HOME/go
    export GOPATH=$HOME/go

2. Install grange-server and erg client

    go get -v -u github.com/xaviershay/grange-server
    go get -v -u github.com/xaviershay/erg

Usage
-----

Put some range YAML files in `clusters/`, then:

    $GOPATH/bin/grange-server --port=8888 $GOPATH/src/github.com/xaviershay/grange-server/grange.gcfg

Simple expansion (with [erg client](https://github.com/xaviershay/erg)):

    erg -v 1..2

Find all clusters that have type key set to "mysql":

    erg -v '%{has(TYPE;mysql)}'

Features
--------

* Read-only serving of cluster YAML files on disk.
* `-parse` option to dry-run parse source files and not start the server. Will
  return non-zero exit code if any warnings were emitted.
* Reloads configuration file in response to HUP signal. Allows dynamic log
  level and cluster changes.
* Can report HTTP response metrics to statsd.

Format
------

See the `clusters` directory for sample YAML files. At minimum, your directory
should contain a `GROUPS.yaml`. See `grange.gcfg` for a sample configuration.
