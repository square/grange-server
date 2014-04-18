grange-server
=============

A proof-of-concept server making use of https://github.com/xaviershay/grange

Put some range YAML files in `clusters/`, then:

    go run logging.go server.go -port 8888

    gem install rangeclient
    er -v localhost -p 8888 '%your-cluster'

Features
--------

* Read-only serving of cluster YAML files on disk.
* `-parse` option to dry-run parse source files and not start the server.
* Reloads configuration file in response to HUP signal. Allows dynamic log
  level and cluster changes.
