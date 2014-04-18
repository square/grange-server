grange-server
=============

A read-only range server frontend to
[grange](https://github.com/xaviershay/grange). See there for documentation of
concepts and query language.

Cluster and group data is specified in YAML files.

Usage
-----

Put some range YAML files in `clusters/`, then:

    go build

    ./grange-server --port=8888 grange.yaml

    gem install rangeclient
    er -v localhost -p 8888 '%{has(TYPE;mysql)}'

Features
--------

* Read-only serving of cluster YAML files on disk.
* `-parse` option to dry-run parse source files and not start the server. Will
  return non-zero exit code if any warnings were emitted.
* Reloads configuration file in response to HUP signal. Allows dynamic log
  level and cluster changes.

Format
------

See the `clusters` directory for sample YAML files. At minimum, your directory
should contain a `GROUPS.yaml`.
